package codegen

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

// GenerateKotlinClient renders Kotlin/JVM client source for a validated schema.
func GenerateKotlinClient(s *Schema, packageName string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("schema is nil")
	}
	if strings.TrimSpace(packageName) == "" {
		return nil, fmt.Errorf("package name is empty")
	}

	data, err := buildKotlinClientTemplateData(s, packageName)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("kotlin-client").Parse(kotlinClientTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse kotlin template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render kotlin template: %w", err)
	}

	return out.Bytes(), nil
}

const kotlinClientTemplate = `package {{.PackageName}}

import java.io.ByteArrayOutputStream

public interface {{.ProtocolName}} {
    suspend fun select()
{{.ProtocolMethodsBlock}}}

public data class {{.TransportResultName}}(
    val sw: UShort,
    val data: ByteArray,
)

public interface {{.TransportName}} {
    suspend fun transmit(cla: UByte, ins: UByte, p1: UByte, p2: UByte, data: ByteArray?): {{.TransportResultName}}
}

public sealed class {{.ClientExceptionName}}(message: String) : Exception(message) {
    public class StatusWord(public val sw: UShort) :
        {{.ClientExceptionName}}("Status word: 0x%04X".format(sw.toInt()))

    public object InvalidResponse : {{.ClientExceptionName}}("Invalid response")
}

public class {{.ClientName}}(
    private val transport: {{.TransportName}},
) : {{.ProtocolName}} {

    public companion object {
        public val aid: ByteArray = {{.AIDLiteral}}
        private const val CLA: UByte = {{.CLAHex}}u
    }

    override suspend fun select() {
        val response = transport.transmit(cla = 0x00u, ins = 0xA4u, p1 = 0x04u, p2 = 0x00u, data = aid)
        checkStatusWord(response.sw)
    }

{{.MethodsBlock}}
    private fun checkStatusWord(sw: UShort) {
        if (sw != 0x9000u.toUShort()) {
            throw {{.ClientExceptionName}}.StatusWord(sw)
        }
    }

    private fun readU8(data: ByteArray, offset: Int): UByte {
        ensureReadableRange(data, offset, 1)
        return data[offset].toUByte()
    }

    private fun readU16(data: ByteArray, offset: Int): UShort {
        ensureReadableRange(data, offset, 2)
        val hi = data[offset].toUByte().toUInt() shl 8
        val lo = data[offset + 1].toUByte().toUInt()
        return (hi or lo).toUShort()
    }

    private fun readU32(data: ByteArray, offset: Int): UInt {
        ensureReadableRange(data, offset, 4)
        val b0 = data[offset].toUByte().toUInt() shl 24
        val b1 = data[offset + 1].toUByte().toUInt() shl 16
        val b2 = data[offset + 2].toUByte().toUInt() shl 8
        val b3 = data[offset + 3].toUByte().toUInt()
        return b0 or b1 or b2 or b3
    }

    private fun readBool(data: ByteArray, offset: Int): Boolean {
        return when (readU8(data, offset)) {
            0x00u.toUByte() -> false
            0x01u.toUByte() -> true
            else -> throw {{.ClientExceptionName}}.InvalidResponse
        }
    }

    private fun readBytes(data: ByteArray, offset: Int, count: Int): ByteArray {
        ensureReadableRange(data, offset, count)
        return data.copyOfRange(offset, offset + count)
    }

    private fun readBytes(data: ByteArray, offset: Int): ByteArray {
        if (offset < 0 || offset > data.size) {
            throw {{.ClientExceptionName}}.InvalidResponse
        }
        return data.copyOfRange(offset, data.size)
    }

    private fun readASCII(data: ByteArray, offset: Int, count: Int): String = decodeASCII(readBytes(data, offset, count))

    private fun readASCII(data: ByteArray, offset: Int): String = decodeASCII(readBytes(data, offset))

    private fun decodeASCII(data: ByteArray): String {
        if (data.any { it.toInt() and 0x80 != 0 }) {
            throw {{.ClientExceptionName}}.InvalidResponse
        }
        return data.toString(Charsets.US_ASCII)
    }

    private fun asciiBytes(value: String): ByteArray {
        if (value.any { it.code > 0x7F }) {
            throw {{.ClientExceptionName}}.InvalidResponse
        }
        return value.toByteArray(Charsets.US_ASCII)
    }

    private fun readString(data: ByteArray, offset: Int): String {
        val bytes = readBytes(data, offset)
        return try {
            bytes.toString(Charsets.UTF_8)
        } catch (_: Exception) {
            throw {{.ClientExceptionName}}.InvalidResponse
        }
    }

    private fun utf8Bytes(value: String): ByteArray = value.toByteArray(Charsets.UTF_8)

    private fun invalidResponse(): Nothing = throw {{.ClientExceptionName}}.InvalidResponse

    private fun ensureReadableRange(data: ByteArray, offset: Int, length: Int) {
        if (offset < 0 || length < 0 || offset > data.size || length > data.size - offset) {
            throw {{.ClientExceptionName}}.InvalidResponse
        }
    }
}
{{.ResponseStructsBlock}}{{.ErrorBlock}}`

type kotlinClientTemplateData struct {
	PackageName          string
	ClientName           string
	ProtocolName         string
	TransportName        string
	TransportResultName  string
	ClientExceptionName  string
	AIDLiteral           string
	CLAHex               string
	ProtocolMethodsBlock string
	MethodsBlock         string
	ResponseStructsBlock string
	ErrorBlock           string
}

type kotlinMethodData struct {
	Name               string
	ParameterSignature string
	ReturnType         string
	BodyLines          []string
}

type kotlinResponseStructData struct {
	Name   string
	Fields []kotlinResponseFieldData
}

type kotlinResponseFieldData struct {
	Name string
	Type string
}

type kotlinParam struct {
	Name string
	Type string
}

func buildKotlinClientTemplateData(s *Schema, packageName string) (*kotlinClientTemplateData, error) {
	appletName := swiftTypeName(s.Applet.Name)
	clientName := appletName + "Client"
	transportName := appletName + "Transport"
	transportResultName := appletName + "TransportResult"
	clientExceptionName := appletName + "ClientException"
	protocolName := clientName + "Protocol"

	aidBytes, err := parseAIDBytes(s.Applet.AID)
	if err != nil {
		return nil, err
	}

	methods, structs, err := buildKotlinMethods(appletName, s.Methods)
	if err != nil {
		return nil, err
	}

	return &kotlinClientTemplateData{
		PackageName:          packageName,
		ClientName:           clientName,
		ProtocolName:         protocolName,
		TransportName:        transportName,
		TransportResultName:  transportResultName,
		ClientExceptionName:  clientExceptionName,
		AIDLiteral:           formatKotlinByteArrayLiteral(aidBytes),
		CLAHex:               fmt.Sprintf("0x%02X", s.Applet.CLA),
		ProtocolMethodsBlock: buildKotlinProtocolMethodsBlock(methods),
		MethodsBlock:         renderKotlinMethodsBlock(methods),
		ResponseStructsBlock: renderKotlinResponseStructsBlock(structs),
		ErrorBlock:           renderKotlinErrorBlock(appletName, s.StatusWords),
	}, nil
}

func buildKotlinMethods(appletName string, methods map[string]*Method) ([]kotlinMethodData, []kotlinResponseStructData, error) {
	sorted := sortMethodsByINS(methods)
	result := make([]kotlinMethodData, 0, len(sorted))
	responseStructs := make([]kotlinResponseStructData, 0)

	for _, entry := range sorted {
		m := entry.Method
		if m == nil {
			return nil, nil, fmt.Errorf("method %q is nil", entry.Name)
		}

		methodName := m.Name
		if strings.TrimSpace(methodName) == "" {
			methodName = entry.Name
		}

		params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildKotlinRequestSpec(methodName, m.Request)
		if err != nil {
			return nil, nil, err
		}

		returnType, returnLines, responseStruct, err := buildKotlinResponseSpec(appletName, methodName, m.Response)
		if err != nil {
			return nil, nil, err
		}

		if responseStruct != nil {
			responseStructs = append(responseStructs, *responseStruct)
		}

		bodyLines := make([]string, 0, len(dataPrepLines)+4+len(returnLines))
		bodyLines = append(bodyLines, dataPrepLines...)
		bodyLines = append(bodyLines, buildKotlinTransmitLine(m.INS, p1Expr, p2Expr, dataExpr, hasData))
		bodyLines = append(bodyLines, "checkStatusWord(response.sw)")
		bodyLines = append(bodyLines, returnLines...)

		result = append(result, kotlinMethodData{
			Name:               methodName,
			ParameterSignature: renderKotlinParameterSignature(params),
			ReturnType:         returnType,
			BodyLines:          bodyLines,
		})
	}

	return result, responseStructs, nil
}

func buildKotlinRequestSpec(methodName string, req *Message) (
	params []kotlinParam,
	p1Expr string,
	p2Expr string,
	dataPrepLines []string,
	dataExpr string,
	hasData bool,
	err error,
) {
	p1Expr = "0x00u"
	p2Expr = "0x00u"

	if req == nil {
		return nil, p1Expr, p2Expr, nil, "", false, nil
	}

	params = make([]kotlinParam, 0, len(req.Fields))
	dataFields := make([]Field, 0, len(req.Fields))

	for _, f := range req.Fields {
		typ, mapErr := kotlinFieldType(f.Type)
		if mapErr != nil {
			return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: %w", methodName, f.Name, mapErr)
		}
		params = append(params, kotlinParam{Name: f.Name, Type: typ})

		switch f.Location {
		case ParameterLocationP1:
			if f.Type == FieldTypeBool {
				p1Expr = fmt.Sprintf("if (%s) 0x01u else 0x00u", f.Name)
			} else {
				p1Expr = f.Name
			}
		case ParameterLocationP2:
			if f.Type == FieldTypeBool {
				p2Expr = fmt.Sprintf("if (%s) 0x01u else 0x00u", f.Name)
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
		case FieldTypeASCII:
			lines := []string{
				fmt.Sprintf("val data = asciiBytes(%s)", f.Name),
			}
			if f.Length != nil {
				lines = append(lines, fmt.Sprintf("if (data.size != %d) invalidResponse()", *f.Length))
			}
			return params, p1Expr, p2Expr, lines, "data", true, nil
		case FieldTypeString:
			if f.Length != nil {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: string field does not support fixed length", methodName, f.Name)
			}
			return params, p1Expr, p2Expr, []string{fmt.Sprintf("val data = utf8Bytes(%s)", f.Name)}, "data", true, nil
		case FieldTypeBytes:
			return params, p1Expr, p2Expr, nil, f.Name, true, nil
		case FieldTypeBytesFixed:
			if f.FixedLength <= 0 {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: fixed-length bytes field must have length > 0", methodName, f.Name)
			}
			return params, p1Expr, p2Expr, []string{
				fmt.Sprintf("if (%s.size != %d) invalidResponse()", f.Name, f.FixedLength),
			}, f.Name, true, nil
		case FieldTypeU16:
			return params, p1Expr, p2Expr, []string{
				fmt.Sprintf("val data = byteArrayOf((((%s.toInt() ushr 8) and 0xFF).toByte()), ((%s.toInt() and 0xFF).toByte()))", f.Name, f.Name),
			}, "data", true, nil
		case FieldTypeU32:
			return params, p1Expr, p2Expr, []string{
				fmt.Sprintf("val data = byteArrayOf((((%s.toLong() ushr 24) and 0xFF).toByte()), (((%s.toLong() ushr 16) and 0xFF).toByte()), (((%s.toLong() ushr 8) and 0xFF).toByte()), ((%s.toLong() and 0xFF).toByte()))", f.Name, f.Name, f.Name, f.Name),
			}, "data", true, nil
		case FieldTypeU8:
			return params, p1Expr, p2Expr, []string{fmt.Sprintf("val data = byteArrayOf(%s.toByte())", f.Name)}, "data", true, nil
		case FieldTypeBool:
			return params, p1Expr, p2Expr, []string{fmt.Sprintf("val data = byteArrayOf((if (%s) 0x01 else 0x00).toByte())", f.Name)}, "data", true, nil
		}
	}

	dataPrepLines = append(dataPrepLines, "val data = ByteArrayOutputStream()")
	for _, f := range dataFields {
		switch f.Type {
		case FieldTypeU8:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.write(%s.toInt())", f.Name))
		case FieldTypeBool:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.write(if (%s) 0x01 else 0x00)", f.Name))
		case FieldTypeU16:
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("data.write((%s.toInt() ushr 8) and 0xFF)", f.Name),
				fmt.Sprintf("data.write(%s.toInt() and 0xFF)", f.Name),
			)
		case FieldTypeU32:
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("data.write(((%s.toLong() ushr 24) and 0xFF).toInt())", f.Name),
				fmt.Sprintf("data.write(((%s.toLong() ushr 16) and 0xFF).toInt())", f.Name),
				fmt.Sprintf("data.write(((%s.toLong() ushr 8) and 0xFF).toInt())", f.Name),
				fmt.Sprintf("data.write((%s.toLong() and 0xFF).toInt())", f.Name),
			)
		case FieldTypeBytes:
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.write(%s)", f.Name))
		case FieldTypeASCII:
			asciiName := f.Name + "Ascii"
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("val %s = asciiBytes(%s)", asciiName, f.Name))
			if f.Length != nil {
				dataPrepLines = append(dataPrepLines, fmt.Sprintf("if (%s.size != %d) invalidResponse()", asciiName, *f.Length))
			}
			dataPrepLines = append(dataPrepLines, fmt.Sprintf("data.write(%s)", asciiName))
		case FieldTypeString:
			if f.Length != nil {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: string field does not support fixed length", methodName, f.Name)
			}
			utf8Name := f.Name + "Utf8"
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("val %s = utf8Bytes(%s)", utf8Name, f.Name),
				fmt.Sprintf("data.write(%s)", utf8Name),
			)
		case FieldTypeBytesFixed:
			if f.FixedLength <= 0 {
				return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: fixed-length bytes field must have length > 0", methodName, f.Name)
			}
			dataPrepLines = append(dataPrepLines,
				fmt.Sprintf("if (%s.size != %d) invalidResponse()", f.Name, f.FixedLength),
				fmt.Sprintf("data.write(%s)", f.Name),
			)
		default:
			return nil, "", "", nil, "", false, fmt.Errorf("method %q request field %q: unsupported type %q", methodName, f.Name, f.Type)
		}
	}

	dataPrepLines = append(dataPrepLines, "val payload = data.toByteArray()")
	return params, p1Expr, p2Expr, dataPrepLines, "payload", true, nil
}

func buildKotlinResponseSpec(appletName, methodName string, resp *Message) (string, []string, *kotlinResponseStructData, error) {
	if resp == nil || len(resp.Fields) == 0 {
		return "", nil, nil, nil
	}

	if len(resp.Fields) == 1 {
		field := resp.Fields[0]
		typ, err := kotlinFieldType(field.Type)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}
		line, _, err := kotlinResponseReadLine(field, 0, true, true)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}
		return typ, []string{line}, nil, nil
	}

	structName := responseStructName(appletName, methodName)
	structData := &kotlinResponseStructData{
		Name:   structName,
		Fields: make([]kotlinResponseFieldData, 0, len(resp.Fields)),
	}

	offset := 0
	returnLines := []string{fmt.Sprintf("return %s(", structName)}
	for i, field := range resp.Fields {
		typ, err := kotlinFieldType(field.Type)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}
		structData.Fields = append(structData.Fields, kotlinResponseFieldData{Name: field.Name, Type: typ})

		expr, nextOffset, err := kotlinResponseReadExpr(field, offset, i == len(resp.Fields)-1)
		if err != nil {
			return "", nil, nil, fmt.Errorf("method %q response field %q: %w", methodName, field.Name, err)
		}

		comma := ","
		if i == len(resp.Fields)-1 {
			comma = ""
		}
		returnLines = append(returnLines, fmt.Sprintf("    %s = %s%s", field.Name, expr, comma))
		offset = nextOffset
	}
	returnLines = append(returnLines, ")")

	return structName, returnLines, structData, nil
}

func kotlinResponseReadLine(field Field, offset int, isLast bool, singleField bool) (string, int, error) {
	if singleField && field.Type == FieldTypeBytes && field.Length == nil {
		return "return response.data", offset, nil
	}
	expr, nextOffset, err := kotlinResponseReadExpr(field, offset, isLast)
	if err != nil {
		return "", 0, err
	}
	return "return " + expr, nextOffset, nil
}

func kotlinResponseReadExpr(field Field, offset int, isLast bool) (string, int, error) {
	switch field.Type {
	case FieldTypeU8:
		return fmt.Sprintf("readU8(response.data, %d)", offset), offset + 1, nil
	case FieldTypeBool:
		return fmt.Sprintf("readBool(response.data, %d)", offset), offset + 1, nil
	case FieldTypeU16:
		return fmt.Sprintf("readU16(response.data, %d)", offset), offset + 2, nil
	case FieldTypeU32:
		return fmt.Sprintf("readU32(response.data, %d)", offset), offset + 4, nil
	case FieldTypeBytesFixed:
		if field.FixedLength <= 0 {
			return "", 0, fmt.Errorf("fixed-length bytes field must have length > 0")
		}
		return fmt.Sprintf("readBytes(response.data, %d, %d)", offset, field.FixedLength), offset + field.FixedLength, nil
	case FieldTypeASCII:
		if field.Length != nil {
			return fmt.Sprintf("readASCII(response.data, %d, %d)", offset, *field.Length), offset + *field.Length, nil
		}
		if !isLast {
			return "", 0, fmt.Errorf("variable-length ascii field must be the last response field")
		}
		return fmt.Sprintf("readASCII(response.data, %d)", offset), offset, nil
	case FieldTypeString:
		if field.Length != nil {
			return "", 0, fmt.Errorf("string field does not support fixed length")
		}
		if !isLast {
			return "", 0, fmt.Errorf("variable-length string field must be the last response field")
		}
		return fmt.Sprintf("readString(response.data, %d)", offset), offset, nil
	case FieldTypeBytes:
		if field.Length != nil {
			return fmt.Sprintf("readBytes(response.data, %d, %d)", offset, *field.Length), offset + *field.Length, nil
		}
		if !isLast {
			return "", 0, fmt.Errorf("variable-length bytes field must be the last response field")
		}
		return fmt.Sprintf("readBytes(response.data, %d)", offset), offset, nil
	default:
		return "", 0, fmt.Errorf("unsupported field type %q", field.Type)
	}
}

func buildKotlinTransmitLine(ins byte, p1Expr, p2Expr, dataExpr string, hasData bool) string {
	dataArg := "null"
	if hasData {
		dataArg = dataExpr
	}
	return fmt.Sprintf(
		"val response = transport.transmit(cla = CLA, ins = 0x%02Xu, p1 = %s, p2 = %s, data = %s)",
		ins,
		p1Expr,
		p2Expr,
		dataArg,
	)
}

func buildKotlinProtocolMethodsBlock(methods []kotlinMethodData) string {
	var b strings.Builder
	for _, method := range methods {
		b.WriteString("    suspend fun ")
		b.WriteString(method.Name)
		b.WriteString("(")
		b.WriteString(method.ParameterSignature)
		b.WriteString(")")
		if method.ReturnType != "" {
			b.WriteString(": ")
			b.WriteString(method.ReturnType)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderKotlinMethodsBlock(methods []kotlinMethodData) string {
	var b strings.Builder
	for _, method := range methods {
		b.WriteString("    override suspend fun ")
		b.WriteString(method.Name)
		b.WriteString("(")
		b.WriteString(method.ParameterSignature)
		b.WriteString(")")
		if method.ReturnType != "" {
			b.WriteString(": ")
			b.WriteString(method.ReturnType)
		}
		b.WriteString(" {\n")
		for _, line := range method.BodyLines {
			b.WriteString("        ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("    }\n\n")
	}
	return b.String()
}

func renderKotlinResponseStructsBlock(structs []kotlinResponseStructData) string {
	if len(structs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	for _, s := range structs {
		b.WriteString("public data class ")
		b.WriteString(s.Name)
		b.WriteString("(\n")
		for i, field := range s.Fields {
			comma := ","
			if i == len(s.Fields)-1 {
				comma = ""
			}
			b.WriteString("    val ")
			b.WriteString(field.Name)
			b.WriteString(": ")
			b.WriteString(field.Type)
			b.WriteString(comma)
			b.WriteString("\n")
		}
		b.WriteString(")\n\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func renderKotlinErrorBlock(appletName string, statusWords map[string]StatusWord) string {
	if len(statusWords) == 0 {
		return ""
	}

	entries := sortStatusWords(appletName, statusWords)
	var b strings.Builder
	b.WriteString("\n\npublic object ")
	b.WriteString(appletName)
	b.WriteString("Error {\n")
	for _, entry := range entries {
		b.WriteString("    public val ")
		b.WriteString(entry.name)
		b.WriteString(": UShort = ")
		b.WriteString(fmt.Sprintf("0x%04Xu.toUShort()", entry.status.Code))
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderKotlinParameterSignature(params []kotlinParam) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, param := range params {
		parts = append(parts, fmt.Sprintf("%s: %s", param.Name, param.Type))
	}
	return strings.Join(parts, ", ")
}

func kotlinFieldType(fieldType FieldType) (string, error) {
	switch fieldType {
	case FieldTypeU8:
		return "UByte", nil
	case FieldTypeBool:
		return "Boolean", nil
	case FieldTypeU16:
		return "UShort", nil
	case FieldTypeU32:
		return "UInt", nil
	case FieldTypeASCII, FieldTypeString:
		return "String", nil
	case FieldTypeBytes, FieldTypeBytesFixed:
		return "ByteArray", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
}

func formatKotlinByteArrayLiteral(bytes []byte) string {
	parts := make([]string, 0, len(bytes))
	for _, b := range bytes {
		parts = append(parts, fmt.Sprintf("0x%02X.toByte()", b))
	}
	return "byteArrayOf(" + strings.Join(parts, ", ") + ")"
}

func GenerateKotlinBuildGradle(appletLower, packageName, version string) string {
	return fmt.Sprintf(`plugins {
    kotlin("jvm") version "2.1.10"
}

group = %q
version = %q

kotlin {
    jvmToolchain(17)
}

dependencies {
    testImplementation(kotlin("test"))
}

tasks.test {
    useJUnitPlatform()
}
`, packageName, version)
}

func GenerateKotlinSettingsGradle(appletLower string) string {
	return fmt.Sprintf(`pluginManagement {
    repositories {
        gradlePluginPortal()
        mavenCentral()
    }
}

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        mavenCentral()
    }
}

rootProject.name = %q
`, appletLower+"-client-kotlin")
}

func DefaultKotlinPackage(appletName string) string {
	stem := strings.ToLower(kotlinStemName(appletName))
	if stem == "" {
		return "generated"
	}
	return stem
}

func KotlinSourceFileName(appletName string) string {
	return swiftTypeName(appletName) + "Client.kt"
}

func kotlinStemName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Applet"
	}

	var b strings.Builder
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Applet"
	}
	return b.String()
}

func sortKotlinStatusWords(statusWords map[string]StatusWord) []statusWordEntry {
	entries := make([]statusWordEntry, 0, len(statusWords))
	for name, sw := range statusWords {
		entries = append(entries, statusWordEntry{name: name, status: sw})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].status.Code != entries[j].status.Code {
			return entries[i].status.Code < entries[j].status.Code
		}
		return entries[i].name < entries[j].name
	})
	return entries
}
