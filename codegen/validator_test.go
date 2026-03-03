package codegen

import (
	"path/filepath"
	"strings"
	"testing"
)

const minimalValidSchemaTOML = `
[applet]
name = "Demo"
version = "1.2.3"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01
[methods.echo.request]
fields = [{ name = "arg", type = "u8" }]
[methods.echo.response]
fields = [{ name = "value", type = "u8" }]

[status_words]
SW_OK = { code = 0x9000 }
`

func TestValidateCounterSchemaHasNoErrors(t *testing.T) {
	path := filepath.Join("testdata", "counter.toml")
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%s) returned error: %v", path, err)
	}

	errs := Validate(s)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

func TestValidateReturnsAllErrors(t *testing.T) {
	s := validSchema(t)
	s.Applet.Name = ""
	s.Applet.CLA = 0x00

	errs := Validate(s)
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, got %d (%v)", len(errs), errs)
	}

	requireValidationError(t, errs, "applet.name", "must be non-empty")
	requireValidationError(t, errs, "applet.cla", "reserved")
}

func TestValidateRejectsEmptyAppletName(t *testing.T) {
	s := validSchema(t)
	s.Applet.Name = " \t"

	errs := Validate(s)
	requireValidationError(t, errs, "applet.name", "non-empty")
}

func TestValidateRejectsInvalidAIDHex(t *testing.T) {
	s := validSchema(t)
	s.Applet.AID = "A00000000G"

	errs := Validate(s)
	requireValidationError(t, errs, "applet.aid", "hex string")
}

func TestValidateRejectsInvalidAIDLength(t *testing.T) {
	s := validSchema(t)
	s.Applet.AID = "A0000000"

	errs := Validate(s)
	requireValidationError(t, errs, "applet.aid", "5-16 bytes")
}

func TestValidateRejectsZeroCLA(t *testing.T) {
	s := validSchema(t)
	s.Applet.CLA = 0x00

	errs := Validate(s)
	requireValidationError(t, errs, "applet.cla", "reserved")
}

func TestValidateRejectsInvalidSemver(t *testing.T) {
	s := validSchema(t)
	s.Applet.Version = "1.0"

	errs := Validate(s)
	requireValidationError(t, errs, "applet.version", "semver")
}

func TestValidateRequiresAtLeastOneMethod(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods", "at least one method")
}

func TestValidateRejectsDuplicateINS(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.alpha]
ins = 0x01

[methods.beta]
ins = 0x01

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.beta.ins", "duplicate INS")
}

func TestValidateRejectsReservedINSRange(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.alpha]
ins = 0x60

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.alpha.ins", "reserved ISO 7816 range")
}

func TestValidateRejectsInvalidMethodName(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.bad-name]
ins = 0x01

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.bad-name", "method name")
}

func TestValidateRejectsDuplicateMethodNames(t *testing.T) {
	s := validSchema(t)
	s.Methods["other"] = &Method{
		Name: "echo",
		INS:  0x02,
	}

	errs := Validate(s)
	requireValidationError(t, errs, "methods.other", "duplicate method name")
}

func TestValidateRejectsInvalidFieldName(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01
[methods.echo.request]
fields = [{ name = "bad-name", type = "u8" }]

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[0].name", "field name")
}

func TestValidateRejectsUnknownFieldType(t *testing.T) {
	s := validSchema(t)
	s.Methods["echo"].Request.Fields[0].Type = FieldType("unknown")
	s.Methods["echo"].Request.Fields[0].Location = ParameterLocationData

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[0].type", "unsupported field type")
}

func TestValidateAcceptsU32BoolAndFixedBytes(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.setEnabled]
ins = 0x01
[methods.setEnabled.request]
fields = [{ name = "enabled", type = "bool" }]

[methods.setCount]
ins = 0x02
[methods.setCount.request]
fields = [{ name = "value", type = "u32" }]

[methods.getHash]
ins = 0x03
[methods.getHash.response]
fields = [{ name = "hash", type = "bytes[32]" }]

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
}

func TestValidateRejectsBytesFixedLengthZero(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.getHash]
ins = 0x01
[methods.getHash.response]
fields = [{ name = "hash", type = "bytes[0]" }]

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.getHash.response.fields[0].type", "bytes[N] length must be > 0")
}

func TestValidateRejectsInvalidBytesLength(t *testing.T) {
	s := validSchema(t)
	zero := 0
	s.Methods["echo"].Request.Fields = []Field{{
		Name:     "payload",
		Type:     FieldTypeBytes,
		Length:   &zero,
		Location: ParameterLocationData,
	}}

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[0].length", "must be > 0")
}

func TestValidateRejectsP1TypeNotU8(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01
[methods.echo.request]
fields = [{ name = "arg", type = "u16", location = "p1" }]

[status_words]
SW_OK = { code = 0x9000 }
	`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[0].type", "p1 field must be of type u8 or bool")
}

func TestValidateRejectsP1U32Field(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01
[methods.echo.request]
fields = [{ name = "arg", type = "u32", location = "p1" }]

[status_words]
SW_OK = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[0].type", "p1 field must be of type u8 or bool")
}

func TestValidateRejectsDuplicateP1Fields(t *testing.T) {
	s := validSchema(t)
	s.Methods["echo"].Request.Fields = []Field{
		{Name: "a", Type: FieldTypeU8, Location: ParameterLocationP1},
		{Name: "b", Type: FieldTypeU8, Location: ParameterLocationP1},
	}

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[1].location", "duplicate p1 field")
}

func TestValidateRejectsDuplicateP2Fields(t *testing.T) {
	s := validSchema(t)
	s.Methods["echo"].Request.Fields = []Field{
		{Name: "a", Type: FieldTypeU8, Location: ParameterLocationP2},
		{Name: "b", Type: FieldTypeU8, Location: ParameterLocationP2},
	}

	errs := Validate(s)
	requireValidationError(t, errs, "methods.echo.request.fields[1].location", "duplicate p2 field")
}

func TestValidateRejectsStatusWordCodeOutOfISO7816Range(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01

[status_words]
SW_BAD = { code = 0x1234 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "status_words.SW_BAD.code", "must be in ISO 7816 ranges")
}

func TestValidateRejectsDuplicateStatusWordCodes(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01

[status_words]
SW_ONE = { code = 0x9000 }
SW_TWO = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "status_words.SW_TWO.code", "duplicate status word code")
}

func TestValidateRejectsInvalidStatusWordName(t *testing.T) {
	s := mustParseSchema(t, `
[applet]
name = "Demo"
version = "1.0.0"
aid = "A000000001"
cla = 0x80

[methods.echo]
ins = 0x01

[status_words]
SW-BAD = { code = 0x9000 }
`)

	errs := Validate(s)
	requireValidationError(t, errs, "status_words.SW-BAD", "status word name")
}

func TestValidateRejectsDuplicateStatusWordNames(t *testing.T) {
	s := validSchema(t)
	s.StatusWords["SW_OTHER"] = StatusWord{
		Name: "SW_OK",
		Code: 0x9001,
	}

	errs := Validate(s)
	requireValidationError(t, errs, "status_words.SW_OTHER", "duplicate status word name")
}

func validSchema(t *testing.T) *Schema {
	t.Helper()
	return mustParseSchema(t, minimalValidSchemaTOML)
}

func mustParseSchema(t *testing.T, input string) *Schema {
	t.Helper()

	s, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	return s
}

func requireValidationError(t *testing.T, errs []ValidationError, wantPath string, wantMessageSubstr string) {
	t.Helper()

	for _, err := range errs {
		if err.Path == wantPath && strings.Contains(err.Message, wantMessageSubstr) {
			return
		}
	}

	t.Fatalf("expected error path=%q containing %q, got %v", wantPath, wantMessageSubstr, errs)
}
