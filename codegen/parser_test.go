package codegen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCounterAppletMetadata(t *testing.T) {
	s := parseCounter(t)

	if s.Applet.Name != "Counter" {
		t.Fatalf("unexpected applet name: %q", s.Applet.Name)
	}
	if s.Applet.Description != "Simple counter with increment, decrement, get, reset" {
		t.Fatalf("unexpected applet description: %q", s.Applet.Description)
	}
	if s.Applet.Version != "1.0.0" {
		t.Fatalf("unexpected applet version: %q", s.Applet.Version)
	}
	if s.Applet.AID != "F000000101" {
		t.Fatalf("unexpected applet AID: %q", s.Applet.AID)
	}
	if s.Applet.CLA != 0xB0 {
		t.Fatalf("unexpected CLA: got 0x%02X", s.Applet.CLA)
	}
}

func TestParseCounterMethodsINS(t *testing.T) {
	s := parseCounter(t)

	expected := map[string]byte{
		"increment":  0x01,
		"decrement":  0x02,
		"get":        0x03,
		"reset":      0x04,
		"setLimit":   0x05,
		"getInfo":    0x06,
		"store":      0x07,
		"load":       0x08,
		"setCount":   0x09,
		"setEnabled": 0x0A,
		"getHash":    0x0B,
	}

	if len(s.Methods) != len(expected) {
		t.Fatalf("method count mismatch: got %d want %d", len(s.Methods), len(expected))
	}

	for method, wantINS := range expected {
		m, ok := s.Methods[method]
		if !ok {
			t.Fatalf("missing method %q", method)
		}
		if m.INS != wantINS {
			t.Fatalf("method %q ins mismatch: got 0x%02X want 0x%02X", method, m.INS, wantINS)
		}
	}
}

func TestParseCounterStatusWords(t *testing.T) {
	s := parseCounter(t)

	expected := map[string]uint16{
		"SW_UNDERFLOW":     0x6985,
		"SW_OVERFLOW":      0x6986,
		"SW_NO_DATA":       0x6A88,
		"SW_DATA_TOO_LONG": 0x6A80,
	}

	if len(s.StatusWords) != len(expected) {
		t.Fatalf("status word count mismatch: got %d want %d", len(s.StatusWords), len(expected))
	}

	for name, wantCode := range expected {
		sw, ok := s.StatusWords[name]
		if !ok {
			t.Fatalf("missing status word %q", name)
		}
		if sw.Code != wantCode {
			t.Fatalf("status word %q mismatch: got 0x%04X want 0x%04X", name, sw.Code, wantCode)
		}
	}
}

func TestFieldTypesAndRequestLocations(t *testing.T) {
	s := parseCounter(t)

	assertMethodRequestField(t, s, "increment", "amount", FieldTypeU8, ParameterLocationP1)
	assertMethodRequestField(t, s, "decrement", "amount", FieldTypeU8, ParameterLocationP1)
	assertMethodRequestField(t, s, "setLimit", "limit", FieldTypeU16, ParameterLocationData)
	assertMethodRequestField(t, s, "store", "data", FieldTypeBytes, ParameterLocationData)
	assertMethodRequestField(t, s, "setCount", "value", FieldTypeU32, ParameterLocationData)
	assertMethodRequestField(t, s, "setEnabled", "enabled", FieldTypeBool, ParameterLocationP1)

	if s.Methods["get"].Request != nil {
		t.Fatalf("get should have no request")
	}
	if s.Methods["reset"].Request != nil {
		t.Fatalf("reset should have no request")
	}

	loadResp := s.Methods["load"].Response
	if loadResp == nil || len(loadResp.Fields) != 1 {
		t.Fatalf("load response should have exactly one field")
	}
	if loadResp.Fields[0].Type != FieldTypeBytes {
		t.Fatalf("load response field type mismatch: got %q", loadResp.Fields[0].Type)
	}
	if loadResp.Fields[0].FixedLength != 0 {
		t.Fatalf("load response fixed length mismatch: got %d", loadResp.Fields[0].FixedLength)
	}

	getHashResp := s.Methods["getHash"].Response
	if getHashResp == nil || len(getHashResp.Fields) != 1 {
		t.Fatalf("getHash response should have exactly one field")
	}
	if getHashResp.Fields[0].Type != FieldTypeBytesFixed {
		t.Fatalf("getHash response field type mismatch: got %q", getHashResp.Fields[0].Type)
	}
	if getHashResp.Fields[0].FixedLength != 32 {
		t.Fatalf("getHash response fixed length mismatch: got %d want 32", getHashResp.Fields[0].FixedLength)
	}

	getInfoResp := s.Methods["getInfo"].Response
	if getInfoResp == nil || len(getInfoResp.Fields) != 3 {
		t.Fatalf("getInfo response should have 3 fields")
	}
	want := []FieldType{FieldTypeU16, FieldTypeU16, FieldTypeU8}
	for i, wt := range want {
		if getInfoResp.Fields[i].Type != wt {
			t.Fatalf("getInfo response field[%d] type mismatch: got %q want %q", i, getInfoResp.Fields[i].Type, wt)
		}
	}
}

func TestInferP1P2ForTwoU8Fields(t *testing.T) {
	input := `
[applet]
name = "X"
version = "1.0.0"
aid = "A00001"
cla = 0x80

[methods.dual]
ins = 0x09
[methods.dual.request]
fields = [
  { name = "first", type = "u8" },
  { name = "second", type = "u8" }
]
`

	s, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	fields := s.Methods["dual"].Request.Fields
	if fields[0].Location != ParameterLocationP1 {
		t.Fatalf("first field location mismatch: got %q", fields[0].Location)
	}
	if fields[1].Location != ParameterLocationP2 {
		t.Fatalf("second field location mismatch: got %q", fields[1].Location)
	}
}

func TestInferP1P2ForBoolAndU8Fields(t *testing.T) {
	input := `
[applet]
name = "X"
version = "1.0.0"
aid = "A00001"
cla = 0x80

[methods.dual]
ins = 0x09
[methods.dual.request]
fields = [
  { name = "enabled", type = "bool" },
  { name = "mode", type = "u8" }
]
`

	s, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	fields := s.Methods["dual"].Request.Fields
	if fields[0].Location != ParameterLocationP1 {
		t.Fatalf("first field location mismatch: got %q", fields[0].Location)
	}
	if fields[1].Location != ParameterLocationP2 {
		t.Fatalf("second field location mismatch: got %q", fields[1].Location)
	}
}

func TestParseMalformedTOMLReturnsDescriptiveError(t *testing.T) {
	_, err := Parse(strings.NewReader("[applet\nname='broken'"))
	if err == nil {
		t.Fatalf("expected error for malformed TOML")
	}
	if !strings.Contains(err.Error(), "parse TOML") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func parseCounter(t *testing.T) *Schema {
	t.Helper()

	path := filepath.Join("testdata", "counter.toml")
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%s) returned error: %v", path, err)
	}
	return s
}

func assertMethodRequestField(
	t *testing.T,
	s *Schema,
	methodName string,
	wantName string,
	wantType FieldType,
	wantLocation ParameterLocation,
) {
	t.Helper()

	m, ok := s.Methods[methodName]
	if !ok {
		t.Fatalf("missing method %q", methodName)
	}
	if m.Request == nil || len(m.Request.Fields) != 1 {
		t.Fatalf("method %q should have exactly one request field", methodName)
	}

	f := m.Request.Fields[0]
	if f.Name != wantName {
		t.Fatalf("method %q field name mismatch: got %q want %q", methodName, f.Name, wantName)
	}
	if f.Type != wantType {
		t.Fatalf("method %q field type mismatch: got %q want %q", methodName, f.Type, wantType)
	}
	if f.Location != wantLocation {
		t.Fatalf("method %q location mismatch: got %q want %q", methodName, f.Location, wantLocation)
	}
}
