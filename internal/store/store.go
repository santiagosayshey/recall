// Package store keeps grabs and imports on disk as JSON Lines, one file
// each, append only. It holds the grabs in memory by download id, which is
// the one lookup the decision needs.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/santiagosayshey/recall/internal/webhook"
)

const (
	grabsFile   = "grabs.jsonl"
	importsFile = "imports.jsonl"
)

// Store is one data directory. Safe for concurrent use.
type Store struct {
	now     func() time.Time
	mu      sync.Mutex
	grabs   *os.File
	imports *os.File
	byID    map[string]webhook.Grab
}

type Options struct {
	Now func() time.Time // for tests; nil means time.Now
}

// Open creates the files if needed and reads the grabs into memory. The
// directory must exist.
func Open(dir string, o Options) (*Store, error) {
	s := &Store{now: o.Now, byID: map[string]webhook.Grab{}}
	if s.now == nil {
		s.now = time.Now
	}
	var err error
	if s.grabs, err = open(dir, grabsFile); err != nil {
		return nil, err
	}
	if s.imports, err = open(dir, importsFile); err != nil {
		s.grabs.Close()
		return nil, err
	}
	if err := s.load(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func open(dir, name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
}

// load reads every grab line, last one per download id winning.
func (s *Store) load() error {
	if _, err := s.grabs.Seek(0, io.SeekStart); err != nil {
		return err
	}
	sc := bufio.NewScanner(s.grabs)
	sc.Buffer(nil, 4<<20)
	n := 0
	for sc.Scan() {
		n++
		var l grabLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return fmt.Errorf("%s line %d: %w", grabsFile, n, err)
		}
		l.Grab.Raw = l.Raw
		s.byID[l.DownloadID] = l.Grab
	}
	return sc.Err()
}

func (s *Store) Close() error {
	return errors.Join(s.grabs.Close(), s.imports.Close())
}

// grabLine and importLine are what a line holds: when Recall received it,
// the record, and the raw body last.
type grabLine struct {
	At time.Time `json:"at"`
	webhook.Grab
	Raw json.RawMessage `json:"raw"`
}

type importLine struct {
	At time.Time `json:"at"`
	webhook.Import
	Raw json.RawMessage `json:"raw"`
}

// AddGrab appends the grab and makes it the one found for its download id.
func (s *Store) AddGrab(g webhook.Grab) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeLine(s.grabs, grabLine{s.now(), g, g.Raw}); err != nil {
		return err
	}
	s.byID[g.DownloadID] = g
	return nil
}

// AddImport appends the import.
func (s *Store) AddImport(i webhook.Import) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeLine(s.imports, importLine{s.now(), i, i.Raw})
}

// Grab returns the latest grab for a download id.
func (s *Store) Grab(downloadID string) (webhook.Grab, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.byID[downloadID]
	return g, ok
}

// Grabs returns how many download ids have a grab.
func (s *Store) Grabs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

// writeLine appends one JSON line in a single write, so a crash leaves at
// most a torn tail and never an interleaved line.
func writeLine(f *os.File, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}
