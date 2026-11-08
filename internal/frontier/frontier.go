// Package frontier implements the registered native Frontier PKCE client.
package frontier

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/securestore"
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const ClientID = "0a4c1d43-e433-411e-bcf5-54527fe5a34f"
const DefaultRedirect = "http://localhost:7122/api/frontier/callback"

type Error struct {
	Message, Code string
	Status        int
}

func (e *Error) Error() string                    { return e.Message }
func fail(message, code string, status int) error { return &Error{message, code, status} }

type Options struct {
	DataDir, RedirectURI, AuthURL, CAPIURL string
	HTTPClient                             *http.Client
	GetCommander                           func() v.M
	OnUpdate                               func()
	Now                                    func() time.Time
	Encrypt, Decrypt                       func([]byte) ([]byte, error)
}
type pending struct {
	Verifier, State, ExpectedFID, ExpectedName string
	Expires                                    time.Time
	Epoch                                      uint64
}
type Client struct {
	mu, refreshMu, syncMu     sync.Mutex
	opts                      Options
	http                      *http.Client
	store                     *securestore.Store
	tokens, identity, summary v.M
	pending                   *pending
	epoch                     uint64
	lastSync, lastCarrierSync string
	endpointTimes             map[string]time.Time
	lastError                 string
	syncing, mismatch         bool
	storageAvailable          bool
}

func New(o Options) (*Client, error) {
	if o.DataDir == "" {
		return nil, errors.New("Не задан профиль Frontier.")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.GetCommander == nil {
		o.GetCommander = func() v.M { return v.M{} }
	}
	if o.RedirectURI == "" {
		o.RedirectURI = DefaultRedirect
	}
	u, err := url.Parse(o.RedirectURI)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Нужен локальный HTTP-адрес возврата Frontier с портом.")
	}
	if o.AuthURL == "" {
		o.AuthURL = "https://auth.frontierstore.net"
	}
	if o.CAPIURL == "" {
		o.CAPIURL = "https://companion.orerve.net"
	}
	hc := o.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	clone := *hc
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	s := securestore.New(o.DataDir)
	storageAvailable := o.Encrypt != nil && o.Decrypt != nil || s.Available()
	if o.Encrypt == nil {
		o.Encrypt = s.Encrypt
	}
	if o.Decrypt == nil {
		o.Decrypt = s.Decrypt
	}
	c := &Client{opts: o, http: &clone, store: s, summary: v.M{}, endpointTimes: map[string]time.Time{}, storageAvailable: storageAvailable}
	tokenBytes, tokenReadErr := os.ReadFile(c.tokenFile())
	if os.IsNotExist(tokenReadErr) {
		tokenBytes, tokenReadErr = os.ReadFile(filepath.Join(o.DataDir, "frontier-tokens.enc"))
	}
	if b, err := tokenBytes, tokenReadErr; err == nil {
		plain, err := o.Decrypt(b)
		if err == nil {
			var saved v.M
			dec := json.NewDecoder(strings.NewReader(string(plain)))
			dec.UseNumber()
			if dec.Decode(&saved) == nil && v.String(saved, "clientId") == ClientID && (v.Bool(saved["loggedOut"]) || v.String(saved, "refreshToken") != "") {
				if !v.Bool(saved["loggedOut"]) {
					c.tokens = saved
					c.identity = v.Obj(saved["identity"])
				}
			} else {
				c.lastError = "Не удалось открыть сохранённый вход Frontier. Войдите заново."
			}
		} else {
			c.lastError = "Не удалось открыть сохранённый вход Frontier. Войдите заново."
		}
		clear(plain)
	}
	return c, nil
}
func (c *Client) tokenFile() string { return filepath.Join(c.opts.DataDir, "frontier-go-tokens.enc") }
func (c *Client) available() bool {
	return c.storageAvailable
}
func (c *Client) notify() {
	if c.opts.OnUpdate != nil {
		go c.opts.OnUpdate()
	}
}
func (c *Client) Status(commanders ...v.M) v.M {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(commanders) > 0 && c.identity != nil {
		c.mismatch = c.assert(c.identity, commanders[len(commanders)-1]) != nil
	}
	s := v.M{"storageAvailable": c.available(), "connected": c.tokens != nil, "pending": c.pending != nil && c.opts.Now().Before(c.pending.Expires), "syncing": c.syncing, "commander": v.String(c.identity, "name"), "fid": v.String(c.identity, "fid"), "lastSync": nil, "lastCarrierSync": nil, "nextSyncAt": nil, "nextCarrierSyncAt": nil, "commanderMismatch": c.mismatch, "error": c.lastError, "summary": v.Copy(c.summary)}
	if c.lastSync != "" {
		s["lastSync"] = c.lastSync
	}
	if c.lastCarrierSync != "" {
		s["lastCarrierSync"] = c.lastCarrierSync
	}
	for key, at := range c.endpointTimes {
		if key == "profile" {
			s["nextSyncAt"] = at.Add(time.Minute).UnixMilli()
		} else if key == "carrier" {
			s["nextCarrierSyncAt"] = at.Add(15 * time.Minute).UnixMilli()
		}
	}
	if c.mismatch {
		s["summary"] = v.M{}
	}
	return s
}
func randomString(n int) (string, error) {
	b := make([]byte, n)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func (c *Client) LoginURL() (string, error) {
	if !c.available() {
		return "", errors.New("Защищённое хранилище Windows недоступно.")
	}
	verifier, e := randomString(48)
	if e != nil {
		return "", e
	}
	state, e := randomString(32)
	if e != nil {
		return "", e
	}
	expected := c.opts.GetCommander()
	c.mu.Lock()
	c.epoch++
	c.pending = &pending{verifier, state, v.String(expected, "fid"), v.String(expected, "name"), c.opts.Now().Add(10 * time.Minute), c.epoch}
	c.lastError = ""
	c.mu.Unlock()
	challenge := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "audience": {"frontier,steam,epic"}, "scope": {"auth capi"}, "client_id": {ClientID}, "redirect_uri": {c.opts.RedirectURI}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	c.notify()
	return c.opts.AuthURL + "/auth?" + q.Encode(), nil
}
func (c *Client) request(ctx context.Context, target, method string, body io.Reader, headers map[string]string, timeout time.Duration) (v.M, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fail("Некорректный запрос Frontier.", "REQUEST", 400)
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "EDCopilot/2.0.0")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fail("Frontier не отвечает. Повторите попытку позднее.", "NETWORK", 502)
	}
	defer res.Body.Close()
	if res.StatusCode == 204 {
		return nil, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code, message := "HTTP_ERROR", fmt.Sprintf("Frontier вернул ошибку HTTP %d.", res.StatusCode)
		if res.StatusCode == 401 {
			code = "UNAUTHORIZED"
			message = "Срок входа Frontier истёк. Войдите заново."
		}
		if res.StatusCode == 429 {
			code = "RATE_LIMITED"
			message = "Frontier ограничил частоту запросов. Повторите позднее."
		}
		return nil, fail(message, code, res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16*1024*1024+1))
	if err != nil || len(raw) > 16*1024*1024 {
		return nil, fail("Некорректный ответ Frontier.", "INVALID_RESPONSE", 502)
	}
	var result v.M
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if dec.Decode(&result) != nil || result == nil {
		return nil, fail("Некорректный ответ Frontier.", "INVALID_RESPONSE", 502)
	}
	return result, nil
}
func (c *Client) token(ctx context.Context, fields url.Values) (v.M, error) {
	fields.Set("client_id", ClientID)
	data, err := c.request(ctx, c.opts.AuthURL+"/token", "POST", strings.NewReader(fields.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if v.String(data, "access_token") == "" || v.String(data, "token_type") != "" && !strings.EqualFold(v.String(data, "token_type"), "bearer") {
		return nil, fail("Frontier не выдал токен доступа.", "TOKEN_INVALID", 502)
	}
	refresh := v.String(data, "refresh_token")
	if refresh == "" && fields.Get("grant_type") == "refresh_token" {
		refresh = fields.Get("refresh_token")
	}
	expires := v.Num(data["expires_in"])
	if expires < 1 {
		expires = 3600
	}
	return v.M{"accessToken": v.String(data, "access_token"), "refreshToken": refresh, "expires": c.opts.Now().Add(time.Duration(expires) * time.Second).UnixMilli(), "clientId": ClientID}, nil
}
func (c *Client) assert(identity, expected v.M) error {
	if v.String(expected, "fid") != "" && v.String(identity, "fid") != v.String(expected, "fid") || v.String(expected, "fid") == "" && v.String(expected, "name") != "" && v.String(identity, "name") != "" && !strings.EqualFold(v.String(expected, "name"), v.String(identity, "name")) {
		return fail("Аккаунт Frontier не совпадает с командиром в журнале игры.", "COMMANDER_MISMATCH", 400)
	}
	return nil
}
func (c *Client) persistLocked(tokens, identity v.M) error {
	if v.String(tokens, "refreshToken") == "" {
		return fail("Frontier не выдал токен продолжения входа.", "REFRESH_REQUIRED", 502)
	}
	saved := v.Copy(tokens)
	saved["identity"] = identity
	b, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	cipher, err := c.opts.Encrypt(b)
	clear(b)
	if err != nil {
		return fail("Не удалось безопасно сохранить вход Frontier.", "SECURE_STORAGE_FAILED", 500)
	}
	if err = securestore.AtomicWrite(c.tokenFile(), cipher); err != nil {
		return fail("Не удалось безопасно сохранить вход Frontier.", "SECURE_STORAGE_FAILED", 500)
	}
	return nil
}
func (c *Client) HandleCallback(params url.Values) error {
	return c.HandleCallbackContext(context.Background(), params)
}
func (c *Client) HandleCallbackContext(ctx context.Context, params url.Values) (problem error) {
	c.mu.Lock()
	p := c.pending
	if p == nil || !c.opts.Now().Before(p.Expires) || len(params["state"]) != 1 || subtle.ConstantTimeCompare([]byte(params.Get("state")), []byte(p.State)) != 1 {
		c.mu.Unlock()
		return fail("Ответ входа Frontier устарел или не относится к текущему запросу.", "STATE_INVALID", 400)
	}
	c.pending = nil
	c.mu.Unlock()
	defer func() {
		if problem != nil {
			c.mu.Lock()
			if p.Epoch == c.epoch {
				c.lastError = problem.Error()
			}
			c.mu.Unlock()
		}
		c.notify()
	}()
	if _, has := params["error"]; has {
		return fail("Вход Frontier отменён или не разрешён.", "ACCESS_DENIED", 400)
	}
	if len(params["code"]) != 1 || params.Get("code") == "" || len(params.Get("code")) > 8192 {
		return fail("Frontier не вернул код авторизации.", "CODE_REQUIRED", 400)
	}
	tokens, err := c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {params.Get("code")}, "code_verifier": {p.Verifier}, "redirect_uri": {c.opts.RedirectURI}})
	if err != nil {
		return err
	}
	decoded, err := c.request(ctx, c.opts.AuthURL+"/decode", "GET", nil, map[string]string{"Authorization": "Bearer " + v.String(tokens, "accessToken")}, 30*time.Second)
	if err != nil {
		return err
	}
	customer := v.Str(v.At(decoded, "usr", "customer_id"))
	if _, err = strconv.ParseUint(customer, 10, 64); err != nil {
		return fail("Frontier не подтвердил владельца аккаунта.", "IDENTITY_INVALID", 502)
	}
	identity := v.M{"fid": "F" + customer, "name": ""}
	if err = c.assert(identity, v.M{"fid": p.ExpectedFID, "name": p.ExpectedName}); err != nil {
		return err
	}
	if err = c.assert(identity, c.opts.GetCommander()); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if p.Epoch != c.epoch {
		return fail("Вход Frontier отменён.", "CANCELLED", 409)
	}
	if err = c.persistLocked(tokens, identity); err != nil {
		return err
	}
	c.tokens = tokens
	c.identity = identity
	c.summary = v.M{}
	c.endpointTimes = map[string]time.Time{}
	c.lastSync = ""
	c.lastCarrierSync = ""
	c.lastError = ""
	c.mismatch = false
	return nil
}
func (c *Client) refresh(ctx context.Context, expected string) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.mu.Lock()
	if c.tokens == nil {
		c.mu.Unlock()
		return fail("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401)
	}
	if v.String(c.tokens, "accessToken") != expected {
		c.mu.Unlock()
		return nil
	}
	epoch := c.epoch
	oldRefresh := v.String(c.tokens, "refreshToken")
	c.mu.Unlock()
	tokens, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {oldRefresh}})
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.epoch {
		return fail("Вход Frontier отменён.", "CANCELLED", 409)
	}
	if err != nil {
		c.lastError = err.Error()
		var issue *Error
		if errors.As(err, &issue) && (issue.Status == 400 || issue.Status == 401) {
			c.tokens = nil
			os.Remove(c.tokenFile())
		}
		return err
	}
	if err = c.persistLocked(tokens, c.identity); err != nil {
		return err
	}
	c.tokens = tokens
	c.lastError = ""
	return nil
}
func (c *Client) capi(ctx context.Context, endpoint string) (v.M, error) {
	c.mu.Lock()
	tokens := v.Copy(c.tokens)
	identity := v.Copy(c.identity)
	c.mu.Unlock()
	if len(tokens) == 0 {
		return nil, fail("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401)
	}
	if err := c.assert(identity, c.opts.GetCommander()); err != nil {
		c.mu.Lock()
		c.mismatch = true
		c.mu.Unlock()
		return nil, err
	}
	if v.Num(tokens["expires"]) < float64(c.opts.Now().Add(time.Minute).UnixMilli()) {
		if err := c.refresh(ctx, v.String(tokens, "accessToken")); err != nil {
			return nil, err
		}
	}
	query := func() (v.M, string, error) {
		c.mu.Lock()
		token := v.String(c.tokens, "accessToken")
		c.mu.Unlock()
		if token == "" {
			return nil, "", fail("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401)
		}
		timeout := 15 * time.Second
		if endpoint == "fleetcarrier" {
			timeout = time.Minute
		}
		data, err := c.request(ctx, c.opts.CAPIURL+"/"+endpoint+"?language=ru", "GET", nil, map[string]string{"Authorization": "Bearer " + token}, timeout)
		return data, token, err
	}
	data, token, err := query()
	var issue *Error
	if errors.As(err, &issue) && issue.Code == "UNAUTHORIZED" {
		if err = c.refresh(ctx, token); err != nil {
			return nil, err
		}
		data, _, err = query()
	}
	return data, err
}
func (c *Client) Sync(ctx context.Context, carrier bool) (v.M, error) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	if c.tokens == nil {
		c.mu.Unlock()
		return nil, fail("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401)
	}
	epoch := c.epoch
	c.syncing = true
	c.mu.Unlock()
	c.notify()
	defer func() { c.mu.Lock(); c.syncing = false; c.mu.Unlock(); c.notify() }()
	data, issues, observed, requested := v.M{}, v.M{}, v.M{}, v.M{}
	current := c.opts.Now()
	c.mu.Lock()
	profileDue := c.endpointTimes["profile"].IsZero() || !current.Before(c.endpointTimes["profile"].Add(time.Minute))
	if profileDue {
		c.endpointTimes["profile"] = current
	}
	c.mu.Unlock()
	if profileDue {
		profile, err := c.capi(ctx, "profile")
		if err != nil {
			c.mu.Lock()
			if epoch == c.epoch {
				c.lastError = err.Error()
			}
			c.mu.Unlock()
			return nil, err
		}
		if v.Str(v.At(profile, "commander", "name")) == "" {
			return nil, fail("Frontier не вернул профиль командира.", "PROFILE_INVALID", 502)
		}
		c.mu.Lock()
		identity := v.Copy(c.identity)
		c.mu.Unlock()
		identity["name"] = v.At(profile, "commander", "name")
		if err = c.assert(identity, c.opts.GetCommander()); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if epoch != c.epoch {
			c.mu.Unlock()
			return nil, fail("Синхронизация Frontier отменена.", "CANCELLED", 409)
		}
		c.identity = identity
		c.mismatch = false
		c.summary = Summarise(profile, nil, false, c.summary)
		c.lastSync = v.ISO(c.opts.Now())
		c.mu.Unlock()
		data["profile"] = profile
		observed["profile"] = v.ISO(c.opts.Now())
		if v.Bool(v.At(profile, "commander", "docked")) {
			for _, endpoint := range []string{"market", "shipyard"} {
				station, err := c.capi(ctx, endpoint)
				if err == nil && v.Str(v.At(profile, "lastStarport", "id")) != "" && v.Str(v.At(profile, "lastStarport", "id")) != v.Str(v.First(station["id"], v.At(station, "lastStarport", "id"))) {
					err = fail("Данные Frontier относятся к другой станции.", "STATION_MISMATCH", 400)
				}
				if err != nil {
					issues[endpoint] = err.Error()
				} else {
					data[endpoint] = station
					observed[endpoint] = v.ISO(c.opts.Now())
				}
			}
		}
	}
	c.mu.Lock()
	carrierDue := carrier && (c.endpointTimes["carrier"].IsZero() || !current.Before(c.endpointTimes["carrier"].Add(15*time.Minute)))
	if carrierDue {
		c.endpointTimes["carrier"] = current
	}
	c.mu.Unlock()
	if carrierDue {
		requested["carrier"] = v.ISO(c.opts.Now())
		record, err := c.capi(ctx, "fleetcarrier")
		if err == nil && record != nil && v.Str(v.At(record, "name", "callsign")) == "" {
			err = fail("Frontier не вернул данные флотоносца.", "CARRIER_INVALID", 502)
		}
		if err != nil {
			issues["carrier"] = err.Error()
		} else {
			c.mu.Lock()
			if epoch == c.epoch {
				c.summary = Summarise(nil, record, true, c.summary)
				c.lastCarrierSync = v.ISO(c.opts.Now())
			}
			c.mu.Unlock()
			data["carrier"] = record
			observed["carrier"] = v.ISO(c.opts.Now())
		}
		// Squadron data is optional and separate from the personal carrier.
		// Never use bank.commodities as the physical squadron carrier hold.
		requested["squadron"] = v.ISO(c.opts.Now())
		squadron, squadronErr := c.capi(ctx, "squadron")
		if squadronErr != nil {
			issues["squadron"] = squadronErr.Error()
		} else {
			data["squadron"] = squadron
			observed["squadron"] = v.ISO(c.opts.Now())
		}
	}
	c.mu.Lock()
	if epoch != c.epoch {
		c.mu.Unlock()
		return nil, fail("Синхронизация Frontier отменена.", "CANCELLED", 409)
	}
	messages := []string{}
	for _, issue := range issues {
		messages = append(messages, v.Str(issue))
	}
	c.lastError = strings.Join(messages, " ")
	c.mu.Unlock()
	return v.M{"status": c.Status(), "data": data, "errors": issues, "metadata": v.M{"source": "Frontier CAPI", "observedAt": observed, "requestStartedAt": requested}}, nil
}
func (c *Client) Logout() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	c.tokens = nil
	c.pending = nil
	c.identity = nil
	c.summary = v.M{}
	c.lastError = ""
	c.lastSync = ""
	c.lastCarrierSync = ""
	c.endpointTimes = map[string]time.Time{}
	// A tombstone prevents a later launch from re-importing the intentionally
	// preserved 1.x credentials after an explicit logout in the Go application.
	plain, _ := json.Marshal(v.M{"clientId": ClientID, "loggedOut": true})
	protected, err := c.opts.Encrypt(plain)
	if err != nil {
		return errors.New("Не удалось безопасно сохранить выход Frontier.")
	}
	if err = securestore.AtomicWrite(c.tokenFile(), protected); err != nil {
		return errors.New("Не удалось безопасно сохранить выход Frontier.")
	}
	c.notify()
	return nil
}
func carrierName(raw string) string {
	if len(raw) > 0 && len(raw)%2 == 0 {
		if b, err := hex.DecodeString(raw); err == nil && utf8.Valid(b) {
			return strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", ""))
		}
	}
	return raw
}
func rowsOrObject(raw any) []v.M {
	rows := v.Rows(raw)
	if rows != nil {
		return rows
	}
	out := []v.M{}
	for _, x := range v.Obj(raw) {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func Summarise(profile, carrier v.M, carrierPresent bool, previous v.M) v.M {
	out := v.Copy(previous)
	if profile != nil {
		ship := v.Obj(profile["ship"])
		cargo := 0.0
		for _, row := range rowsOrObject(ship["cargo"]) {
			cargo += v.Num(v.First(row["qty"], row["quantity"]))
		}
		out["profile"] = v.Merge(v.M{"commander": v.Str(v.At(profile, "commander", "name")), "credits": v.At(profile, "commander", "credits"), "debt": v.At(profile, "commander", "debt"), "docked": v.At(profile, "commander", "docked"), "system": v.At(profile, "lastSystem", "name"), "station": v.At(profile, "lastStarport", "name"), "marketId": v.At(profile, "lastStarport", "id"), "cargoCount": cargo}, ships.CAPI(ship))
	}
	if carrierPresent {
		if carrier == nil {
			out["carrier"] = v.M{"owned": false}
		} else {
			names, capacity, finance := v.Obj(carrier["name"]), v.Obj(carrier["capacity"]), v.Obj(carrier["finance"])
			groups := map[string]v.M{}
			for _, row := range rowsOrObject(carrier["cargo"]) {
				name, count := v.String(row, "commodity"), v.Num(row["qty"])
				if name == "" || count <= 0 {
					continue
				}
				key := fmt.Sprint(name, "/", row["stolen"], "/", row["mission"])
				if groups[key] == nil {
					groups[key] = v.M{"name": name, "localized": row["locName"], "count": 0.0, "stolen": v.Bool(row["stolen"]), "mission": v.Bool(row["mission"])}
				}
				groups[key]["count"] = v.Num(groups[key]["count"]) + count
			}
			cargo := []v.M{}
			for _, row := range groups {
				cargo = append(cargo, row)
			}
			result := v.M{"owned": true, "callsign": names["callsign"], "name": carrierName(v.Text(names["filteredVanityName"], names["vanityName"], names["callsign"])), "system": v.Text(carrier["currentStarSystem"], v.At(carrier, "currentStarSystem", "name")), "fuel": carrier["fuel"], "balance": v.First(finance["bankBalance"], carrier["balance"]), "reservedBalance": finance["bankReservedBalance"], "maintenance": finance["maintenance"], "coreCost": finance["coreCost"], "servicesCost": finance["servicesCost"], "jumpCost": finance["jumpsCost"], "state": carrier["state"], "dockingAccess": carrier["dockingAccess"], "notoriousAccess": v.Bool(carrier["notoriousAccess"]), "freeSpace": capacity["freeSpace"], "crewSpace": capacity["crew"], "shipPackSpace": capacity["shipPacks"], "modulePackSpace": capacity["modulePacks"], "cargoSpace": v.Num(capacity["cargoForSale"]) + v.Num(capacity["cargoNotForSale"]), "reservedSpace": capacity["cargoSpaceReserved"], "cargo": cargo, "currentJump": v.At(carrier, "itinerary", "currentJump"), "totalDistance": v.At(carrier, "itinerary", "totalDistanceJumpedLY")}
			if _, ok := v.Number(capacity["freeSpace"]); ok {
				result["usedCapacity"] = 25000 - v.Num(capacity["freeSpace"])
			}
			for _, direction := range []string{"sales", "purchases"} {
				orders := []v.M{}
				for _, row := range rowsOrObject(v.At(carrier, "orders", "commodities", direction)) {
					orders = append(orders, v.M{"name": row["name"], "count": v.First(row["stock"], row["total"]), "price": row["price"], "outstanding": row["outstanding"], "blackmarket": v.Bool(row["blackmarket"])})
				}
				result[direction] = orders
			}
			services := []v.M{}
			for key, status := range v.Obj(v.At(carrier, "market", "services")) {
				services = append(services, v.M{"name": key, "status": status})
			}
			result["services"] = services
			out["carrier"] = result
		}
	}
	return out
}
func MarketSnapshot(result v.M) v.M {
	status, data, meta := v.Obj(result["status"]), v.Obj(result["data"]), v.Obj(result["metadata"])
	profile, market := v.Obj(data["profile"]), v.Obj(data["market"])
	observed := v.At(meta, "observedAt", "market")
	if !v.Bool(status["connected"]) || v.Bool(status["commanderMismatch"]) || !v.Bool(v.At(profile, "commander", "docked")) || !strings.EqualFold(v.Str(v.At(profile, "commander", "name")), v.String(status, "commander")) || v.Int(market["id"]) <= 0 || v.Str(market["id"]) != v.Str(v.At(profile, "lastStarport", "id")) || v.String(meta, "source") != "Frontier CAPI" || v.Time(observed).IsZero() {
		return nil
	}
	items := []v.M{}
	seen := map[string]bool{}
	for _, row := range v.Rows(market["commodities"]) {
		name := strings.ToLower(v.String(row, "name"))
		if name == "" || seen[name] || v.Bool(row["legality"]) {
			continue
		}
		valid := true
		for _, key := range []string{"buyPrice", "sellPrice", "stock", "demand", "stockBracket", "demandBracket"} {
			n, ok := v.Number(row[key])
			if !ok || n < 0 {
				valid = false
			}
			if strings.HasSuffix(key, "Bracket") && (n > 3 || n != float64(int(n))) {
				valid = false
			}
			if (key == "stock" || key == "demand") && n != float64(int64(n)) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		seen[name] = true
		stock, demand := row["stock"], row["demand"]
		if v.Num(row["stockBracket"]) == 0 {
			stock = 0
		}
		if v.Num(row["demandBracket"]) == 0 {
			demand = 0
		}
		items = append(items, v.M{"Name": name, "Name_Localised": row["locName"], "BuyPrice": row["buyPrice"], "SellPrice": row["sellPrice"], "Stock": stock, "Demand": demand})
	}
	return v.M{"event": "Market", "timestamp": observed, "MarketID": market["id"], "StationName": market["name"], "StarSystem": v.At(profile, "lastSystem", "name"), "Items": items, "Source": "Frontier CAPI"}
}
