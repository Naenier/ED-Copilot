package inara

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Naenier/ED-Copilot/internal/securestore"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Identity struct {
	Name string `json:"name"`
	FID  string `json:"fid"`
}
type Config struct {
	Enabled bool      `json:"enabled"`
	APIKey  string    `json:"apiKey"`
	Bound   *Identity `json:"boundCommander"`
}
type QueueRow struct {
	Key         string   `json:"key"`
	Service     string   `json:"service"`
	Identity    Identity `json:"identity"`
	Payload     Event    `json:"payload"`
	Replace     string   `json:"replace"`
	Timestamp   string   `json:"timestamp"`
	Attempts    int      `json:"attempts"`
	NextAttempt int64    `json:"nextAttemptAt"`
	EnqueuedAt  int64    `json:"enqueuedAt"`
}
type Service struct {
	Sent        int    `json:"sent"`
	Failed      int    `json:"failed"`
	LastSuccess string `json:"lastSuccess"`
	LastError   string `json:"lastError"`
	RetryAt     int64  `json:"retryAt"`
}
type Options struct {
	DataDir, Endpoint, AppVersion string
	HTTPClient                    *http.Client
	GetState                      func() v.M
	OnUpdate                      func()
	Now                           func() time.Time
	Encrypt, Decrypt              func([]byte) ([]byte, error)
	AutoDelay                     time.Duration
	Send                          func(context.Context, SendOptions) (BatchResult, error)
}
type Manager struct {
	mu, runMu                                    sync.Mutex
	opts                                         Options
	config                                       Config
	queue                                        []QueueRow
	delivered                                    []string
	cursors                                      map[string]int64
	service                                      Service
	current, game                                v.M
	enabledAt                                    int64
	storageError                                 string
	available, closed, busy, suppressDock, dirty bool
	epoch                                        uint64
	cancel                                       context.CancelFunc
	stop, done                                   chan struct{}
	lastSync                                     string
	lastDrain                                    time.Time
	shipType                                     string
	shipID, loan                                 any
}

func clean(raw string, max int) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw)
	rows := []rune(s)
	if len(rows) > max {
		rows = rows[:max]
	}
	return string(rows)
}
func identity(raw v.M) Identity {
	return Identity{clean(v.String(raw, "name"), 180), clean(v.String(raw, "fid"), 40)}
}
func same(a Identity, b *Identity) bool {
	return b != nil && a.FID != "" && a.Name != "" && a.FID == b.FID && a.Name == b.Name
}
func New(o Options) (*Manager, error) {
	if o.DataDir == "" {
		return nil, errors.New("Не задан каталог профиля Inara.")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.GetState == nil {
		o.GetState = func() v.M { return v.M{} }
	}
	if o.AutoDelay <= 0 {
		o.AutoDelay = 20 * time.Second
	}
	if o.AppVersion == "" {
		o.AppVersion = "2.0.0"
	}
	store := securestore.New(o.DataDir)
	available := o.Encrypt != nil && o.Decrypt != nil || store.Available()
	if o.Encrypt == nil {
		o.Encrypt = store.Encrypt
	}
	if o.Decrypt == nil {
		o.Decrypt = store.Decrypt
	}
	if o.Send == nil {
		o.Send = SendBatch
	}
	m := &Manager{opts: o, cursors: map[string]int64{}, current: v.M{}, game: v.M{}, available: available, stop: make(chan struct{}), done: make(chan struct{}), lastDrain: o.Now()}
	m.load()
	go m.worker()
	return m, nil
}
func (m *Manager) file() string { return filepath.Join(m.opts.DataDir, "integration-go-state.bin") }
func (m *Manager) load() {
	blob, err := os.ReadFile(m.file())
	if os.IsNotExist(err) {
		blob, err = os.ReadFile(filepath.Join(m.opts.DataDir, "integration-state.bin"))
	}
	if os.IsNotExist(err) {
		return
	}
	if err != nil || !m.available {
		m.storageError = "Защищённое хранилище Windows недоступно."
		return
	}
	plain, err := m.opts.Decrypt(blob)
	if err != nil {
		m.storageError = "Не удалось открыть защищённые данные. Используйте тот же профиль Windows."
		return
	}
	defer clear(plain)
	var saved struct {
		Version   int              `json:"version"`
		Config    json.RawMessage  `json:"config"`
		Queue     []QueueRow       `json:"queue"`
		Delivered []string         `json:"delivered"`
		Cursors   map[string]int64 `json:"cursors"`
		Service   Service          `json:"service"`
		Services  struct {
			Inara Service `json:"inara"`
		} `json:"services"`
		EnabledAt int64 `json:"enabledAt"`
	}
	if json.Unmarshal(plain, &saved) != nil || saved.Version < 1 || saved.Version > 3 {
		m.storageError = "Не удалось открыть сохранённую очередь Inara."
		return
	}
	if saved.Version == 1 {
		var old struct {
			Publisher string    `json:"publisher"`
			Enabled   bool      `json:"inaraEnabled"`
			APIKey    string    `json:"inaraApiKey"`
			Bound     *Identity `json:"boundCommander"`
		}
		if json.Unmarshal(saved.Config, &old) != nil {
			m.storageError = "Не удалось открыть настройки Inara."
			return
		}
		m.config = Config{old.Publisher == "copilot" && old.Enabled, old.APIKey, old.Bound}
		m.service = saved.Services.Inara
	} else {
		if json.Unmarshal(saved.Config, &m.config) != nil {
			m.storageError = "Не удалось открыть настройки Inara."
			return
		}
		m.service = saved.Service
	}
	if len(m.config.APIKey) > 2048 || strings.ContainsAny(m.config.APIKey, "\x00\r\n") {
		m.config.APIKey = ""
		m.config.Enabled = false
	}
	for _, row := range saved.Queue {
		if row.Service == "inara" && row.Key != "" && row.Identity.FID != "" && Supported[row.Payload.Name] {
			m.queue = append(m.queue, row)
		}
	}
	if len(m.queue) > 1000 {
		m.queue = m.queue[len(m.queue)-1000:]
	}
	for _, key := range saved.Delivered {
		if strings.HasPrefix(key, "inara:") {
			m.delivered = append(m.delivered, key)
		}
	}
	if len(m.delivered) > 4000 {
		m.delivered = m.delivered[len(m.delivered)-4000:]
	}
	if saved.Cursors != nil {
		m.cursors = saved.Cursors
	}
	m.enabledAt = saved.EnabledAt
	m.dirty = true
}
func (m *Manager) persistLocked() {
	if !m.available || m.storageError != "" {
		return
	}
	saved := struct {
		Version   int              `json:"version"`
		Config    Config           `json:"config"`
		Queue     []QueueRow       `json:"queue"`
		Delivered []string         `json:"delivered"`
		Cursors   map[string]int64 `json:"cursors"`
		Service   Service          `json:"service"`
		EnabledAt int64            `json:"enabledAt"`
	}{3, m.config, m.queue, m.delivered, m.cursors, m.service, m.enabledAt}
	plain, err := json.Marshal(saved)
	if err == nil {
		var blob []byte
		blob, err = m.opts.Encrypt(plain)
		clear(plain)
		if err == nil {
			err = securestore.AtomicWrite(m.file(), blob)
		}
	}
	if err != nil {
		m.storageError = "Не удалось сохранить защищённую очередь. Отправка приостановлена."
	} else {
		m.dirty = false
	}
}
func (m *Manager) notify() {
	if m.opts.OnUpdate != nil {
		go m.opts.OnUpdate()
	}
}
func (m *Manager) metadataLocked() v.M {
	meta := v.Copy(m.game)
	v.Merge(meta, v.Obj(m.current["game"]))
	meta["gameVersion"] = v.Text(meta["version"], meta["gameversion"], meta["gameVersion"])
	return meta
}
func (m *Manager) cacheLocked(state v.M) {
	before := identity(v.Obj(m.current["commander"]))
	for _, key := range []string{"commander", "game", "connection", "app", "pilot", "engineering"} {
		if raw, ok := state[key]; ok {
			m.current[key] = v.Copy(v.Obj(raw))
		}
	}
	after := identity(v.Obj(m.current["commander"]))
	if before.FID != "" && after.FID != "" && (before.FID != after.FID || before.Name != after.Name) {
		m.epoch++
		if m.cancel != nil {
			m.cancel()
		}
	}
}
func (m *Manager) refresh() {
	state := m.opts.GetState()
	m.mu.Lock()
	if !m.closed {
		m.cacheLocked(state)
	}
	m.mu.Unlock()
}
func (m *Manager) eligibleLocked() bool {
	return !m.closed && m.available && m.storageError == "" && m.config.Enabled && m.config.APIKey != "" && same(identity(v.Obj(m.current["commander"])), m.config.Bound) && !v.Bool(v.At(m.current, "connection", "scanning")) && ValidateLive(m.metadataLocked()).OK
}
func (m *Manager) contextLocked() v.M {
	commander := v.Obj(m.current["commander"])
	out := m.metadataLocked()
	v.Merge(out, v.M{"commander": commander, "state": m.current, "shipType": v.Text(commander["shipType"], m.shipType), "shipId": v.First(commander["shipId"], m.shipID), "loan": v.First(v.At(m.current, "game", "loan"), m.loan), "cargoIsShip": strings.EqualFold(v.String(commander, "cargoVessel"), "ship"), "locationUpdatedAt": commander["locationUpdatedAt"], "suppressDock": m.suppressDock})
	return out
}
func (m *Manager) Status(contexts ...v.M) v.M {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, state := range contexts {
		m.cacheLocked(state)
	}
	return m.statusLocked()
}
func (m *Manager) statusLocked() v.M {
	live := ValidateLive(m.metadataLocked())
	current := identity(v.Obj(m.current["commander"]))
	reason := ""
	switch {
	case !m.config.Enabled:
		reason = "Отправка выключена"
	case m.storageError != "":
		reason = m.storageError
	case !m.available:
		reason = "Требуется защищённое хранилище Windows"
	case m.config.APIKey == "":
		reason = "Сохраните личный API-ключ Inara"
	case m.config.Bound == nil || current.FID == "":
		reason = "Ожидаем командира из журнала игры"
	case !same(current, m.config.Bound):
		reason = "В игре выбран другой командир"
	case v.Bool(v.At(m.current, "connection", "scanning")):
		reason = "Читаем журналы игры"
	case !live.OK:
		reason = live.Reason
	}
	var retry any
	soon := int64(0)
	now := m.opts.Now().UnixMilli()
	for _, row := range m.queue {
		at := row.NextAttempt
		if at < m.service.RetryAt {
			at = m.service.RetryAt
		}
		if at > now && (soon == 0 || at < soon) {
			soon = at
		}
	}
	if soon > 0 {
		retry = v.ISO(time.UnixMilli(soon))
	}
	var bound any
	if m.config.Bound != nil {
		bound = v.M{"name": m.config.Bound.Name, "fid": m.config.Bound.FID}
	}
	return v.M{"enabled": m.config.Enabled, "apiKeySet": m.config.APIKey != "", "storageAvailable": m.available, "storageError": nullable(m.storageError), "commander": v.M{"name": current.Name, "fid": current.FID}, "boundCommander": bound, "identityMatches": same(current, m.config.Bound), "live": v.M{"ok": live.OK, "reason": live.Reason}, "blockedReason": reason, "sent": m.service.Sent, "failed": m.service.Failed, "lastSuccess": nullable(m.service.LastSuccess), "lastError": nullable(displayError(m.service.LastError, m.config.APIKey)), "retryAt": m.service.RetryAt, "queued": len(m.queue), "nextRetryAt": retry, "busy": m.busy, "lastSync": nullable(m.lastSync)}
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (m *Manager) Configure(body v.M) error {
	m.refresh()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("Отправка Inara закрыта.")
	}
	next := m.config
	if raw, ok := body["enabled"]; ok {
		enabled, valid := raw.(bool)
		if !valid {
			return errors.New("Некорректная настройка отправки.")
		}
		next.Enabled = enabled
	}
	if raw, ok := body["apiKey"]; ok {
		key, valid := raw.(string)
		if !valid || len(key) > 2048 || strings.ContainsAny(key, "\r\n\x00") {
			return errors.New("Некорректный API-ключ Inara.")
		}
		if strings.TrimSpace(key) != "" {
			next.APIKey = strings.TrimSpace(key)
		}
	}
	if v.Bool(body["clearApiKey"]) {
		next.APIKey = ""
	}
	if (next.Enabled || next.APIKey != "") && (!m.available || m.storageError != "") {
		return errors.New("Защищённое хранилище Windows недоступно.")
	}
	current := identity(v.Obj(m.current["commander"]))
	if next.Enabled && (current.Name == "" || current.FID == "") {
		return errors.New("Откройте игру: сначала нужно прочитать имя командира.")
	}
	if next.Enabled && next.Bound != nil && !same(current, next.Bound) && !v.Bool(body["rebindCommander"]) {
		return errors.New("Ключ привязан к другому командиру. Отправка приостановлена.")
	}
	if next.Enabled && (next.Bound == nil || v.Bool(body["rebindCommander"])) {
		copy := current
		next.Bound = &copy
	}
	if v.Bool(body["rebindCommander"]) {
		m.queue = nil
	}
	if next.APIKey != m.config.APIKey {
		for i := range m.queue {
			m.queue[i].Attempts = 0
			m.queue[i].NextAttempt = 0
		}
		m.service.RetryAt = 0
		m.service.LastError = ""
	}
	if !m.config.Enabled && next.Enabled {
		m.enabledAt = m.opts.Now().UnixMilli()
	}
	m.config = next
	m.epoch++
	if m.cancel != nil {
		m.cancel()
	}
	m.persistLocked()
	m.notify()
	return nil
}
func (m *Manager) enqueueLocked(event Event) {
	if !m.eligibleLocked() || !validTime(event.Timestamp, m.opts.Now()) {
		return
	}
	id := identity(v.Obj(m.current["commander"]))
	key := "inara:" + EventKey(event, id.FID)
	for _, done := range m.delivered {
		if done == key {
			return
		}
	}
	replace := SnapshotKey(event)
	for _, row := range m.queue {
		if row.Key == key || replace != "" && row.Replace == replace && row.Identity.FID == id.FID && v.Time(row.Timestamp).After(v.Time(event.Timestamp)) {
			return
		}
	}
	if replace != "" {
		next := m.queue[:0]
		for _, row := range m.queue {
			if row.Replace == replace && row.Identity.FID == id.FID && !v.Time(row.Timestamp).After(v.Time(event.Timestamp)) {
				continue
			}
			next = append(next, row)
		}
		m.queue = next
	}
	m.queue = append(m.queue, QueueRow{Key: key, Service: "inara", Identity: id, Payload: event, Replace: replace, Timestamp: event.Timestamp, EnqueuedAt: m.opts.Now().UnixMilli()})
	for {
		encoded, _ := json.Marshal(m.queue)
		if len(m.queue) <= 1000 && len(encoded) <= 8*1024*1024 {
			break
		}
		m.queue = m.queue[1:]
		m.service.Failed++
		m.service.LastError = "Очередь переполнена: старые неподтверждённые наблюдения пропущены."
	}
	m.dirty = true
}
func (m *Manager) ApplyJournal(event, meta v.M) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if state := v.Obj(meta["state"]); len(state) > 0 {
		m.cacheLocked(state)
	} else if meta["commander"] != nil {
		m.cacheLocked(v.M{"commander": meta["commander"], "game": meta["game"], "connection": meta["connection"]})
	}
	typ := v.String(event, "event")
	if typ == "Fileheader" {
		version := v.String(event, "gameversion")
		m.game["version"] = version
		m.game["build"] = event["build"]
		m.game["isBeta"] = strings.Contains(strings.ToLower(version), "beta") || strings.Contains(strings.ToLower(version), "alpha")
		m.game["live"] = ValidateLive(v.M{"gameVersion": version}).OK
	}
	if typ == "LoadGame" {
		for _, field := range []string{"Horizons", "Odyssey"} {
			if raw, ok := event[field]; ok {
				m.game[strings.ToLower(field)] = raw
			}
		}
		if loan, ok := integer(event["Loan"]); ok {
			m.loan = loan
		} else {
			m.loan = nil
		}
		commander := v.Copy(v.Obj(m.current["commander"]))
		if v.String(event, "FID") != "" {
			commander["fid"] = event["FID"]
		}
		if name := v.Text(event["Commander"], event["Name"]); name != "" {
			commander["name"] = name
		}
		m.current["commander"] = commander
	}
	if typ == "Loadout" {
		m.shipType = v.String(event, "Ship")
		m.shipID = event["ShipID"]
	}
	if contains([]string{"JoinACrew", "CrewJoin"}, typ) {
		m.game["multicrew"] = true
	}
	if contains([]string{"QuitACrew", "CrewQuit", "LoadGame"}, typ) {
		m.game["multicrew"] = false
	}
	file := clean(v.String(meta, "file"), 2048)
	offset, ok := integer(meta["offset"])
	if file == "" || !ok {
		return
	}
	previous, exists := m.cursors[file]
	if !exists || int64(offset) > previous {
		m.cursors[file] = int64(offset)
	}
	m.dirty = true
	if v.Bool(meta["replay"]) || exists && int64(offset) <= previous || v.Time(event["timestamp"]).UnixMilli() < m.enabledAt-5000 {
		return
	}
	for _, payload := range JournalEvents(event, m.contextLocked(), m.opts.Now()) {
		m.enqueueLocked(payload)
	}
	switch typ {
	case "Location":
		m.suppressDock = v.Bool(event["Docked"])
	case "Docked", "FSDJump", "Undocked":
		m.suppressDock = false
	case "CarrierJump":
		m.suppressDock = true
	}
	m.notify()
}
func (m *Manager) ApplySide(event, meta v.M) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || v.Bool(meta["replay"]) || v.Time(event["timestamp"]).UnixMilli() < m.enabledAt-5000 {
		return
	}
	if state := v.Obj(meta["state"]); len(state) > 0 {
		m.cacheLocked(state)
	} else if meta["commander"] != nil {
		m.cacheLocked(v.M{"commander": meta["commander"], "game": meta["game"], "connection": meta["connection"]})
	}
	for _, payload := range JournalEvents(event, m.contextLocked(), m.opts.Now()) {
		m.enqueueLocked(payload)
	}
	m.notify()
}
func (m *Manager) ApplyCAPI(result v.M) {
	m.refresh()
	m.mu.Lock()
	defer m.mu.Unlock()
	if v.Str(v.At(result, "metadata", "source")) != "Frontier CAPI" {
		return
	}
	profile := v.Obj(v.At(result, "data", "profile"))
	current := identity(v.Obj(m.current["commander"]))
	if v.Str(v.At(profile, "commander", "name")) != current.Name || !same(current, m.config.Bound) {
		return
	}
	debt, loanOK := integer(v.At(profile, "commander", "debt"))
	credits, creditsOK := integer(v.At(profile, "commander", "credits"))
	if loanOK {
		m.loan = debt
	}
	if creditsOK {
		if loan, ok := integer(m.loan); ok {
			m.enqueueLocked(Event{"setCommanderCredits", v.Str(v.At(result, "metadata", "observedAt", "profile")), v.M{"commanderCredits": credits, "commanderLoan": loan}})
		}
	}
	m.notify()
}

// ObserveCAPI accepts only a verified Frontier profile observation; carrier
// warehouse/fuel/balance are deliberately not mapped to ship inventory.
func (m *Manager) ObserveCAPI(result v.M) { m.ApplyCAPI(result) }
func (m *Manager) retryLocked(row QueueRow, message string, delay time.Duration) {
	for i := range m.queue {
		if m.queue[i].Key != row.Key {
			continue
		}
		m.queue[i].Attempts++
		exponent := m.queue[i].Attempts - 1
		if exponent > 6 {
			exponent = 6
		}
		backoff := time.Minute * time.Duration(1<<exponent)
		if backoff > time.Hour {
			backoff = time.Hour
		}
		if delay > backoff {
			backoff = delay
		}
		m.queue[i].NextAttempt = m.opts.Now().Add(backoff).UnixMilli()
		if m.service.RetryAt < m.queue[i].NextAttempt {
			m.service.RetryAt = m.queue[i].NextAttempt
		}
		m.service.LastError = clean(message, 500)
		m.dirty = true
		return
	}
}
func (m *Manager) finishLocked(row QueueRow, ok bool, message string) {
	next := m.queue[:0]
	for _, queued := range m.queue {
		if queued.Key != row.Key {
			next = append(next, queued)
		}
	}
	m.queue = next
	m.delivered = append(m.delivered, row.Key)
	if len(m.delivered) > 4000 {
		m.delivered = m.delivered[len(m.delivered)-4000:]
	}
	if ok {
		m.service.Sent++
		m.service.LastSuccess = v.ISO(m.opts.Now())
		pendingRetry := false
		for _, queued := range m.queue {
			if queued.Attempts > 0 {
				pendingRetry = true
				break
			}
		}
		if !pendingRetry {
			m.service.LastError = ""
		}
	} else {
		m.service.Failed++
		m.service.LastError = clean(message, 500)
	}
	m.dirty = true
}
func (m *Manager) Sync(ctx context.Context) error {
	if !m.runMu.TryLock() {
		return nil
	}
	defer m.runMu.Unlock()
	m.refresh()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("Отправка Inara закрыта.")
	}
	if !m.eligibleLocked() {
		m.mu.Unlock()
		return nil
	}
	for _, payload := range BuildSnapshotEvents(m.current, m.contextLocked(), m.opts.Now()) {
		m.enqueueLocked(payload)
	}
	now := m.opts.Now()
	for _, row := range append([]QueueRow(nil), m.queue...) {
		if !validTime(row.Timestamp, now) {
			m.finishLocked(row, false, "Наблюдение устарело и не отправлено.")
		}
	}
	if m.service.RetryAt > now.UnixMilli() {
		m.persistLocked()
		m.mu.Unlock()
		return nil
	}
	batch := []QueueRow{}
	bytes := 0
	for _, row := range m.queue {
		if !same(row.Identity, m.config.Bound) || row.NextAttempt > now.UnixMilli() {
			continue
		}
		payload, _ := json.Marshal(row.Payload)
		if len(batch) > 0 && (len(batch) >= 50 || bytes+len(payload) > 768000) {
			break
		}
		batch = append(batch, row)
		bytes += len(payload)
	}
	if len(batch) == 0 {
		m.persistLocked()
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.busy = true
	epoch := m.epoch
	events := []Event{}
	for _, row := range batch {
		events = append(events, row.Payload)
	}
	bound := *m.config.Bound
	options := SendOptions{HTTPClient: m.opts.HTTPClient, Endpoint: m.opts.Endpoint, CommanderName: bound.Name, FrontierID: bound.FID, APIKey: m.config.APIKey, AppVersion: m.opts.AppVersion, Events: events, Now: m.opts.Now}
	m.persistLocked()
	if m.storageError != "" {
		m.busy = false
		m.cancel = nil
		m.mu.Unlock()
		cancel()
		return errors.New("Не удалось сохранить защищённую очередь.")
	}
	m.mu.Unlock()
	m.notify()
	response, err := m.opts.Send(ctx, options)
	cancel()
	m.refresh()
	m.mu.Lock()
	defer m.mu.Unlock()
	defer m.notify()
	m.busy = false
	m.cancel = nil
	m.lastDrain = m.opts.Now()
	if m.closed || epoch != m.epoch || !m.eligibleLocked() {
		m.persistLocked()
		return nil
	}
	for i, row := range batch {
		if err != nil {
			var issue *Error
			if errors.As(err, &issue) && !issue.Retryable {
				m.finishLocked(row, false, issue.Message)
			} else {
				delay, message := time.Minute, "Не удалось связаться с Inara."
				if issue != nil {
					delay = issue.RetryAfter
					message = issue.Message
				}
				m.retryLocked(row, message, delay)
			}
			continue
		}
		if i >= len(response.Results) {
			m.retryLocked(row, "Не получено подтверждение события.", time.Minute)
			continue
		}
		outcome := response.Results[i]
		if outcome.OK {
			m.finishLocked(row, true, "")
		} else if response.SetupRequired {
			m.retryLocked(row, v.Text(response.Message, outcome.Message), time.Hour)
		} else if outcome.Retryable {
			m.retryLocked(row, outcome.Message, time.Minute)
		} else {
			m.finishLocked(row, false, outcome.Message)
		}
	}
	m.lastSync = v.ISO(m.opts.Now())
	m.persistLocked()
	return err
}
func (m *Manager) worker() {
	defer close(m.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-tick.C:
			m.mu.Lock()
			shouldRefresh := m.config.Enabled && m.config.APIKey != ""
			dirty := m.dirty
			m.mu.Unlock()
			if shouldRefresh {
				m.refresh()
			}
			m.mu.Lock()
			ready := len(m.queue) > 0 && m.eligibleLocked() && m.opts.Now().Sub(m.lastDrain) >= m.opts.AutoDelay
			if dirty && m.dirty {
				m.persistLocked()
			}
			m.mu.Unlock()
			if ready {
				_ = m.Sync(context.Background())
			}
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.epoch++
	if m.cancel != nil {
		m.cancel()
	}
	m.persistLocked()
	close(m.stop)
	m.mu.Unlock()
	<-m.done
}
