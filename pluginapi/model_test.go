package pluginapi

import "testing"

// Model helpers retain fixed widths and reject variable/invalid widths without
// requiring the facade or TOML dependency in this independent module.
func TestWireWidths(t *testing.T) {
	length := 12
	for _, tc := range []struct {
		field Field
		size  int
		fixed bool
	}{
		{Field{Type: FieldTypeU8}, 1, true},
		{Field{Type: FieldTypeBool}, 1, true},
		{Field{Type: FieldTypeU16}, 2, true},
		{Field{Type: FieldTypeU32}, 4, true},
		{Field{Type: FieldTypeBytesFixed, FixedLength: 32}, 32, true},
		{Field{Type: FieldTypeASCII, Length: &length}, 12, true},
		{Field{Type: FieldTypeBytes, Length: &length}, 12, true},
		{Field{Type: FieldTypeBytesFixed, FixedLength: -1}, 0, false},
		{Field{Type: FieldTypeString}, 0, false},
		{Field{Type: FieldTypeStream}, 0, false},
		{Field{Type: "unknown"}, 0, false},
	} {
		size, fixed := tc.field.WireSize()
		if size != tc.size || fixed != tc.fixed {
			t.Errorf("%+v: got %d/%t, want %d/%t", tc.field, size, fixed, tc.size, tc.fixed)
		}
		if tc.field.IsSingleByte() != (tc.fixed && tc.size == 1) {
			t.Errorf("single byte: %+v", tc.field)
		}
	}
}

// Stream discovery handles nil ordinary payloads and finds either stream direction.
func TestStreamDiscovery(t *testing.T) {
	var nilMethod *Method
	if nilMethod.HasStream() || (&Method{}).HasStream() {
		t.Fatal("nil/ordinary method claims stream")
	}
	message := &Message{Fields: []Field{{Type: FieldTypeU8}, {Type: FieldTypeStream}}}
	if message.StreamField() != &message.Fields[1] {
		t.Fatal("wrong stream field")
	}
	for _, m := range []*Method{{Request: message}, {Response: message}} {
		if !m.HasStream() {
			t.Fatal("stream direction lost")
		}
	}
}
