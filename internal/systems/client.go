// Package systems supplies a bounded, ephemeral autocomplete over the public
// Spansh name endpoint. It never downloads or persists a galaxy catalogue.
package systems

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	Endpoint         = "https://spansh.co.uk/api/systems/field_values/system_names"
	MinChars         = 3
	MaxResults       = 20
	maxResponseBytes = 256 << 10
	maxScopes        = 32
)

type Options struct {
	HTTPClient  *http.Client
	Endpoint    string
	Now         func() time.Time
	MinInterval time.Duration // Zero selects 400ms; negative disables spacing for tests.
	TTL         time.Duration
	CacheSize   int
}
type Request struct {
	Query    string `json:"query"`
	FieldID  string `json:"fieldId"`
	Sequence uint64 `json:"sequence"`
	Limit    int    `json:"limit"`
}
type Coordinates struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type Suggestion struct {
	Name        string       `json:"name"`
	ID64        string       `json:"id64,omitempty"`
	Coordinates *Coordinates `json:"coordinates,omitempty"`
	Match       string       `json:"match"`
}
type Result struct {
	Query        string       `json:"query"`
	FieldID      string       `json:"fieldId"`
	Sequence     uint64       `json:"sequence"`
	Status       string       `json:"status"`
	Source       string       `json:"source"`
	MinChars     int          `json:"minChars"`
	Suggestions  []Suggestion `json:"suggestions"`
	Limited      bool         `json:"limited"`
	Cached       bool         `json:"cached"`
	ErrorCode    string       `json:"errorCode,omitempty"`
	RetryAfterMS int64        `json:"retryAfterMs,omitempty"`
}
type queryError struct {
	code  string
	retry time.Duration
}

func (e *queryError) Error() string { return "system autocomplete: " + e.code }

type scope struct {
	sequence uint64
	cancel   context.CancelFunc
	used     uint64
}
type cached struct {
	key     string
	result  Result
	expires time.Time
}
type Client struct {
	mu                        sync.Mutex
	http                      *http.Client
	endpoint                  string
	now                       func() time.Time
	spacing, ttl              time.Duration
	capacity                  int
	cache                     map[string]*list.Element
	lru                       *list.List
	scopes                    map[string]*scope
	clock                     uint64
	gate                      chan struct{}
	lastRequest, blockedUntil time.Time
	closed                    bool
}

func NewClient(o Options) *Client {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{}
	}
	copyHTTP := *o.HTTPClient
	copyHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if o.Endpoint == "" {
		o.Endpoint = Endpoint
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MinInterval == 0 {
		o.MinInterval = 400 * time.Millisecond
	} else if o.MinInterval < 0 {
		o.MinInterval = 0
	}
	if o.TTL <= 0 {
		o.TTL = 5 * time.Minute
	}
	if o.CacheSize <= 0 {
		o.CacheSize = 128
	}
	if o.CacheSize > 512 {
		o.CacheSize = 512
	}
	return &Client{http: &copyHTTP, endpoint: o.Endpoint, now: o.Now, spacing: o.MinInterval, ttl: o.TTL, capacity: o.CacheSize, cache: map[string]*list.Element{}, lru: list.New(), scopes: map[string]*scope{}, gate: make(chan struct{}, 1)}
}

func normalize(q string) string { return strings.Join(strings.Fields(q), " ") }
func key(q string) string       { return strings.ToLower(normalize(q)) }
func validText(q string) bool {
	if !utf8.ValidString(q) || utf8.RuneCountInString(q) > 200 {
		return false
	}
	for _, r := range q {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
func comparable(q string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, q)
}
func cloneResult(r Result) Result {
	r.Suggestions = append([]Suggestion{}, r.Suggestions...)
	for i := range r.Suggestions {
		if p := r.Suggestions[i].Coordinates; p != nil {
			copy := *p
			r.Suggestions[i].Coordinates = &copy
		}
	}
	return r
}
func (c *Client) begin(ctx context.Context, field string, seq uint64) (context.Context, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ctx, false
	}
	old := c.scopes[field]
	if old != nil && seq <= old.sequence {
		return ctx, false
	}
	if old == nil && len(c.scopes) >= maxScopes {
		var oldest string
		age := uint64(math.MaxUint64)
		for id, s := range c.scopes {
			if s.used < age {
				oldest, age = id, s.used
			}
		}
		if s := c.scopes[oldest]; s != nil && s.cancel != nil {
			s.cancel()
		}
		delete(c.scopes, oldest)
	}
	if old != nil && old.cancel != nil {
		old.cancel()
	}
	child, cancel := context.WithCancel(ctx)
	c.clock++
	c.scopes[field] = &scope{sequence: seq, cancel: cancel, used: c.clock}
	return child, true
}
func (c *Client) finish(field string, seq uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.scopes[field]; s != nil && s.sequence == seq {
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel = nil
	}
}
func (c *Client) Cancel(field string, seq uint64) {
	if field == "" || len(field) > 128 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.scopes[field]; s != nil && seq >= s.sequence {
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel = nil
		s.sequence = seq
	} else if s == nil && !c.closed && len(c.scopes) < maxScopes {
		c.clock++
		c.scopes[field] = &scope{sequence: seq, used: c.clock}
	}
}
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, s := range c.scopes {
		if s.cancel != nil {
			s.cancel()
		}
	}
	c.scopes = map[string]*scope{}
	c.cache = map[string]*list.Element{}
	c.lru.Init()
}
func (c *Client) getCached(q string) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.cache[key(q)]
	if e == nil {
		return Result{}, false
	}
	entry := e.Value.(cached)
	if !c.now().Before(entry.expires) {
		delete(c.cache, entry.key)
		c.lru.Remove(e)
		return Result{}, false
	}
	c.lru.MoveToFront(e)
	r := cloneResult(entry.result)
	r.Cached = true
	return r, true
}
func (c *Client) putCached(q string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	k, ttl := key(q), c.ttl
	if r.Status == "empty" {
		ttl = min(ttl, 30*time.Second)
	}
	if e := c.cache[k]; e != nil {
		c.lru.Remove(e)
	}
	c.cache[k] = c.lru.PushFront(cached{k, cloneResult(r), c.now().Add(ttl)})
	for c.lru.Len() > c.capacity {
		e := c.lru.Back()
		delete(c.cache, e.Value.(cached).key)
		c.lru.Remove(e)
	}
}

// Query is cancellable per field. A newer sequence supersedes its predecessor;
// separate fields cannot cancel one another. Expected failures retain a result
// with an errorCode, so renderers can translate status without exposing a body.
func (c *Client) Query(ctx context.Context, req Request) (result Result, err error) {
	result = Result{Query: normalize(req.Query), FieldID: req.FieldID, Sequence: req.Sequence, Status: "empty", Source: "Spansh", MinChars: MinChars, Suggestions: []Suggestion{}}
	if req.FieldID == "" || len(req.FieldID) > 128 || !validText(req.Query) {
		result.Status, result.ErrorCode = "error", "INVALID_QUERY"
		return result, &queryError{code: result.ErrorCode}
	}
	child, ok := c.begin(ctx, req.FieldID, req.Sequence)
	if !ok {
		result.Status = "cancelled"
		return result, context.Canceled
	}
	defer c.finish(req.FieldID, req.Sequence)
	if utf8.RuneCountInString(comparable(result.Query)) < MinChars {
		result.Status = "too-short"
		return result, nil
	}
	limit := req.Limit
	if limit <= 0 || limit > MaxResults {
		limit = MaxResults
	}
	defer func() {
		if child.Err() != nil {
			result.Status, result.ErrorCode = "cancelled", ""
			result.Suggestions = []Suggestion{}
			err = child.Err()
		}
		result.Query, result.FieldID, result.Sequence = normalize(req.Query), req.FieldID, req.Sequence
		if len(result.Suggestions) > limit {
			result.Limited = true
			result.Suggestions = result.Suggestions[:limit]
		}
	}()
	if cached, ok := c.getCached(result.Query); ok {
		return cached, nil
	}
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-child.Done():
		return result, child.Err()
	}
	if cached, ok := c.getCached(result.Query); ok {
		return cached, nil
	}
	deadline, cancel := context.WithTimeout(child, 8*time.Second)
	defer cancel()
	var names []Suggestion
	names, result.Limited, err = c.fetch(deadline, result.Query)
	if err == nil {
		result.Suggestions = relevant(names, result.Query, false)
		// A known spelling correction is only a lookup hint. The selected
		// canonical record must actually be returned by the remote provider.
		if len(result.Suggestions) == 0 && correctionQuery(result.Query) != "" {
			var extra []Suggestion
			var capped bool
			extra, capped, err = c.fetch(deadline, correctionQuery(result.Query))
			result.Limited = result.Limited || capped
			if err == nil {
				result.Suggestions = relevant(extra, result.Query, true)
			}
		}
	}
	if err != nil {
		result.Status, result.ErrorCode = "error", "NETWORK"
		var issue *queryError
		if errors.As(err, &issue) {
			result.ErrorCode = issue.code
			result.RetryAfterMS = issue.retry.Milliseconds()
			if issue.code == "RATE_LIMIT" {
				result.Status = "rate-limited"
			}
		}
		return result, err
	}
	if len(result.Suggestions) > 0 {
		result.Status = "ready"
	}
	c.putCached(result.Query, result)
	return result, nil
}

func (c *Client) fetch(ctx context.Context, q string) ([]Suggestion, bool, error) {
	c.mu.Lock()
	blocked, last := c.blockedUntil, c.lastRequest
	c.mu.Unlock()
	if wait := time.Until(blocked); wait > 0 {
		return nil, false, &queryError{code: "RATE_LIMIT", retry: wait}
	}
	if wait := c.spacing - time.Since(last); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	uri, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, false, &queryError{code: "ENDPOINT"}
	}
	values := uri.Query()
	values.Set("q", q)
	uri.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
	if err != nil {
		return nil, false, &queryError{code: "ENDPOINT"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED Copilot/SystemAutocomplete")
	c.mu.Lock()
	c.lastRequest = time.Now()
	c.mu.Unlock()
	response, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		retry := 30 * time.Second
		if n, e := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 32); e == nil {
			retry = time.Duration(n) * time.Second
		} else if date, e := http.ParseTime(response.Header.Get("Retry-After")); e == nil {
			retry = time.Until(date)
		}
		retry = max(time.Second, min(5*time.Minute, retry))
		c.mu.Lock()
		c.blockedUntil = time.Now().Add(retry)
		c.mu.Unlock()
		return nil, false, &queryError{code: "RATE_LIMIT", retry: retry}
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, &queryError{code: fmt.Sprintf("HTTP_%d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > maxResponseBytes {
		return nil, false, &queryError{code: "RESPONSE_TOO_LARGE"}
	}
	var payload struct {
		Values  []string `json:"values"`
		Details []struct {
			Name string      `json:"name"`
			ID64 json.Number `json:"id64"`
			X    *float64    `json:"x"`
			Y    *float64    `json:"y"`
			Z    *float64    `json:"z"`
		} `json:"min_max"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, false, &queryError{code: "RESPONSE_INVALID"}
	}
	if payload.Values == nil && payload.Details == nil {
		var shape map[string]json.RawMessage
		if json.Unmarshal(body, &shape) != nil || shape["values"] == nil {
			return nil, false, &queryError{code: "RESPONSE_INVALID"}
		}
	}
	details := map[string]Suggestion{}
	for _, row := range payload.Details {
		s := Suggestion{Name: row.Name}
		if _, e := strconv.ParseUint(string(row.ID64), 10, 64); e == nil {
			s.ID64 = string(row.ID64)
		}
		if row.X != nil && row.Y != nil && row.Z != nil && math.Abs(*row.X) < 1e9 && math.Abs(*row.Y) < 1e9 && math.Abs(*row.Z) < 1e9 {
			s.Coordinates = &Coordinates{*row.X, *row.Y, *row.Z}
		}
		details[key(row.Name)] = s
	}
	rows, seen := []Suggestion{}, map[string]bool{}
	for _, name := range payload.Values {
		if !validText(name) || strings.TrimSpace(name) == "" || seen[key(name)] {
			continue
		}
		seen[key(name)] = true
		s := details[key(name)]
		s.Name = name
		rows = append(rows, s)
		if len(rows) == MaxResults {
			break
		}
	}
	return rows, len(payload.Values) >= MaxResults, nil
}
