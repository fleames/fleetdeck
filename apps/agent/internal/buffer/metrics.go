// Package buffer provides a durable on-disk spool for agent metrics when the API is unreachable.
package buffer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultMaxBytes = 16 << 20 // 16 MiB
	DefaultMaxAge   = 24 * time.Hour
	fileName        = "metrics-buffer.jsonl"
)

// Spool appends failed metric payloads as JSONL and flushes when the API is back.
type Spool struct {
	Dir      string
	MaxBytes int64
	MaxAge   time.Duration
}

type entry struct {
	BufferedAt time.Time       `json:"buffered_at"`
	Payload    json.RawMessage `json:"payload"`
}

func (s *Spool) path() string {
	return filepath.Join(s.Dir, fileName)
}

func (s *Spool) maxBytes() int64 {
	if s.MaxBytes > 0 {
		return s.MaxBytes
	}
	return DefaultMaxBytes
}

func (s *Spool) maxAge() time.Duration {
	if s.MaxAge > 0 {
		return s.MaxAge
	}
	return DefaultMaxAge
}

// Append stores one metrics JSON object. Drops oldest lines when over size/age caps.
func (s *Spool) Append(payload any) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	line, err := json.Marshal(entry{BufferedAt: time.Now().UTC(), Payload: raw})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(s.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(line)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	return s.compact()
}

// Stats returns buffered sample count and age of the oldest retained entry.
func (s *Spool) Stats() (count int, oldestAgeSec int) {
	entries, err := s.readAll()
	if err != nil || len(entries) == 0 {
		return 0, 0
	}
	oldest := entries[0].BufferedAt
	for _, e := range entries[1:] {
		if e.BufferedAt.Before(oldest) {
			oldest = e.BufferedAt
		}
	}
	age := int(time.Since(oldest).Seconds())
	if age < 0 {
		age = 0
	}
	return len(entries), age
}

// FlushAll calls send for each buffered payload oldest-first. On success the
// entry is dropped; on first failure remaining entries stay on disk.
func (s *Spool) FlushAll(send func(payload json.RawMessage) error) error {
	entries, err := s.readAll()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	kept := make([]entry, 0, len(entries))
	var firstErr error
	for _, e := range entries {
		if firstErr != nil {
			kept = append(kept, e)
			continue
		}
		if err := send(e.Payload); err != nil {
			firstErr = err
			kept = append(kept, e)
			continue
		}
	}
	if err := s.rewrite(kept); err != nil {
		return err
	}
	return firstErr
}

func (s *Spool) readAll() ([]entry, error) {
	f, err := os.Open(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	cutoff := time.Now().UTC().Add(-s.maxAge())
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.BufferedAt.Before(cutoff) {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (s *Spool) compact() error {
	entries, err := s.readAll()
	if err != nil {
		return err
	}
	// Drop from the front until under MaxBytes (approx by JSON size).
	for len(entries) > 1 {
		raw, _ := json.Marshal(entries)
		if int64(len(raw)) <= s.maxBytes() {
			break
		}
		entries = entries[1:]
	}
	return s.rewrite(entries)
}

func (s *Spool) rewrite(entries []entry) error {
	path := s.path()
	if len(entries) == 0 {
		_ = os.Remove(path)
		return nil
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename spool: %w", err)
	}
	return nil
}
