package api

import (
	"strings"
	"testing"
)

type fakeRows struct {
	data [][]any
	i    int
}

func (f *fakeRows) Next() bool {
	if f.i >= len(f.data) {
		return false
	}
	f.i++
	return true
}

func (f *fakeRows) Scan(dest ...any) error {
	row := f.data[f.i-1]
	for i := range dest {
		*(dest[i].(*any)) = row[i]
	}
	return nil
}

func TestRowsScanAll(t *testing.T) {
	rows := &fakeRows{data: [][]any{
		{"a", "b", nil},
		{"c", []byte("d"), int64(3)},
	}}
	got := rowsScanAll(rows, 3)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0][0] != "a" || got[0][2] != "" {
		t.Fatalf("row0=%v", got[0])
	}
	if got[1][1] != "d" || !strings.HasPrefix(got[1][2], "3") {
		t.Fatalf("row1=%v", got[1])
	}
}
