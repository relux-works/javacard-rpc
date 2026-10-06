package render

// P1/P2 can carry only the IDL's single-byte scalar types.
func isP1P2FieldType(t FieldType) bool {
	return t == FieldTypeU8 || t == FieldTypeBool
}
