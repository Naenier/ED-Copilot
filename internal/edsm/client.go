// Package edsm queries the public EDSM catalogue on demand.
package edsm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Object preserves integer identifiers as json.Number, including values that
// cannot be represented exactly as a float64.
type Object = map[string]any

var (
	ErrNotFound       = errors.New("system not found in EDSM")
	ErrTooManyMatches = errors.New("too many matching systems in EDSM")
	ErrBusy           = errors.New("EDSM request queue is full")
)

type RateLimitError struct {
	RetryAt time.Time
}

func (e *RateLimitError) Error() string {
	return "EDSM requested a pause until " + e.RetryAt.UTC().Format(time.RFC3339)
}

const (
	requestInterval = time.Second
	requestTimeout  = 12 * time.Second
	fallbackPause   = 30 * time.Second
	notFoundTTL     = 2 * time.Minute
	maxPending      = 12
	maxCachedItems  = 64
	maxCachedBytes  = 12 << 20
)

type dataset struct {
	path       string
	collection string
	list       bool
	maxBytes   int64
	ttl        time.Duration
}

var (
	systemData  = dataset{path: "/api-v1/system", maxBytes: 64 << 10, ttl: 30 * time.Minute}
	bodiesData  = dataset{path: "/api-system-v1/bodies", collection: "bodies", maxBytes: 4 << 20, ttl: 6 * time.Hour}
	stationData = dataset{path: "/api-system-v1/stations", collection: "stations", maxBytes: 2 << 20, ttl: 10 * time.Minute}
	factionData = dataset{path: "/api-system-v1/factions", collection: "factions", maxBytes: 1 << 20, ttl: 5 * time.Minute}
	nearbyData  = dataset{path: "/api-v1/sphere-systems", list: true, maxBytes: 2 << 20, ttl: 20 * time.Minute}
	prefixData  = dataset{path: "/api-v1/systems", list: true, maxBytes: 128 << 10, ttl: 5 * time.Minute}
)

type reply struct {
	data []byte
	err  error
}

type cached struct {
	reply
	expires time.Time
	used    uint64
}

type pending struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	reply   reply
}

// Client shares one request cadence, cooldown and bounded in-memory cache
// across the application's EDSM consumers.
type Client struct {
	http     *http.Client
	endpoint string
	interval time.Duration
	now      func() time.Time
	network  chan struct{}

	mu            sync.Mutex
	nextRequest   time.Time
	cooldownUntil time.Time
	cache         map[string]cached
	cacheBytes    int
	serial        uint64
	pending       map[string]*pending
}

func NewClient(transport ...*http.Client) *Client {
	httpClient := http.Client{Timeout: 8 * time.Second}
	if len(transport) > 0 && transport[0] != nil {
		httpClient = *transport[0]
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{
		http: &httpClient, endpoint: "https://www.edsm.net", interval: requestInterval,
		now: time.Now, network: make(chan struct{}, 1),
		cache: make(map[string]cached), pending: make(map[string]*pending),
	}
}

func (c *Client) System(ctx context.Context, name string) (Object, error) {
	return c.object(ctx, systemData, name, url.Values{
		"showId": {"1"}, "showCoordinates": {"1"}, "showPermit": {"1"},
		"showInformation": {"1"}, "showPrimaryStar": {"1"},
	})
}

func (c *Client) Bodies(ctx context.Context, name string) (Object, error) {
	return c.object(ctx, bodiesData, name, make(url.Values))
}

func (c *Client) Stations(ctx context.Context, name string) (Object, error) {
	return c.object(ctx, stationData, name, make(url.Values))
}

func (c *Client) Factions(ctx context.Context, name string) (Object, error) {
	return c.object(ctx, factionData, name, make(url.Values))
}

func (c *Client) Nearby(ctx context.Context, name string, radius int) ([]Object, error) {
	if radius < 1 || radius > 100 {
		return nil, errors.New("EDSM radius must be between 1 and 100 light-years")
	}
	return c.list(ctx, nearbyData, name, url.Values{
		"radius": {strconv.Itoa(radius)}, "showId": {"1"},
		"showCoordinates": {"1"}, "showPermit": {"1"}, "showInformation": {"1"},
	})
}

// Prefix is intended for specific autocomplete queries after a bounded primary
// provider has been tried. EDSM does not document a server-side result limit.
func (c *Client) Prefix(ctx context.Context, name string, limit int) ([]Object, error) {
	name = strings.TrimSpace(name)
	chars := 0
	for _, ch := range name {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			chars++
		}
	}
	if chars < 8 || strings.ContainsAny(name, "%_*") {
		return nil, ErrTooManyMatches
	}
	rows, err := c.list(ctx, prefixData, name, url.Values{
		"showId": {"1"}, "showCoordinates": {"1"}, "showPermit": {"1"},
	})
	if err != nil {
		return nil, err
	}
	normalized := strings.ToLower(name)
	filtered := rows[:0]
	for _, row := range rows {
		title, _ := row["name"].(string)
		if strings.HasPrefix(strings.ToLower(title), normalized) {
			filtered = append(filtered, row)
		}
	}
	if len(filtered) > max(1, min(limit, 30)) {
		return nil, ErrTooManyMatches
	}
	return filtered, nil
}

func (c *Client) object(ctx context.Context, source dataset, name string, query url.Values) (Object, error) {
	data, err := c.get(ctx, source, name, query)
	if err != nil {
		return nil, err
	}
	return decodeObject(data, source.collection)
}

func (c *Client) list(ctx context.Context, source dataset, name string, query url.Values) ([]Object, error) {
	data, err := c.get(ctx, source, name, query)
	if err != nil {
		return nil, err
	}
	return decodeList(data)
}

func (c *Client) get(ctx context.Context, source dataset, name string, query url.Values) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 150 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return nil, errors.New("invalid system name")
	}
	key := source.path + "?" + query.Encode() + "\n" + strings.ToLower(name)
	query.Set("systemName", name)

	c.mu.Lock()
	now := c.now()
	if item, ok := c.cache[key]; ok {
		if now.Before(item.expires) {
			c.serial++
			item.used = c.serial
			c.cache[key] = item
			c.mu.Unlock()
			return item.data, item.err
		}
		c.removeCached(key)
	}
	if now.Before(c.cooldownUntil) {
		retryAt := c.cooldownUntil
		c.mu.Unlock()
		return nil, &RateLimitError{RetryAt: retryAt}
	}
	inFlight := c.pending[key]
	if inFlight == nil {
		if len(c.pending) >= maxPending {
			c.mu.Unlock()
			return nil, ErrBusy
		}
		requestContext, cancel := context.WithTimeout(context.Background(), requestTimeout)
		inFlight = &pending{done: make(chan struct{}), cancel: cancel}
		c.pending[key] = inFlight
		go c.run(requestContext, source, query, key, inFlight)
	}
	inFlight.waiters++
	c.mu.Unlock()

	select {
	case <-inFlight.done:
		return inFlight.reply.data, inFlight.reply.err
	case <-ctx.Done():
		c.mu.Lock()
		inFlight.waiters--
		if inFlight.waiters == 0 && c.pending[key] == inFlight {
			delete(c.pending, key)
			inFlight.cancel()
		}
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) run(ctx context.Context, source dataset, query url.Values, key string, inFlight *pending) {
	defer inFlight.cancel()
	data, err := c.request(ctx, source, query)
	inFlight.reply = reply{data: data, err: err}
	c.mu.Lock()
	if c.pending[key] == inFlight {
		delete(c.pending, key)
		ttl := source.ttl
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrTooManyMatches) {
			ttl = notFoundTTL
		}
		if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrTooManyMatches) {
			c.saveCached(key, inFlight.reply, ttl)
		}
	}
	c.mu.Unlock()
	close(inFlight.done)
}

func (c *Client) request(ctx context.Context, source dataset, query url.Values) ([]byte, error) {
	select {
	case c.network <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.network }()

	c.mu.Lock()
	now := c.now()
	wait := c.nextRequest.Sub(now)
	if now.Before(c.cooldownUntil) {
		retryAt := c.cooldownUntil
		c.mu.Unlock()
		return nil, &RateLimitError{RetryAt: retryAt}
	}
	c.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+source.path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED-Copilot (https://github.com/Naenier/ED-Copilot)")
	c.mu.Lock()
	c.nextRequest = c.now().Add(c.interval)
	c.mu.Unlock()
	response, err := c.http.Do(req)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			err = requestError.Err
		}
		return nil, fmt.Errorf("EDSM request failed: %w", err)
	}
	defer response.Body.Close()
	return c.read(response, source)
}

func (c *Client) read(response *http.Response, source dataset) ([]byte, error) {
	retryHeader := response.Header.Get("Retry-After")
	switch {
	case response.StatusCode == http.StatusTooManyRequests ||
		response.StatusCode == http.StatusServiceUnavailable && retryHeader != "":
		retryAt := parseRetryAt(retryHeader, c.now())
		c.mu.Lock()
		c.cooldownUntil = retryAt
		c.mu.Unlock()
		return nil, &RateLimitError{RetryAt: retryAt}
	case response.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case response.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("EDSM returned HTTP %d", response.StatusCode)
	}

	oversized := func() error {
		if source.path == prefixData.path {
			return ErrTooManyMatches
		}
		return fmt.Errorf("EDSM response exceeded %d bytes", source.maxBytes)
	}
	if response.ContentLength > source.maxBytes {
		return nil, oversized()
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, source.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read EDSM response: %w", err)
	}
	if int64(len(body)) > source.maxBytes {
		return nil, oversized()
	}
	if source.list {
		_, err = decodeList(body)
	} else {
		_, err = decodeObject(body, source.collection)
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

func parseRetryAt(header string, now time.Time) time.Time {
	header = strings.TrimSpace(header)
	if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds >= 0 {
		return now.Add(max(requestInterval, time.Duration(seconds)*time.Second))
	}
	if date, err := http.ParseTime(header); err == nil {
		return maxTime(now.Add(requestInterval), date)
	}
	return now.Add(fallbackPause)
}

func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

func decode(body []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid EDSM JSON response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("invalid EDSM JSON response")
	}
	return value, nil
}

func decodeObject(body []byte, collection string) (Object, error) {
	value, err := decode(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(Object)
	if !ok {
		if list, ok := value.([]any); value == nil || ok && len(list) == 0 {
			return nil, ErrNotFound
		}
		return nil, errors.New("invalid EDSM system response")
	}
	if len(object) == 0 {
		return nil, ErrNotFound
	}
	if name, _ := object["name"].(string); strings.TrimSpace(name) == "" {
		return nil, errors.New("EDSM response omitted the system name")
	}
	if collection != "" {
		if _, ok := object[collection].([]any); !ok {
			return nil, fmt.Errorf("EDSM response omitted the %s collection", collection)
		}
	}
	return object, nil
}

func decodeList(body []byte) ([]Object, error) {
	value, err := decode(body)
	if err != nil {
		return nil, err
	}
	if object, ok := value.(Object); value == nil || ok && len(object) == 0 {
		return []Object{}, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("invalid EDSM systems response")
	}
	rows := make([]Object, 0, len(list))
	for _, item := range list {
		object, ok := item.(Object)
		name, _ := object["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, errors.New("invalid system in EDSM results")
		}
		rows = append(rows, object)
	}
	return rows, nil
}

func (c *Client) removeCached(key string) {
	c.cacheBytes -= len(c.cache[key].data)
	delete(c.cache, key)
}

func (c *Client) saveCached(key string, answer reply, ttl time.Duration) {
	for len(c.cache) >= maxCachedItems || c.cacheBytes+len(answer.data) > maxCachedBytes {
		var oldestKey string
		oldest := c.serial + 1
		for key, item := range c.cache {
			if item.used < oldest {
				oldest, oldestKey = item.used, key
			}
		}
		c.removeCached(oldestKey)
	}
	c.serial++
	c.cache[key] = cached{reply: answer, expires: c.now().Add(ttl), used: c.serial}
	c.cacheBytes += len(answer.data)
}
