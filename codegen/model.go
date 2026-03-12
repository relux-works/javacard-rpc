package codegen

// Schema is the root model for a TOML applet contract.
type Schema struct {
	Applet      Applet                `toml:"applet"`
	Methods     map[string]*Method    `toml:"methods"`
	StatusWords map[string]StatusWord `toml:"status_words"`
}

// Applet holds metadata about the Java Card applet contract.
type Applet struct {
	Name        string
	Description string
	Version     string
	AID         string
	CLA         byte
}

// Method describes a callable applet instruction.
type Method struct {
	Name        string
	INS         byte
	Description string
	Request     *Message
	Response    *Message
}

// Message describes structured request or response payload fields.
type Message struct {
	Fields []Field
}

// FieldType is a supported IDL field type.
type FieldType string

const (
	FieldTypeU8         FieldType = "u8"
	FieldTypeU16        FieldType = "u16"
	FieldTypeU32        FieldType = "u32"
	FieldTypeBool       FieldType = "bool"
	FieldTypeASCII      FieldType = "ascii"
	FieldTypeString     FieldType = "string"
	FieldTypeBytes      FieldType = "bytes"
	FieldTypeBytesFixed FieldType = "bytes_fixed"
)

// ParameterLocation indicates where a request field is carried in an APDU.
type ParameterLocation string

const (
	ParameterLocationNone ParameterLocation = ""
	ParameterLocationP1   ParameterLocation = "p1"
	ParameterLocationP2   ParameterLocation = "p2"
	ParameterLocationData ParameterLocation = "data"
)

// Field is a typed name/value declaration in request/response messages.
type Field struct {
	Name        string
	Type        FieldType
	Length      *int
	FixedLength int
	Location    ParameterLocation
}

// WireSize reports the encoded byte width for fixed-size fields.
// The second return value is false for variable-length fields.
func (f Field) WireSize() (int, bool) {
	switch f.Type {
	case FieldTypeU8, FieldTypeBool:
		return 1, true
	case FieldTypeU16:
		return 2, true
	case FieldTypeU32:
		return 4, true
	case FieldTypeBytesFixed:
		if f.FixedLength > 0 {
			return f.FixedLength, true
		}
		return 0, false
	case FieldTypeASCII:
		if f.Length != nil && *f.Length > 0 {
			return *f.Length, true
		}
		return 0, false
	case FieldTypeString:
		return 0, false
	case FieldTypeBytes:
		if f.Length != nil && *f.Length > 0 {
			return *f.Length, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// IsSingleByte reports whether this field occupies exactly one wire byte.
func (f Field) IsSingleByte() bool {
	size, ok := f.WireSize()
	return ok && size == 1
}

// StatusWord maps a symbolic status to its APDU code.
type StatusWord struct {
	Name        string
	Code        uint16
	Description string
}
