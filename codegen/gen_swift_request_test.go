package codegen

import (
	"strings"
	"testing"
)

// The released backend emits fixed-length request guards for these two payloads.
func TestBuildRequestSpecBytesFixedSingleFieldValidation(t *testing.T) {
	checkFixedSwiftGuard(t, false)
}
func TestBuildRequestSpecBytesFixedMultiFieldValidation(t *testing.T) { checkFixedSwiftGuard(t, true) }
func checkFixedSwiftGuard(t *testing.T, mixed bool) {
	t.Helper()
	s, e := ParseFile("testdata/counter.toml")
	if e != nil {
		t.Fatal(e)
	}
	fields := []Field{{Name: "hash", Type: FieldTypeBytesFixed, FixedLength: 32, Location: ParameterLocationData}}
	if mixed {
		fields = append([]Field{{Name: "kind", Type: FieldTypeU8, Location: ParameterLocationData}}, fields...)
	}
	s.Methods = map[string]*Method{"setHash": {Name: "setHash", INS: 1, Request: &Message{Fields: fields}}}
	b, e := GenerateSwiftClient(s, "CounterClient")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "if hash.count != 32 { throw TransportError.invalidResponse }") {
		t.Fatalf("fixed byte guard missing:\n%s", b)
	}
}
