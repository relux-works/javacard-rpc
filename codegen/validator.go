package codegen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	aidHexPattern     = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
	semverPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// ValidationError is a structured semantic validation error.
type ValidationError struct {
	Path    string
	Message string
}

// Validate checks a parsed Schema for semantic errors.
// It returns all errors found.
func Validate(s *Schema) []ValidationError {
	errs := make([]ValidationError, 0)
	add := func(path, msg string) {
		errs = append(errs, ValidationError{Path: path, Message: msg})
	}

	if s == nil {
		add("schema", "schema is nil")
		return errs
	}

	validateApplet(s, add)
	validateMethods(s, add)
	validateStatusWords(s, add)

	return errs
}

func validateApplet(s *Schema, add func(path, msg string)) {
	if strings.TrimSpace(s.Applet.Name) == "" {
		add("applet.name", "must be non-empty")
	}

	if !isValidAID(s.Applet.AID) {
		add("applet.aid", "must be a hex string of 5-16 bytes (10-32 hex chars)")
	}

	if s.Applet.CLA == 0x00 {
		add("applet.cla", "0x00 is reserved and not allowed")
	}

	if !semverPattern.MatchString(s.Applet.Version) {
		add("applet.version", "must be a valid semver version (X.Y.Z)")
	}
}

func validateMethods(s *Schema, add func(path, msg string)) {
	if len(s.Methods) == 0 {
		add("methods", "at least one method is required")
		return
	}

	keys := sortedMethodKeys(s.Methods)
	methodNameSeen := make(map[string]string, len(keys))
	insSeen := make(map[byte]string, len(keys))

	for _, key := range keys {
		path := fmt.Sprintf("methods.%s", key)
		if !isIdentifier(key) {
			add(path, "method name must start with a letter and contain only letters, digits, and underscore")
		}

		m := s.Methods[key]
		if m == nil {
			add(path, "method definition is nil")
			continue
		}

		if strings.TrimSpace(m.Name) != "" && !isIdentifier(m.Name) {
			add(path+".name", "method name must start with a letter and contain only letters, digits, and underscore")
		}

		methodName := key
		if strings.TrimSpace(m.Name) != "" {
			methodName = m.Name
		}
		if firstPath, ok := methodNameSeen[methodName]; ok {
			add(path, fmt.Sprintf("duplicate method name %q (already used at %s)", methodName, firstPath))
		} else {
			methodNameSeen[methodName] = path
		}

		insPath := path + ".ins"
		if firstPath, ok := insSeen[m.INS]; ok {
			add(insPath, fmt.Sprintf("duplicate INS 0x%02X (already used at %s)", m.INS, firstPath))
		} else {
			insSeen[m.INS] = insPath
		}
		if isReservedINS(m.INS) {
			add(insPath, fmt.Sprintf("INS 0x%02X is in reserved ISO 7816 range", m.INS))
		}

		validateMessage(path+".request", m.Request, true, add)
		validateMessage(path+".response", m.Response, false, add)
	}
}

func validateMessage(path string, msg *Message, isRequest bool, add func(path, msg string)) {
	if msg == nil {
		return
	}

	seenP1 := false
	seenP2 := false

	for i, f := range msg.Fields {
		fieldPath := fmt.Sprintf("%s.fields[%d]", path, i)
		if !isIdentifier(f.Name) {
			add(fieldPath+".name", "field name must start with a letter and contain only letters, digits, and underscore")
		}

		if !isKnownFieldType(f.Type) {
			add(fieldPath+".type", fmt.Sprintf("unsupported field type %q (expected u8, u16, u32, bool, bytes, or bytes[N])", f.Type))
		}

		if f.Length != nil {
			if f.Type != FieldTypeBytes {
				add(fieldPath+".length", "only supported for bytes fields")
			}
			if *f.Length <= 0 {
				add(fieldPath+".length", "must be > 0")
			}
		}
		if f.Type == FieldTypeBytesFixed && f.FixedLength <= 0 {
			add(fieldPath+".type", "bytes[N] length must be > 0")
		}

		if !isRequest && f.Location == ParameterLocationNone {
			continue
		}

		switch f.Location {
		case ParameterLocationNone, ParameterLocationData:
			// valid
		case ParameterLocationP1:
			if !isP1P2FieldType(f.Type) {
				add(fieldPath+".type", "p1 field must be of type u8 or bool")
			}
			if seenP1 {
				add(fieldPath+".location", "duplicate p1 field")
			}
			seenP1 = true
		case ParameterLocationP2:
			if !isP1P2FieldType(f.Type) {
				add(fieldPath+".type", "p2 field must be of type u8 or bool")
			}
			if seenP2 {
				add(fieldPath+".location", "duplicate p2 field")
			}
			seenP2 = true
		default:
			add(fieldPath+".location", fmt.Sprintf("unsupported field location %q", f.Location))
		}
	}
}

func validateStatusWords(s *Schema, add func(path, msg string)) {
	keys := sortedStatusWordKeys(s.StatusWords)
	statusNameSeen := make(map[string]string, len(keys))
	codeSeen := make(map[uint16]string, len(keys))

	for _, key := range keys {
		path := fmt.Sprintf("status_words.%s", key)
		if !isIdentifier(key) {
			add(path, "status word name must start with a letter and contain only letters, digits, and underscore")
		}

		sw := s.StatusWords[key]
		statusName := key
		if strings.TrimSpace(sw.Name) != "" {
			if !isIdentifier(sw.Name) {
				add(path+".name", "status word name must start with a letter and contain only letters, digits, and underscore")
			}
			statusName = sw.Name
		}

		if firstPath, ok := statusNameSeen[statusName]; ok {
			add(path, fmt.Sprintf("duplicate status word name %q (already used at %s)", statusName, firstPath))
		} else {
			statusNameSeen[statusName] = path
		}

		codePath := path + ".code"
		if !isValidStatusWordCode(sw.Code) {
			add(codePath, fmt.Sprintf("status word code 0x%04X must be in ISO 7816 ranges 0x6000-0x6FFF or 0x9000-0x9FFF", sw.Code))
		}
		if firstPath, ok := codeSeen[sw.Code]; ok {
			add(codePath, fmt.Sprintf("duplicate status word code 0x%04X (already used at %s)", sw.Code, firstPath))
		} else {
			codeSeen[sw.Code] = codePath
		}
	}
}

func sortedMethodKeys(methods map[string]*Method) []string {
	keys := make([]string, 0, len(methods))
	for k := range methods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStatusWordKeys(statusWords map[string]StatusWord) []string {
	keys := make([]string, 0, len(statusWords))
	for k := range statusWords {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isIdentifier(name string) bool {
	return identifierPattern.MatchString(name)
}

func isKnownFieldType(t FieldType) bool {
	switch t {
	case FieldTypeU8, FieldTypeU16, FieldTypeU32, FieldTypeBool, FieldTypeBytes, FieldTypeBytesFixed:
		return true
	default:
		return false
	}
}

func isP1P2FieldType(t FieldType) bool {
	return t == FieldTypeU8 || t == FieldTypeBool
}

func isReservedINS(ins byte) bool {
	return (ins >= 0x60 && ins <= 0x6F) || (ins >= 0x90 && ins <= 0x9F)
}

func isValidStatusWordCode(code uint16) bool {
	return (code >= 0x6000 && code <= 0x6FFF) || (code >= 0x9000 && code <= 0x9FFF)
}

func isValidAID(aid string) bool {
	if aid != strings.TrimSpace(aid) {
		return false
	}
	if len(aid) < 10 || len(aid) > 32 || len(aid)%2 != 0 {
		return false
	}
	return aidHexPattern.MatchString(aid)
}
