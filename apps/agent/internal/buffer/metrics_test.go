package buffer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolAppendFlush(t *testing.T) {
	dir := t.TempDir()
	s := &Spool{Dir: dir, MaxBytes: 1 << 20, MaxAge: time.Hour}
	payload := map[string]any{"schema_version": 1, "host": []any{}}
	if err := s.Append(payload); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(payload); err != nil {
		t.Fatal(err)
	}
	n, age := s.Stats()
	if n != 2 {
		t.Fatalf("count=%d want 2", n)
	}
	if age < 0 {
		t.Fatal("negative age")
	}
	var sent int
	if err := s.FlushAll(func(p json.RawMessage) error {
		sent++
		var m map[string]any
		if err := json.Unmarshal(p, &m); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if sent != 2 {
		t.Fatalf("sent=%d", sent)
	}
	n, _ = s.Stats()
	if n != 0 {
		t.Fatalf("expected empty after flush, got %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(err) {
		t.Fatalf("expected spool file removed, err=%v", err)
	}
}

func TestSpoolFlushStopsOnError(t *testing.T) {
	dir := t.TempDir()
	s := &Spool{Dir: dir}
	_ = s.Append(map[string]any{"n": 1})
	_ = s.Append(map[string]any{"n": 2})
	var sent int
	err := s.FlushAll(func(p json.RawMessage) error {
		sent++
		if sent == 1 {
			return os.ErrDeadlineExceeded
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	n, _ := s.Stats()
	if n != 2 {
		t.Fatalf("both entries should remain, got %d", n)
	}
}

func TestSpoolDropsExpired(t *testing.T) {
	dir := t.TempDir()
	s := &Spool{Dir: dir, MaxAge: time.Millisecond}
	_ = s.Append(map[string]any{"n": 1})
	time.Sleep(5 * time.Millisecond)
	n, _ := s.Stats()
	if n != 0 {
		t.Fatalf("expected expired drop, got %d", n)
	}
}
