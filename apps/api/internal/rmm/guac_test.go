package rmm

import (
	"strings"
	"testing"
)

func TestEncodeParseGuacInstruction(t *testing.T) {
	raw := EncodeGuacInstruction("select", "rdp")
	want := "6.select,3.rdp;"
	if string(raw) != want {
		t.Fatalf("encode: got %q want %q", raw, want)
	}
	inst, err := parseGuacInstruction(raw)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Opcode != "select" || len(inst.Args) != 1 || inst.Args[0] != "rdp" {
		t.Fatalf("parse: %+v", inst)
	}
}

func TestExtractOneInstruction(t *testing.T) {
	raw := EncodeGuacInstruction("ready", "abc")
	got, ok := extractOneInstruction(append(raw, []byte("garbage")...))
	if !ok {
		t.Fatal("expected instruction")
	}
	if string(got) != string(raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestExtractOneInstructionPartial(t *testing.T) {
	raw := EncodeGuacInstruction("blob", "1", strings.Repeat("x", 100))
	half := raw[:len(raw)/2]
	if _, ok := extractOneInstruction(half); ok {
		t.Fatal("partial buffer must not yield an instruction")
	}
	rest := append([]byte{}, half...)
	rest = append(rest, raw[len(raw)/2:]...)
	got, ok := extractOneInstruction(rest)
	if !ok || string(got) != string(raw) {
		t.Fatalf("reassembled: ok=%v got=%q", ok, got)
	}
}
