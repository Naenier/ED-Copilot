package shipfits

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

const (
	maxBuildBytes  = 64 << 10
	maxBatchBytes  = 768 << 10
	maxExportBytes = 512 << 10
	maxBatchFits   = 1000
)

var (
	errInvalidFit = errors.New("Некорректные данные сборки.")
	errMissingFit = errors.New("Сборка не найдена.")
	errBatchSize  = errors.New("Слишком много сборок для одного импорта или экспорта.")
	errDatabase   = errors.New("Не удалось открыть или обновить базу сборок.")
	errExportPage = errors.New("Библиотека сборок изменилась. Начните экспорт заново.")
	bucketName    = []byte("ship-fits-v1")
)

type Record struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Ship      string          `json:"ship"`
	Build     json.RawMessage `json:"build,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Store struct {
	path string
}

func New(path string) Store {
	return Store{path: path}
}

func (s Store) transact(write bool, run func(*bolt.Bucket) error) error {
	if !write {
		if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
			return run(nil)
		}
	}
	if write {
		if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
			return errDatabase
		}
	}
	db, err := bolt.Open(s.path, 0600, &bolt.Options{Timeout: time.Second, ReadOnly: !write})
	if err != nil {
		return errDatabase
	}
	defer db.Close()
	if write {
		return db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(bucketName)
			if err != nil {
				return errDatabase
			}
			return run(b)
		})
	}
	return db.View(func(tx *bolt.Tx) error { return run(tx.Bucket(bucketName)) })
}

func readFit(b *bolt.Bucket, id string) (Record, error) {
	var out Record
	if b == nil || !validID(id) {
		return out, errMissingFit
	}
	value := b.Get([]byte(id))
	if value == nil {
		return out, errMissingFit
	}
	if err := json.Unmarshal(value, &out); err != nil {
		return Record{}, errDatabase
	}
	return out, nil
}

func (s Store) Get(id string) (out Record, err error) {
	err = s.transact(false, func(b *bolt.Bucket) error {
		out, err = readFit(b, id)
		return err
	})
	return
}

func (s Store) List() ([]Record, error) {
	out := []Record{}
	err := s.transact(false, func(b *bolt.Bucket) error {
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, data []byte) error {
			var fit Record
			if err := json.Unmarshal(data, &fit); err != nil {
				return errDatabase
			}
			fit.Build = nil
			out = append(out, fit)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b Record) int {
		if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
			return order
		}
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

type ExportPage struct {
	Fits     []Record
	Next     string
	Revision string
}

// Export reads a page small enough to reimport in one transaction. The cursor
// follows the underlying bbolt keys and the transaction ID detects writes
// between requests before a page can silently skip or duplicate a fit.
func (s Store) Export(after, revision string) (ExportPage, error) {
	page := ExportPage{Fits: []Record{}}
	if after != "" && (!validID(after) || revision == "") {
		return page, errExportPage
	}
	err := s.transact(false, func(b *bolt.Bucket) error {
		if b == nil {
			if revision != "" {
				return errExportPage
			}
			return nil
		}
		page.Revision = strconv.Itoa(b.Tx().ID())
		if revision != "" && revision != page.Revision {
			return errExportPage
		}
		cursor := b.Cursor()
		key, payload := cursor.First()
		if after != "" {
			key, payload = cursor.Seek([]byte(after))
			if string(key) != after {
				return errExportPage
			}
			key, payload = cursor.Next()
		}
		var bytes int
		for ; key != nil; key, payload = cursor.Next() {
			if payload == nil || len(payload) > maxExportBytes {
				return errDatabase
			}
			if len(page.Fits) == maxBatchFits || bytes+len(payload) > maxExportBytes {
				page.Next = page.Fits[len(page.Fits)-1].ID
				break
			}
			var fit Record
			if err := json.Unmarshal(payload, &fit); err != nil || fit.ID != string(key) {
				return errDatabase
			}
			page.Fits = append(page.Fits, fit)
			bytes += len(payload)
		}
		return nil
	})
	return page, err
}

func (s Store) Save(fit Record) (Record, error) {
	if err := normalize(&fit); err != nil {
		return Record{}, err
	}
	err := s.transact(true, func(b *bolt.Bucket) error {
		if fit.ID != "" {
			if _, err := readFit(b, fit.ID); err != nil {
				return err
			}
		}
		return put(b, &fit, time.Now().UTC())
	})
	return fit, err
}

func (s Store) Import(fits []Record) ([]Record, error) {
	if len(fits) == 0 || len(fits) > maxBatchFits {
		return nil, errBatchSize
	}
	total := 0
	for i := range fits {
		fits[i].ID = ""
		if err := normalize(&fits[i]); err != nil {
			return nil, err
		}
		total += len(fits[i].Build)
		if total > maxBatchBytes {
			return nil, errBatchSize
		}
	}
	err := s.transact(true, func(b *bolt.Bucket) error {
		now := time.Now().UTC()
		for i := range fits {
			if err := put(b, &fits[i], now); err != nil {
				return err
			}
			fits[i].Build = nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fits, nil
}

func (s Store) Delete(id string) error {
	return s.transact(true, func(b *bolt.Bucket) error {
		if !validID(id) || b.Get([]byte(id)) == nil {
			return errMissingFit
		}
		if err := b.Delete([]byte(id)); err != nil {
			return errDatabase
		}
		return nil
	})
}

func put(b *bolt.Bucket, fit *Record, at time.Time) error {
	for fit.ID == "" {
		var key [16]byte
		if _, err := rand.Read(key[:]); err != nil {
			return errDatabase
		}
		id := hex.EncodeToString(key[:])
		if b.Get([]byte(id)) == nil {
			fit.ID = id
		}
	}
	fit.UpdatedAt = at
	data, err := json.Marshal(fit)
	if err != nil {
		return errInvalidFit
	}
	if err = b.Put([]byte(fit.ID), data); err != nil {
		return errDatabase
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func plainText(s string, maxRunes int) bool {
	return s != "" && utf8.RuneCountInString(s) <= maxRunes && !strings.ContainsFunc(s, unicode.IsControl)
}

func normalize(fit *Record) error {
	if len(fit.Build) > maxBuildBytes {
		return errInvalidFit
	}
	var build map[string]json.RawMessage
	if err := json.Unmarshal(fit.Build, &build); err != nil || build == nil {
		return errInvalidFit
	}
	var version int
	var ship, code string
	if json.Unmarshal(build["version"], &version) != nil || version != 1 ||
		json.Unmarshal(build["shipId"], &ship) != nil || !plainText(ship, 80) ||
		json.Unmarshal(build["code"], &code) != nil || !plainText(code, maxBuildBytes) {
		return errInvalidFit
	}
	fit.Name = strings.TrimSpace(fit.Name)
	if !plainText(fit.Name, 120) || ship != fit.Ship {
		return errInvalidFit
	}
	build["name"], _ = json.Marshal(fit.Name)
	fit.Build, _ = json.Marshal(build)
	if len(fit.Build) > maxBuildBytes {
		return errInvalidFit
	}
	return nil
}
