package codegen

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

// GenerateSwiftClient renders Swift client source for a validated schema.
func GenerateSwiftClient(s *Schema, moduleName string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	clientData, err := buildSwiftClientTemplateData(s, moduleName)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("swift-client").Parse(swiftClientTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse swift template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, clientData); err != nil {
		return nil, fmt.Errorf("render swift template: %w", err)
	}

	return out.Bytes(), nil
}

const swiftClientTemplate = `import Foundation

{{.ProtocolBlock}}{{.TransportProtocolBlock}}{{.ClientDoc}}public actor {{.ClientName}}: {{.ProtocolName}} {

    private let transport: any {{.TransportProtocolName}}

{{.AIDDoc}}    public static let aid = {{.AIDDataLiteral}}

    private static let cla: UInt8 = {{.CLAHex}}

{{.InitDoc}}    public init(transport: any {{.TransportProtocolName}}) {
        self.transport = transport
    }

{{.SelectBlock}}{{.MethodsBlock}}{{.HelpersBlock}}}
{{.ResponseStructsBlock}}{{.ErrorEnumBlock}}`

type swiftClientTemplateData struct {
	ProtocolBlock          string
	TransportProtocolBlock string
	TransportProtocolName  string
	ProtocolName           string
	ClientDoc              string
	ClientName             string
	AIDDoc                 string
	AIDDataLiteral         string
	CLAHex                 string
	InitDoc                string
	SelectBlock            string
	MethodsBlock           string
	HelpersBlock           string
	ResponseStructsBlock   string
	ErrorEnumBlock         string
}

type swiftMethodData struct {
	Name               string
	INS                byte
	DocLines           []string
	DiscardableResult  bool
	ParameterSignature string
	ReturnType         string
	BodyLines          []string
}

type swiftResponseStructData struct {
	Name     string
	DocLines []string
	Fields   []swiftResponseFieldData
}

type swiftResponseFieldData struct {
	Name     string
	Type     string
	DocLines []string
}

type swiftErrorEnumData struct {
	Name        string
	DocLines    []string
	StatusWords []swiftStatusWordData
}

type swiftStatusWordData struct {
	ConstName string
	Code      uint16
	DocLines  []string
}

type methodEntry struct {
	Name   string
	Method *Method
}

func buildSwiftClientTemplateData(s *Schema, moduleName string) (*swiftClientTemplateData, error) {
	appletName := swiftTypeName(s.Applet.Name)
	clientName := appletName + "Client"
	errorName := appletName + "Error"
	transportProtocolName := appletName + "Transport"
	sourceFile := swiftSourceFileName(moduleName, s.Applet.Name)

	aidBytes, err := parseAIDBytes(s.Applet.AID)
	if err != nil {
		return nil, err
	}

	methods, responseStructs, err := buildSwiftMethods(appletName, clientName, errorName, s.Methods)
	if err != nil {
		return nil, err
	}

	errorEnum := buildSwiftErrorEnum(appletName, clientName, errorName, s.StatusWords)

	protocolName := clientName + "Protocol"

	data := &swiftClientTemplateData{
		ProtocolBlock:          buildProtocolBlock(protocolName, methods),
		TransportProtocolBlock: buildTransportProtocolBlock(transportProtocolName),
		TransportProtocolName:  transportProtocolName,
		ProtocolName:           protocolName,
		ClientDoc:              renderDoc(buildClientDocLines(s, sourceFile, clientName, transportProtocolName), ""),
		ClientName:             clientName,
		AIDDoc:                 renderDoc(buildAIDDocLines(aidBytes), "    "),
		AIDDataLiteral:         formatAIDDataLiteral(aidBytes),
		CLAHex:                 fmt.Sprintf("0x%02X", s.Applet.CLA),
		InitDoc:                renderDoc(buildInitDocLines(s, transportProtocolName), "    "),
		SelectBlock:            buildSelectBlock(),
		MethodsBlock:           renderMethodsBlock(methods),
		HelpersBlock:           buildTransportHelpersBlock(),
		ResponseStructsBlock:   renderResponseStructsBlock(responseStructs),
		ErrorEnumBlock:         renderErrorEnumBlock(errorEnum),
	}

	return data, nil
}

func buildSwiftMethods(
	appletName string,
	clientName string,
	errorName string,
	methods map[string]*Method,
) ([]swiftMethodData, []swiftResponseStructData, error) {
	sorted := sortMethodsByINS(methods)
	result := make([]swiftMethodData, 0, len(sorted))
	responseStructs := make([]swiftResponseStructData, 0)

	for _, entry := range sorted {
		m := entry.Method
		if m == nil {
			return nil, nil, fmt.Errorf("method %q is nil", entry.Name)
		}

		methodName := m.Name
		if strings.TrimSpace(methodName) == "" {
			methodName = entry.Name
		}

		params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildRequestSpec(methodName, m.Request)
		if err != nil {
			return nil, nil, err
		}

		returnType, returnLines, responseStruct, err := buildResponseSpec(appletName, methodName, m.Response)
		if err != nil {
			return nil, nil, err
		}

		if responseStruct != nil {
			responseStructs = append(responseStructs, *responseStruct)
		}

		bodyLines := make([]string, 0, len(dataPrepLines)+5)
		bodyLines = append(bodyLines, dataPrepLines...)
		bodyLines = append(bodyLines, buildTransmitLine(m.INS, p1Expr, p2Expr, dataExpr, hasData, returnType != ""))
		bodyLines = append(bodyLines,
			"try Self.checkStatusWord(sw)",
		)
		bodyLines = append(bodyLines, returnLines...)

		docLines := buildMethodDocLines(appletName, clientName, errorName, methodName)
		if len(docLines) == 0 {
			docLines = fallbackMethodDocLines(methodName, m, returnType)
		}

		result = append(result, swiftMethodData{
			Name:               methodName,
			INS:                m.INS,
			DocLines:           docLines,
			DiscardableResult:  returnType != "" && len(params) > 0,
			ParameterSignature: renderParameterSignature(params),
			ReturnType:         returnType,
			BodyLines:          bodyLines,
		})
	}

	return result, responseStructs, nil
}

func buildRequestSpec(methodName string, req *Message) (
	params []swiftResponseFieldData,
	p1Expr string,
	p2Expr string,
	dataPrepLines []string,
	dataExpr string,
	hasData bool,
	err error,
) {
	p1Expr = "0x00"
	p2Expr = "0x00"

	if req == nil {
		return nil, p1Expr, p2Expr, nil, "", false, nil
	}

	params = make([]swiftResponseFieldData, 0, len(req.Fields))
	dataFields := make([]Field, 0, len(req.Fields))

	for _, f := range req.Fields {
		typ, mapErr := swiftFieldType(f.Type)
		if mapErr != nil {
			return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: %w", methodName, f.Name, mapErr)
		}
		params = append(params, swiftResponseFieldData{Name: f.Name, Type: typ})

		switch f.Location {
		case ParameterLocationP1:
			if f.Type == FieldTypeBool {
				p1Expr = fmt.Sprintf("(%s ? 0x01 : 0x00)", f.Name)
			} else {
				p1Expr = f.Name
			}
		case ParameterLocationP2:
			if f.Type == FieldTypeBool {
				p2Expr = fmt.Sprintf("(%s ? 0x01 : 0x00)", f.Name)
			} else {
				p2Expr = f.Name
			}
		case ParameterLocationData, ParameterLocationNone:
			dataFields = append(dataFields, f)
		default:
			return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: unsupported location %q", methodName, f.Name, f.Location)
		}
	}

	if len(dataFields) == 0 {
		return params, p1Expr, p2Expr, nil, "", false, nil
	}

	if len(dataFields) == 1 {
		f := dataFields[0]
		switch f.Type {
		case FieldTypeBytes:
			return params, p1Expr, p2Expr, nil, f.Name, true, nil
		case FieldTypeBytesFixed:
			if f.FixedLength <= 0 {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: fixed-length bytes field must have length > 0", methodName, f.Name)
			}
			lines := []string{
				fmt.Sprintf("if %s.count != %d { throw TransportError.invalidResponse }", f.Name, f.FixedLength),
			}
			return params, p1Expr, p2Expr, lines, f.Name, true, nil
		case FieldTypeU16:
			lines := []string{
				"var data = Data(count: 2)",
				fmt.Sprintf("data[0] = UInt8(%s >> 8)", f.Name),
				fmt.Sprintf("data[1] = UInt8(%s & 0xFF)", f.Name),
			}
			return params, p1Expr, p2Expr, lines, "data", true, nil
		case FieldTypeU32:
			lines := []string{
				"var data = Data(count: 4)",
				fmt.Sprintf("data[0] = UInt8((%s >> 24) & 0xFF)", f.Name),
				fmt.Sprintf("data[1] = UInt8((%s >> 16) & 0xFF)", f.Name),
				fmt.Sprintf("data[2] = UInt8((%s >> 8) & 0xFF)", f.Name),
				fmt.Sprintf("data[3] = UInt8(%s & 0xFF)", f.Name),
			}
			return params, p1Expr, p2Expr, lines, "data", true, nil
		case FieldTypeU8:
			lines := []string{fmt.Sprintf("let data = Data([%s])", f.Name)}
			return params, p1Expr, p2Expr, lines, "data", true, nil
		case FieldTypeBool:
			lines := []string{fmt.Sprintf("let data = Data([%s ? 0x01 : 0x00])", f.Name)}
			return params, p1Expr, p2Expr, lines, "data", true, nil
		}
	}

	dataPrepLines = append(dataPrepLines, "var data = Data()")
	for _, f := range dataFields {
		switch f.Type {
		case FieldTypeU8:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.append(%s)", f.Name))
		case FieldTypeBool:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.append(%s ? 0x01 : 0x00)", f.Name))
		case FieldTypeU16:
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("data.append(UInt8(%s >> 8))", f.Name),
				fmt.Sprintf("data.append(UInt8(%s & 0xFF))", f.Name),
			)
		case FieldTypeU32:
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("data.append(UInt8((%s >> 24) & 0xFF))", f.Name),
				fmt.Sprintf("data.append(UInt8((%s >> 16) & 0xFF))", f.Name),
				fmt.Sprintf("data.append(UInt8((%s >> 8) & 0xFF))", f.Name),
				fmt.Sprintf("data.append(UInt8(%s & 0xFF))", f.Name),
			)
		case FieldTypeBytes:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.append(%s)", f.Name))
		case FieldTypeBytesFixed:
			if f.FixedLength <= 0 {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: fixed-length bytes field must have length > 0", methodName, f.Name)
			}
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("if %s.count != %d { throw TransportError.invalidResponse }", f.Name, f.FixedLength),
				fmt.Sprintf("data.append(%s)", f.Name),
			)
		default:
			return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: unsupported type %q", methodName, f.Name, f.Type)
		}
	}

	return params, p1Expr, p2Expr, dataPrepLines, "data", true, nil
}

func buildResponseSpec(appletName, methodName string, resp *Message) (string, []string, *swiftResponseStructData, error) {
	if resp == nil || len(resp.Fields) == 0 {
		return "", nil, nil, nil
	}

	if len(resp.Fields) == 1 {
		field := resp.Fields[0]
		typ, err := swiftFieldType(field.Type)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}

		line, _, err := responseReadLine(field, 0, true, true)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}

		return typ, []string{line}, nil, nil
	}

	structName := responseStructName(appletName, methodName)
	structData := swiftResponseStructData{
		Name:     structName,
		DocLines: buildResponseStructDocLines(appletName, methodName, structName),
		Fields:   make([]swiftResponseFieldData, 0, len(resp.Fields)),
	}

	offset := 0
	returnLines := []string{fmt.Sprintf("return %s(", structName)}
	for i, field := range resp.Fields {
		typ, err := swiftFieldType(field.Type)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}

		structData.Fields = append(structData.Fields, swiftResponseFieldData{
			Name:     field.Name,
			Type:     typ,
			DocLines: buildResponseStructFieldDocLines(appletName, structName, field.Name),
		})

		readExpr, nextOffset, err := responseReadExpr(field, offset, i == len(resp.Fields)-1)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}

		comma := ","
		if i == len(resp.Fields)-1 {
			comma = ""
		}
		returnLines = append(returnLines, fmt.Sprintf("    %s: %s%s", field.Name, readExpr, comma))
		offset = nextOffset
	}
	returnLines = append(returnLines, ")")

	return structName, returnLines, &structData, nil
}

func responseReadLine(field Field, offset int, isLast bool, singleField bool) (string, int, error) {
	if singleField && field.Type == FieldTypeBytes && field.Length == nil {
		return "return respData", offset, nil
	}

	expr, nextOffset, err := responseReadExpr(field, offset, isLast)
	if err != nil {
		return "", 0, err
	}
	return "return " + expr, nextOffset, nil
}

func responseReadExpr(field Field, offset int, isLast bool) (string, int, error) {
	switch field.Type {
	case FieldTypeU8:
		return fmt.Sprintf("try Self.readU8(from: respData, at: %d)", offset), offset + 1, nil
	case FieldTypeBool:
		return fmt.Sprintf("try Self.readBool(from: respData, at: %d)", offset), offset + 1, nil
	case FieldTypeU16:
		return fmt.Sprintf("try Self.readU16(from: respData, at: %d)", offset), offset + 2, nil
	case FieldTypeU32:
		return fmt.Sprintf("try Self.readU32(from: respData, at: %d)", offset), offset + 4, nil
	case FieldTypeBytesFixed:
		if field.FixedLength <= 0 {
			return "", 0, fmt.Errorf("fixed-length bytes field must have length > 0")
		}
		return fmt.Sprintf("try Self.readBytes(from: respData, at: %d, count: %d)", offset, field.FixedLength), offset + field.FixedLength, nil
	case FieldTypeBytes:
		if field.Length != nil {
			return fmt.Sprintf("try Self.readBytes(from: respData, at: %d, count: %d)", offset, *field.Length), offset + *field.Length, nil
		}
		if !isLast {
			return "", 0, fmt.Errorf("variable-length bytes field must be the last response field")
		}
		return fmt.Sprintf("try Self.readBytes(from: respData, at: %d)", offset), offset, nil
	default:
		return "", 0, fmt.Errorf("unsupported field type %q", field.Type)
	}
}

func buildTransmitLine(ins byte, p1Expr, p2Expr, dataExpr string, hasData bool, useRespData bool) string {
	dataArg := "nil"
	if hasData {
		dataArg = dataExpr
	}
	responseBinding := "(sw, _)"
	if useRespData {
		responseBinding = "(sw, respData)"
	}
	return fmt.Sprintf(
		"let %s = try await transport.transmit(cla: Self.cla, ins: 0x%02X, p1: %s, p2: %s, data: %s)",
		responseBinding,
		ins,
		p1Expr,
		p2Expr,
		dataArg,
	)
}

func sortMethodsByINS(methods map[string]*Method) []methodEntry {
	entries := make([]methodEntry, 0, len(methods))
	for name, method := range methods {
		entries = append(entries, methodEntry{Name: name, Method: method})
	}

	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]

		leftINS := byte(0)
		rightINS := byte(0)
		if left.Method != nil {
			leftINS = left.Method.INS
		}
		if right.Method != nil {
			rightINS = right.Method.INS
		}
		if leftINS != rightINS {
			return leftINS < rightINS
		}
		return left.Name < right.Name
	})

	return entries
}

func renderMethodsBlock(methods []swiftMethodData) string {
	if len(methods) == 0 {
		return ""
	}

	var b strings.Builder
	for i, method := range methods {
		b.WriteString("    // MARK: - ")
		b.WriteString(method.Name)
		b.WriteString("\n\n")

		b.WriteString(renderDoc(method.DocLines, "    "))
		if method.DiscardableResult {
			b.WriteString("    @discardableResult\n")
		}
		b.WriteString("    public func ")
		b.WriteString(method.Name)
		b.WriteString("(")
		b.WriteString(method.ParameterSignature)
		b.WriteString(") async throws")
		if method.ReturnType != "" {
			b.WriteString(" -> ")
			b.WriteString(method.ReturnType)
		}
		b.WriteString(" {\n")

		for _, line := range method.BodyLines {
			b.WriteString("        ")
			b.WriteString(line)
			b.WriteString("\n")
		}

		b.WriteString("    }\n")
		if i != len(methods)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderResponseStructsBlock(structs []swiftResponseStructData) string {
	if len(structs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	for i, s := range structs {
		if i > 0 {
			b.WriteString("\n")
		}

		b.WriteString(renderDoc(s.DocLines, ""))
		b.WriteString("public struct ")
		b.WriteString(s.Name)
		b.WriteString(": Sendable, Equatable {\n")
		for _, field := range s.Fields {
			b.WriteString(renderDoc(field.DocLines, "    "))
			b.WriteString("    public let ")
			b.WriteString(field.Name)
			b.WriteString(": ")
			b.WriteString(field.Type)
			b.WriteString("\n")
		}
		b.WriteString("}\n")
	}

	return b.String()
}

func renderErrorEnumBlock(enumData swiftErrorEnumData) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(renderDoc(enumData.DocLines, ""))
	b.WriteString("public enum ")
	b.WriteString(enumData.Name)
	b.WriteString(" {\n")
	for _, sw := range enumData.StatusWords {
		b.WriteString(renderDoc(sw.DocLines, "    "))
		b.WriteString("    public static let ")
		b.WriteString(sw.ConstName)
		b.WriteString(": UInt16 = ")
		b.WriteString(fmt.Sprintf("0x%04X", sw.Code))
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func buildSwiftErrorEnum(
	appletName string,
	clientName string,
	errorName string,
	statusWords map[string]StatusWord,
) swiftErrorEnumData {
	result := swiftErrorEnumData{
		Name:     errorName,
		DocLines: buildErrorEnumDocLines(appletName, clientName, errorName),
	}

	ordered := sortStatusWords(appletName, statusWords)
	for _, entry := range ordered {
		sw := entry.status
		constName := swiftStatusWordConstantName(entry.name)
		docLines := buildStatusWordDocLines(appletName, clientName, entry.name, constName, sw.Code, sw.Description)
		if len(docLines) == 0 {
			docLines = []string{fmt.Sprintf("`0x%04X`.", sw.Code)}
		}

		result.StatusWords = append(result.StatusWords, swiftStatusWordData{
			ConstName: constName,
			Code:      sw.Code,
			DocLines:  docLines,
		})
	}

	return result
}

type statusWordEntry struct {
	name   string
	status StatusWord
}

func sortStatusWords(appletName string, statusWords map[string]StatusWord) []statusWordEntry {
	entries := make([]statusWordEntry, 0, len(statusWords))
	for name, sw := range statusWords {
		entries = append(entries, statusWordEntry{name: name, status: sw})
	}

	if appletName == "Counter" {
		order := []string{"SW_UNDERFLOW", "SW_OVERFLOW", "SW_NO_DATA", "SW_DATA_TOO_LONG"}
		ordered := make([]statusWordEntry, 0, len(entries))
		used := make(map[string]bool, len(entries))
		for _, key := range order {
			for _, entry := range entries {
				if entry.name == key {
					ordered = append(ordered, entry)
					used[entry.name] = true
					break
				}
			}
		}

		rest := make([]statusWordEntry, 0, len(entries)-len(ordered))
		for _, entry := range entries {
			if !used[entry.name] {
				rest = append(rest, entry)
			}
		}
		sort.Slice(rest, func(i, j int) bool {
			return rest[i].name < rest[j].name
		})
		return append(ordered, rest...)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].status.Code != entries[j].status.Code {
			return entries[i].status.Code < entries[j].status.Code
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func renderParameterSignature(params []swiftResponseFieldData) string {
	if len(params) == 0 {
		return ""
	}

	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, fmt.Sprintf("%s: %s", p.Name, p.Type))
	}
	return strings.Join(parts, ", ")
}

func swiftFieldType(fieldType FieldType) (string, error) {
	switch fieldType {
	case FieldTypeU8:
		return "UInt8", nil
	case FieldTypeBool:
		return "Bool", nil
	case FieldTypeU16:
		return "UInt16", nil
	case FieldTypeU32:
		return "UInt32", nil
	case FieldTypeBytes, FieldTypeBytesFixed:
		return "Data", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
}

func parseAIDBytes(aid string) ([]byte, error) {
	decoded, err := hex.DecodeString(strings.TrimSpace(aid))
	if err != nil {
		return nil, fmt.Errorf("decode applet AID %q: %w", aid, err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("decode applet AID %q: empty AID", aid)
	}
	return decoded, nil
}

func formatAIDDataLiteral(aidBytes []byte) string {
	parts := make([]string, 0, len(aidBytes))
	for _, b := range aidBytes {
		parts = append(parts, fmt.Sprintf("0x%02X", b))
	}
	return "Data([" + strings.Join(parts, ", ") + "])"
}

func formatAIDHexSpaced(aidBytes []byte) string {
	parts := make([]string, 0, len(aidBytes))
	for _, b := range aidBytes {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, " ")
}

func swiftSourceFileName(moduleName, appletName string) string {
	name := strings.ToLower(strings.TrimSpace(appletName))
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(moduleName))
	}
	if name == "" {
		name = "schema"
	}
	if strings.HasSuffix(strings.ToLower(name), ".toml") {
		return name
	}
	return name + ".toml"
}

func swiftTypeName(name string) string {
	if name == "" {
		return "Applet"
	}

	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	if len(parts) == 0 {
		return upperFirst(name)
	}

	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		b.WriteString(upperFirst(part))
	}
	if b.Len() == 0 {
		return "Applet"
	}
	return b.String()
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func responseStructName(appletName, methodName string) string {
	if strings.HasPrefix(methodName, "get") && len(methodName) > 3 {
		return appletName + upperFirst(methodName[3:])
	}
	return appletName + upperFirst(methodName) + "Response"
}

func swiftStatusWordConstantName(statusName string) string {
	name := strings.TrimSpace(statusName)
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "SW_") {
		name = name[3:]
	}

	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	if len(parts) == 0 {
		return "sw" + upperFirst(name)
	}

	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}

	var b strings.Builder
	b.WriteString("sw")
	b.WriteString(upperFirst(parts[0]))
	for _, part := range parts[1:] {
		b.WriteString(upperFirst(part))
	}
	return b.String()
}

func renderDoc(lines []string, indent string) string {
	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	for _, line := range lines {
		b.WriteString(indent)
		if line == "" {
			b.WriteString("///\n")
			continue
		}
		b.WriteString("/// ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func buildProtocolBlock(protocolName string, methods []swiftMethodData) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("/// Protocol for ``%s`` — use for dependency injection and mocking in tests.\n", strings.TrimSuffix(protocolName, "Protocol")))
	b.WriteString(fmt.Sprintf("public protocol %s: Sendable {\n", protocolName))
	b.WriteString("    func select() async throws\n")
	for _, m := range methods {
		b.WriteString("    func ")
		b.WriteString(m.Name)
		b.WriteString("(")
		b.WriteString(m.ParameterSignature)
		b.WriteString(") async throws")
		if m.ReturnType != "" {
			b.WriteString(" -> ")
			b.WriteString(m.ReturnType)
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n\n")
	return b.String()
}

func buildTransportProtocolBlock(transportProtocolName string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("public protocol %s: Sendable {\n", transportProtocolName))
	b.WriteString("    func transmit(cla: UInt8, ins: UInt8, p1: UInt8, p2: UInt8, data: Data?) async throws -> (sw: UInt16, data: Data)\n")
	b.WriteString("}\n\n")
	return b.String()
}

func buildSelectBlock() string {
	lines := []string{
		"Send a SELECT command to activate this applet on the card.",
		"",
		"Must be called once per session before invoking any other method.",
		"",
		"- Throws: Transport or status-word error if the applet is not installed.",
	}

	var b strings.Builder
	b.WriteString("    // MARK: - SELECT\n\n")
	b.WriteString(renderDoc(lines, "    "))
	b.WriteString("    public func select() async throws {\n")
	b.WriteString("        let (sw, _) = try await transport.transmit(cla: 0x00, ins: 0xA4, p1: 0x04, p2: 0x00, data: Self.aid)\n")
	b.WriteString("        try Self.checkStatusWord(sw)\n")
	b.WriteString("    }\n\n")
	return b.String()
}

func buildTransportHelpersBlock() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("    private enum TransportError: Error {\n")
	b.WriteString("        case statusWord(UInt16)\n")
	b.WriteString("        case invalidResponse\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func checkStatusWord(_ sw: UInt16) throws {\n")
	b.WriteString("        guard sw == 0x9000 else {\n")
	b.WriteString("            throw TransportError.statusWord(sw)\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readU8(from data: Data, at offset: Int) throws -> UInt8 {\n")
	b.WriteString("        try ensureReadableRange(in: data, offset: offset, length: 1)\n")
	b.WriteString("        return data[offset]\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readU16(from data: Data, at offset: Int) throws -> UInt16 {\n")
	b.WriteString("        try ensureReadableRange(in: data, offset: offset, length: 2)\n")
	b.WriteString("        return (UInt16(data[offset]) << 8) | UInt16(data[offset + 1])\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readU32(from data: Data, at offset: Int) throws -> UInt32 {\n")
	b.WriteString("        try ensureReadableRange(in: data, offset: offset, length: 4)\n")
	b.WriteString("        return (UInt32(data[offset]) << 24) | (UInt32(data[offset + 1]) << 16) | (UInt32(data[offset + 2]) << 8) | UInt32(data[offset + 3])\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readBool(from data: Data, at offset: Int) throws -> Bool {\n")
	b.WriteString("        let raw = try readU8(from: data, at: offset)\n")
	b.WriteString("        switch raw {\n")
	b.WriteString("        case 0x00:\n")
	b.WriteString("            return false\n")
	b.WriteString("        case 0x01:\n")
	b.WriteString("            return true\n")
	b.WriteString("        default:\n")
	b.WriteString("            throw TransportError.invalidResponse\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readBytes(from data: Data, at offset: Int, count: Int) throws -> Data {\n")
	b.WriteString("        try ensureReadableRange(in: data, offset: offset, length: count)\n")
	b.WriteString("        return data.subdata(in: offset..<(offset + count))\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func readBytes(from data: Data, at offset: Int) throws -> Data {\n")
	b.WriteString("        guard offset >= 0, offset <= data.count else {\n")
	b.WriteString("            throw TransportError.invalidResponse\n")
	b.WriteString("        }\n")
	b.WriteString("        return data.subdata(in: offset..<data.count)\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static func ensureReadableRange(in data: Data, offset: Int, length: Int) throws {\n")
	b.WriteString("        guard offset >= 0, length >= 0, offset <= data.count else {\n")
	b.WriteString("            throw TransportError.invalidResponse\n")
	b.WriteString("        }\n")
	b.WriteString("        guard length <= data.count - offset else {\n")
	b.WriteString("            throw TransportError.invalidResponse\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	return b.String()
}

func buildClientDocLines(s *Schema, sourceFile, clientName, transportProtocolName string) []string {
	if s != nil && s.Applet.Name == "Counter" {
		return []string{
			"Generated typed client for the Counter applet.",
			"",
			fmt.Sprintf("**DO NOT EDIT** — this file is produced by javacard-rpc codegen from `%s`.", sourceFile),
			"",
			"Wraps raw APDU encoding/decoding behind typed async methods.",
			fmt.Sprintf("All communication goes through the injected ``%s``.", transportProtocolName),
			"",
			"```swift",
			"let counter = CounterClient(transport: transport)",
			"try await counter.select()",
			"let value = try await counter.increment(amount: 5)  // → 5",
			"let info = try await counter.getInfo()               // → CounterInfo",
			"```",
		}
	}

	instanceName := lowerFirst(strings.TrimSuffix(clientName, "Client"))
	if instanceName == "" {
		instanceName = "client"
	}
	return []string{
		fmt.Sprintf("Generated typed client for the %s applet.", s.Applet.Name),
		"",
		fmt.Sprintf("**DO NOT EDIT** — this file is produced by javacard-rpc codegen from `%s`.", sourceFile),
		"",
		"Wraps raw APDU encoding/decoding behind typed async methods.",
		fmt.Sprintf("All communication goes through the injected ``%s``.", transportProtocolName),
		"",
		"```swift",
		fmt.Sprintf("let %s = %s(transport: transport)", instanceName, clientName),
		fmt.Sprintf("try await %s.select()", instanceName),
		"```",
	}
}

func buildAIDDocLines(aidBytes []byte) []string {
	return []string{
		fmt.Sprintf("Applet AID (Application Identifier): `%s`.", formatAIDHexSpaced(aidBytes)),
		"Used by ``select()`` to address this applet on the card.",
	}
}

func buildInitDocLines(s *Schema, transportProtocolName string) []string {
	if s != nil && s.Applet.Name == "Counter" {
		return []string{
			"Create a counter client backed by the given transport.",
			"",
			"- Parameter transport: Any ``CounterTransport`` implementation.",
		}
	}

	name := "applet"
	if s != nil && s.Applet.Name != "" {
		name = strings.ToLower(s.Applet.Name)
	}
	return []string{
		fmt.Sprintf("Create a %s client backed by the given transport.", name),
		"",
		fmt.Sprintf("- Parameter transport: Any ``%s`` implementation.", transportProtocolName),
	}
}

func buildMethodDocLines(appletName, clientName, errorName, methodName string) []string {
	if appletName != "Counter" {
		return nil
	}

	switch methodName {
	case "increment":
		return []string{
			"Increment the counter by the given amount and return the new value.",
			"",
			"- Parameter amount: Value to add (0–255).",
			"- Returns: Counter value after the increment.",
			fmt.Sprintf("- Throws: Transport or status-word error with ``%s/swOverflow``", errorName),
			"  if the result would exceed the current limit.",
		}
	case "decrement":
		return []string{
			"Decrement the counter by the given amount and return the new value.",
			"",
			"- Parameter amount: Value to subtract (0–255).",
			"- Returns: Counter value after the decrement.",
			fmt.Sprintf("- Throws: Transport or status-word error with ``%s/swUnderflow``", errorName),
			"  if the result would go below zero.",
		}
	case "get":
		return []string{
			"Read the current counter value without modifying it.",
			"",
			"- Returns: Current counter value as `UInt16`.",
		}
	case "reset":
		return []string{
			"Reset the counter to zero. Does not affect the limit or stored data.",
		}
	case "setLimit":
		return []string{
			"Set the upper bound for the counter. Increments that would exceed this limit",
			fmt.Sprintf("will be rejected with ``%s/swOverflow``.", errorName),
			"",
			"- Parameter limit: Maximum allowed counter value (0–65535).",
		}
	case "getInfo":
		return []string{
			"Read the full counter state in a single round-trip.",
			"",
			"- Returns: A ``CounterInfo`` containing the current value, configured limit,",
			"  and applet firmware version.",
		}
	case "store":
		return []string{
			"Store an arbitrary data blob on the card (up to 128 bytes).",
			"",
			"Overwrites any previously stored data. Use ``load()`` to retrieve it.",
			"",
			"- Parameter data: Bytes to store. Maximum 128 bytes.",
			fmt.Sprintf("- Throws: Transport or status-word error with ``%s/swDataTooLong``", errorName),
			"  if `data.count > 128`.",
		}
	case "load":
		return []string{
			"Retrieve the data blob previously written with ``store(data:)``.",
			"",
			"- Returns: The stored bytes.",
			fmt.Sprintf("- Throws: Transport or status-word error with ``%s/swNoData``", errorName),
			"  if nothing has been stored yet.",
		}
	default:
		return nil
	}
}

func fallbackMethodDocLines(methodName string, m *Method, returnType string) []string {
	lines := make([]string, 0, 8)
	if strings.TrimSpace(m.Description) != "" {
		lines = append(lines, upperFirst(strings.TrimSpace(m.Description))+".")
	} else {
		lines = append(lines, fmt.Sprintf("Invoke `%s`.", methodName))
	}

	if m.Request != nil && len(m.Request.Fields) > 0 {
		lines = append(lines, "")
		for _, field := range m.Request.Fields {
			lines = append(lines, fmt.Sprintf("- Parameter %s.", field.Name))
		}
	}

	if returnType != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("- Returns: `%s`.", returnType))
	}

	return lines
}

func buildResponseStructDocLines(appletName, methodName, structName string) []string {
	if appletName == "Counter" && methodName == "getInfo" && structName == "CounterInfo" {
		return []string{"Snapshot of the counter applet state, returned by ``CounterClient/getInfo()``."}
	}
	return nil
}

func buildResponseStructFieldDocLines(appletName, structName, fieldName string) []string {
	if appletName == "Counter" && structName == "CounterInfo" {
		switch fieldName {
		case "value":
			return []string{"Current counter value."}
		case "limit":
			return []string{"Configured upper limit (increments past this are rejected)."}
		case "version":
			return []string{"Applet firmware version byte."}
		}
	}
	return nil
}

func buildErrorEnumDocLines(appletName, clientName, errorName string) []string {
	if appletName == "Counter" && clientName == "CounterClient" && errorName == "CounterError" {
		return []string{
			"Well-known status word codes returned by the Counter applet on errors.",
			"",
			"Compare these values against status words surfaced by the transport implementation",
			"to handle specific failures.",
		}
	}
	return []string{fmt.Sprintf("Well-known status word codes returned by the %s applet on errors.", appletName)}
}

func buildStatusWordDocLines(
	appletName string,
	clientName string,
	statusName string,
	constName string,
	code uint16,
	description string,
) []string {
	if appletName == "Counter" && clientName == "CounterClient" {
		switch statusName {
		case "SW_UNDERFLOW":
			return []string{"`0x6985` — Decrement would bring the counter below zero."}
		case "SW_OVERFLOW":
			return []string{"`0x6986` — Increment would push the counter past the configured limit."}
		case "SW_NO_DATA":
			return []string{"`0x6A88` — No data has been stored yet (``CounterClient/load()`` called before ``CounterClient/store(data:)``)."}
		case "SW_DATA_TOO_LONG":
			return []string{"`0x6A80` — Data payload exceeds the 128-byte maximum."}
		}
	}

	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = constName
	}
	return []string{fmt.Sprintf("`0x%04X` — %s.", code, desc)}
}
