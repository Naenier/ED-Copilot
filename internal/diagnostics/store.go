// Package diagnostics stores local application diagnostics in rotating JSON lines.
package diagnostics

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Level string

const (
	Debug Level = "debug"
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"

	weeksToKeep     = 4
	segmentsPerWeek = 4
	segmentBytes    = 2 * 1024 * 1024
	lineBytes       = 4096
	defaultLimit    = 100
	maxLimit        = 250
)

var (
	ErrUnavailable = errors.New("Журнал приложения недоступен.")
	ErrFilter      = errors.New("Проверьте фильтры журнала приложения.")
	filePattern    = regexp.MustCompile(`^app-(\d{4}-\d{2}-\d{2})-(\d{1,10})\.jsonl$`)
	labelPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
)

type Entry struct {
	ID        string    `json:"id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Level     Level     `json:"level"`
	Subsystem string    `json:"subsystem"`
	Event     string    `json:"event"`
	Message   string    `json:"message"`
}

type Filter struct {
	Level     Level
	Subsystem string
	Search    string
	MatchText func(Entry) string
	Before    string
	Limit     int
}

type Page struct {
	Entries    []Entry  `json:"entries"`
	Next       string   `json:"next"`
	Subsystems []string `json:"subsystems"`
}

type segment struct {
	week     string
	index    int
	size     int64
	modified int64
}

func (s segment) name() string {
	return fmt.Sprintf("app-%s-%03d.jsonl", s.week, s.index)
}

func compareSegments(a, b segment) int {
	if a.week != b.week {
		return strings.Compare(a.week, b.week)
	}
	return cmp.Compare(a.index, b.index)
}

func parseSegment(name string) (segment, bool) {
	match := filePattern.FindStringSubmatch(name)
	if match == nil {
		return segment{}, false
	}
	date, err := time.Parse(time.DateOnly, match[1])
	if err != nil || date.Weekday() != time.Monday {
		return segment{}, false
	}
	index, err := strconv.Atoi(match[2])
	item := segment{week: match[1], index: index}
	return item, err == nil && item.name() == name
}

// Store serializes appends. It holds no open files between calls.
type Store struct {
	dir string
	now func() time.Time

	mu      sync.Mutex
	active  segment
	failure bool
	retryAt time.Time
	index   map[segment][]string
}

func New(dir string) *Store {
	return &Store{dir: dir, now: time.Now}
}

func monday(now time.Time) time.Time {
	day := now.UTC().Truncate(24 * time.Hour)
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}

func validLevel(level Level) bool {
	switch level {
	case Debug, Info, Warn, Error:
		return true
	default:
		return false
	}
}

func safeMessage(message string) string {
	var out strings.Builder
	space := false
	for _, char := range strings.TrimSpace(message) {
		if unicode.IsControl(char) || unicode.IsSpace(char) {
			space = true
			continue
		}
		if out.Len() >= 600 {
			break
		}
		if space {
			out.WriteByte(' ')
			space = false
		}
		out.WriteRune(char)
	}
	return out.String()
}

// Log records an application event. Supply fixed, user-safe summaries and
// metadata. Free-form error strings, request bodies and credentials belong
// outside this log. Diagnostics failure never interrupts the operation itself.
func (s *Store) Log(level Level, subsystem, event, message string) {
	if s == nil || s.dir == "" || !validLevel(level) || !labelPattern.MatchString(subsystem) || !labelPattern.MatchString(event) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	if now.Before(s.retryAt) {
		return
	}
	entry := Entry{Timestamp: now, Level: level, Subsystem: subsystem, Event: event, Message: safeMessage(message)}
	line, err := json.Marshal(entry)
	if err == nil {
		err = s.append(monday(now), append(line, '\n'))
	}
	s.failure = err != nil
	if s.failure {
		s.active = segment{}
		s.retryAt = now.Add(time.Second)
	} else {
		s.retryAt = time.Time{}
	}
}

func (s *Store) append(week time.Time, line []byte) error {
	current := week.Format(time.DateOnly)
	cleanup := s.active.week != current
	if cleanup {
		if err := os.MkdirAll(s.dir, 0700); err != nil {
			return err
		}
		files, err := s.segments()
		if err != nil {
			return err
		}
		s.active = segment{week: current}
		for _, file := range files {
			if file.week == current {
				s.active = file
				break
			}
		}
		if err := s.recoverTail(); err != nil {
			return err
		}
	}
	if s.active.size+int64(len(line)) > segmentBytes {
		s.active.index++
		s.active.size = 0
		cleanup = true
	}
	filePath := filepath.Join(s.dir, s.active.name())
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	n, err := file.Write(line)
	err = errors.Join(err, file.Close())
	s.active.size += int64(n)
	if err != nil {
		return err
	}
	if cleanup {
		return s.prune(week)
	}
	return nil
}

// recoverTail removes an incomplete record if the previous process died
// during the most recent append.
func (s *Store) recoverTail() error {
	if s.active.size == 0 || s.active.size > segmentBytes {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(s.dir, s.active.name()), os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	length := min(s.active.size, lineBytes)
	start := s.active.size - length
	tail := make([]byte, length)
	if _, err := file.ReadAt(tail, start); err != nil {
		return err
	}
	if tail[len(tail)-1] == '\n' {
		return nil
	}
	lineEnd := bytes.LastIndexByte(tail, '\n')
	if lineEnd < 0 && start != 0 {
		// Keep earlier records even if a damaged tail exceeds a valid row.
		s.active.size = segmentBytes
		return nil
	}
	s.active.size = start + int64(lineEnd+1)
	return file.Truncate(s.active.size)
}

func (s *Store) segments() ([]segment, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]segment, 0, len(entries))
	for _, entry := range entries {
		item, ok := parseSegment(entry.Name())
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item.size, item.modified = info.Size(), info.ModTime().UnixNano()
		files = append(files, item)
	}
	slices.SortFunc(files, func(a, b segment) int { return compareSegments(b, a) })
	return files, nil
}

func recentSegments(files []segment, week time.Time) (kept, old []segment) {
	cutoff := week.AddDate(0, 0, -7*(weeksToKeep-1)).Format(time.DateOnly)
	count := make(map[string]int, weeksToKeep)
	for _, item := range files {
		if item.week < cutoff || count[item.week] >= segmentsPerWeek || len(kept) >= weeksToKeep*segmentsPerWeek {
			old = append(old, item)
			continue
		}
		count[item.week]++
		kept = append(kept, item)
	}
	return kept, old
}

func (s *Store) prune(week time.Time) error {
	files, err := s.segments()
	if err != nil {
		return err
	}
	_, old := recentSegments(files, week)
	for _, item := range old {
		err := os.Remove(filepath.Join(s.dir, item.name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func parseCursor(value string) (segment, int64, bool) {
	name, offset, ok := strings.Cut(value, ":")
	if !ok {
		return segment{}, 0, false
	}
	item, valid := parseSegment(name)
	position, err := strconv.ParseInt(offset, 10, 64)
	return item, position, valid && err == nil && position >= 0 && position < segmentBytes
}

func (s *Store) Query(filter Filter) (Page, error) {
	page := Page{Entries: []Entry{}, Subsystems: []string{}}
	if s == nil || s.dir == "" {
		return page, nil
	}
	before, offset, cursorOK := parseCursor(filter.Before)
	if filter.Level != "" && !validLevel(filter.Level) ||
		filter.Subsystem != "" && !labelPattern.MatchString(filter.Subsystem) ||
		filter.Before != "" && !cursorOK || len(filter.Search) > 512 {
		return page, ErrFilter
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	query := strings.ToLower(strings.TrimSpace(filter.Search))
	s.mu.Lock()
	files, err := s.segments()
	failed, week := s.failure, monday(s.now())
	index := s.index
	s.mu.Unlock()
	if err != nil || failed && len(files) == 0 {
		return page, ErrUnavailable
	}
	files, _ = recentSegments(files, week)
	subsystems := make(map[string]bool)
	nextIndex := make(map[segment][]string, len(files))
	for _, file := range files {
		names, indexed := index[file]
		if indexed {
			nextIndex[file] = names
			for _, name := range names {
				subsystems[name] = true
			}
		}
		rank := compareSegments(file, before)
		readEntries := page.Next == "" && (!cursorOK || rank < 0 || rank == 0 && offset != 0)
		if indexed && (!readEntries || filter.Subsystem != "" && !slices.Contains(names, filter.Subsystem)) {
			continue
		}
		raw, err := s.readSegment(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return page, ErrUnavailable
		}
		var discovered map[string]bool
		if !indexed {
			discovered = make(map[string]bool)
		} else if cursorOK && rank == 0 {
			raw = raw[:min(int64(len(raw)), offset)]
		}
		for end := bytes.LastIndexByte(raw, '\n'); end >= 0; {
			start := bytes.LastIndexByte(raw[:end], '\n') + 1
			line := raw[start:end]
			end = start - 1
			var entry Entry
			if len(line) > lineBytes || json.Unmarshal(line, &entry) != nil || !validLevel(entry.Level) ||
				!labelPattern.MatchString(entry.Subsystem) || entry.Timestamp.IsZero() {
				continue
			}
			if !indexed {
				subsystems[entry.Subsystem], discovered[entry.Subsystem] = true, true
			}
			afterCursor := cursorOK && rank == 0 && int64(start) >= offset
			if !readEntries || page.Next != "" || afterCursor ||
				filter.Level != "" && entry.Level != filter.Level || filter.Subsystem != "" && entry.Subsystem != filter.Subsystem ||
				query != "" && !matchesText(entry, query, filter.MatchText) {
				continue
			}
			if len(page.Entries) == limit {
				page.Next = page.Entries[limit-1].ID
				if indexed {
					break
				}
				continue
			}
			entry.ID = file.name() + ":" + strconv.Itoa(start)
			page.Entries = append(page.Entries, entry)
		}
		if !indexed && int64(len(raw)) == min(file.size, segmentBytes) {
			names = make([]string, 0, len(discovered))
			for name := range discovered {
				names = append(names, name)
			}
			nextIndex[file] = names
		}
	}
	// A changed file has a different snapshot key, so a concurrent reader
	// replacing this cache with an older snapshot cannot affect later results.
	s.mu.Lock()
	s.index = nextIndex
	s.mu.Unlock()
	for name := range subsystems {
		page.Subsystems = append(page.Subsystems, name)
	}
	slices.Sort(page.Subsystems)
	return page, nil
}

func matchesText(entry Entry, query string, additional func(Entry) string) bool {
	if strings.Contains(strings.ToLower(entry.Message+" "+entry.Event+" "+entry.Subsystem), query) {
		return true
	}
	return additional != nil && strings.Contains(strings.ToLower(additional(entry)), query)
}

func (s *Store) readSegment(item segment) ([]byte, error) {
	// Windows cannot remove a segment while this reader holds it open.
	// Release the lock before decoding records.
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(filepath.Join(s.dir, item.name()))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, min(item.size, segmentBytes)))
}
