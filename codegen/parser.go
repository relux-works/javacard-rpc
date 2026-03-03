package codegen

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

var bytesFixedTypePattern = regexp.MustCompile(`^bytes\[(\d+)\]$`)

type rawSchema struct {
	Applet      rawApplet                `toml:"applet" json:"applet"`
	Methods     map[string]rawMethod     `toml:"methods" json:"methods"`
	StatusWords map[string]rawStatusWord `toml:"status_words" json:"status_words"`
}

type rawApplet struct {
	Name        string `toml:"name" json:"name"`
	Description string `toml:"description" json:"description"`
	Version     string `toml:"version" json:"version"`
	AID         string `toml:"aid" json:"aid"`
	CLA         int64  `toml:"cla" json:"cla"`
}

type rawMethod struct {
	INS         int64       `toml:"ins" json:"ins"`
	Description string      `toml:"description" json:"description"`
	Request     *rawMessage `toml:"request" json:"request"`
	Response    *rawMessage `toml:"response" json:"response"`
}

type rawMessage struct {
	Fields []rawField `toml:"fields" json:"fields"`
}

type rawField struct {
	Name     string `toml:"name" json:"name"`
	Type     string `toml:"type" json:"type"`
	Length   *int   `toml:"length" json:"length"`
	Location string `toml:"location" json:"location"`
}

type rawStatusWord struct {
	Code        int64  `toml:"code" json:"code"`
	Description string `toml:"description" json:"description"`
}

// Parse reads a TOML schema from r and returns a normalized model.
func Parse(r io.Reader) (*Schema, error) {
	var raw rawSchema
	meta, err := toml.NewDecoder(r).Decode(&raw)
	if err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}

	schema, err := normalizeSchema(raw)
	if err != nil {
		return nil, err
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("unknown TOML keys: %s", strings.Join(keys, ", "))
	}

	return schema, nil
}

// ParseFile opens path and parses the TOML schema from it.
func ParseFile(path string) (*Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open schema file %q: %w", path, err)
	}
	defer f.Close()

	s, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse schema file %q: %w", path, err)
	}
	return s, nil
}

func normalizeSchema(raw rawSchema) (*Schema, error) {
	cla, err := mustByte(raw.Applet.CLA, "applet.cla")
	if err != nil {
		return nil, err
	}

	schema := &Schema{
		Applet: Applet{
			Name:        raw.Applet.Name,
			Description: raw.Applet.Description,
			Version:     raw.Applet.Version,
			AID:         raw.Applet.AID,
			CLA:         cla,
		},
		Methods:     make(map[string]*Method, len(raw.Methods)),
		StatusWords: make(map[string]StatusWord, len(raw.StatusWords)),
	}

	for name, rm := range raw.Methods {
		ins, err := mustByte(rm.INS, fmt.Sprintf("methods.%s.ins", name))
		if err != nil {
			return nil, err
		}

		m := &Method{
			Name:        name,
			INS:         ins,
			Description: rm.Description,
		}

		if rm.Request != nil {
			req, err := normalizeMessage(name, "request", *rm.Request, true)
			if err != nil {
				return nil, err
			}
			m.Request = req
		}
		if rm.Response != nil {
			resp, err := normalizeMessage(name, "response", *rm.Response, false)
			if err != nil {
				return nil, err
			}
			m.Response = resp
		}

		schema.Methods[name] = m
	}

	for name, rsw := range raw.StatusWords {
		code, err := mustUint16(rsw.Code, fmt.Sprintf("status_words.%s.code", name))
		if err != nil {
			return nil, err
		}
		schema.StatusWords[name] = StatusWord{
			Name:        name,
			Code:        code,
			Description: rsw.Description,
		}
	}

	return schema, nil
}

func normalizeMessage(methodName, section string, raw rawMessage, isRequest bool) (*Message, error) {
	msg := &Message{Fields: make([]Field, 0, len(raw.Fields))}
	for i, rf := range raw.Fields {
		path := fmt.Sprintf("methods.%s.%s.fields[%d]", methodName, section, i)
		ft, fixedLen, err := parseFieldType(rf.Type)
		if err != nil {
			return nil, fmt.Errorf("%s.type: %w", path, err)
		}
		if ft != FieldTypeBytes && rf.Length != nil {
			return nil, fmt.Errorf("%s.length: only supported for bytes fields", path)
		}
		if rf.Length != nil && *rf.Length <= 0 {
			return nil, fmt.Errorf("%s.length: must be > 0", path)
		}

		f := Field{
			Name:        rf.Name,
			Type:        ft,
			Length:      cloneIntPtr(rf.Length),
			FixedLength: fixedLen,
			Location:    ParameterLocationNone,
		}
		if rf.Location != "" {
			loc, err := parseLocation(rf.Location)
			if err != nil {
				return nil, fmt.Errorf("%s.location: %w", path, err)
			}
			f.Location = loc
		}
		msg.Fields = append(msg.Fields, f)
	}

	if isRequest {
		if err := assignRequestLocations(msg.Fields); err != nil {
			return nil, fmt.Errorf("methods.%s.request: %w", methodName, err)
		}
	}

	return msg, nil
}

func assignRequestLocations(fields []Field) error {
	if len(fields) == 0 {
		return nil
	}

	explicit := false
	for _, f := range fields {
		if f.Location != ParameterLocationNone {
			explicit = true
			break
		}
	}

	if explicit {
		seenP1 := false
		seenP2 := false
		for i := range fields {
			if fields[i].Location == ParameterLocationNone {
				fields[i].Location = ParameterLocationData
				continue
			}
			switch fields[i].Location {
			case ParameterLocationP1:
				if seenP1 {
					return fmt.Errorf("duplicate p1 field")
				}
				seenP1 = true
			case ParameterLocationP2:
				if seenP2 {
					return fmt.Errorf("duplicate p2 field")
				}
				seenP2 = true
			case ParameterLocationData:
				// valid
			default:
				return fmt.Errorf("unsupported request field location %q", fields[i].Location)
			}
		}
		return nil
	}

	// If request payload looks like APDU parameter bytes, map to P1/P2.
	if len(fields) == 1 && isP1P2CompatibleFieldType(fields[0].Type) {
		fields[0].Location = ParameterLocationP1
		return nil
	}
	if len(fields) == 2 &&
		isP1P2CompatibleFieldType(fields[0].Type) &&
		isP1P2CompatibleFieldType(fields[1].Type) {
		fields[0].Location = ParameterLocationP1
		fields[1].Location = ParameterLocationP2
		return nil
	}

	for i := range fields {
		fields[i].Location = ParameterLocationData
	}
	return nil
}

func parseFieldType(t string) (FieldType, int, error) {
	trimmed := strings.TrimSpace(t)
	switch FieldType(trimmed) {
	case FieldTypeU8, FieldTypeU16, FieldTypeU32, FieldTypeBool, FieldTypeBytes:
		return FieldType(trimmed), 0, nil
	}

	m := bytesFixedTypePattern.FindStringSubmatch(trimmed)
	if len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return "", 0, fmt.Errorf("invalid fixed bytes length %q", m[1])
		}
		return FieldTypeBytesFixed, n, nil
	}

	return "", 0, fmt.Errorf("unsupported field type %q", t)
}

func isP1P2CompatibleFieldType(t FieldType) bool {
	return t == FieldTypeU8 || t == FieldTypeBool
}

func parseLocation(loc string) (ParameterLocation, error) {
	switch strings.ToLower(strings.TrimSpace(loc)) {
	case "p1":
		return ParameterLocationP1, nil
	case "p2":
		return ParameterLocationP2, nil
	case "data":
		return ParameterLocationData, nil
	default:
		return ParameterLocationNone, fmt.Errorf("expected one of p1, p2, data")
	}
}

func mustByte(v int64, path string) (byte, error) {
	if v < 0 || v > 0xFF {
		return 0, fmt.Errorf("%s: %d out of byte range", path, v)
	}
	return byte(v), nil
}

func mustUint16(v int64, path string) (uint16, error) {
	if v < 0 || v > 0xFFFF {
		return 0, fmt.Errorf("%s: %d out of uint16 range", path, v)
	}
	return uint16(v), nil
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	vv := *v
	return &vv
}
