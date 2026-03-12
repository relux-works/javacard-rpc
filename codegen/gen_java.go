package codegen

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

const javaTransportTemplate = `package {{.PackageName}};

/**
 * Transport abstraction for {{.AppletName}} commands.
 * Implement this in your APDU adapter (or any other transport).
 */
public interface {{.TransportInterfaceName}} {
    byte[] transmit(byte ins, byte p1, byte p2, byte[] data);
}
`

const javaSkeletonTemplate = `package {{.PackageName}};

/**
 * Generated skeleton for {{.AppletName}} applet.
 * DO NOT EDIT — this file is produced by javacard-rpc codegen from {{.SchemaFileName}}.
 *
 * CLA: 0x{{.CLAHex}}
 * Methods:
{{.MethodCommentBlock}} */
public abstract class {{.ClassName}} {

    // INS codes from {{.SchemaFileName}}
{{.INSConstantsBlock}}

    // Status words from {{.SchemaFileName}}
{{.StatusConstantsBlock}}

    public static final byte {{.CLAConstName}} = (byte) 0x{{.CLAHex}};

    private static final short SW_WRONG_LENGTH = (short) 0x6700;
    private static final short SW_INS_NOT_SUPPORTED = (short) 0x6D00;
    private static final byte[] EMPTY = new byte[0];

    protected final {{.TransportInterfaceName}} transport;

    protected {{.ClassName}}({{.TransportInterfaceName}} transport) {
        if (transport == null) {
            throw new IllegalArgumentException("transport is null");
        }
        this.transport = transport;
    }

    public final byte[] dispatch(byte ins, byte p1, byte p2, byte[] data) {
        byte[] requestData = safeBytes(data);
        switch (ins) {
{{.DispatchCasesBlock}}            default:
                throw new StatusWordException(SW_INS_NOT_SUPPORTED);
        }
    }

    protected final byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {
        return safeBytes(transport.transmit(ins, p1, p2, safeBytes(data)));
    }

    // --- Dispatch wiring (reads request fields, calls abstract method, returns encoded response) ---

{{.HandlersBlock}}    // --- Abstract methods — developer implements these ---

{{.AbstractMethodsBlock}}
    public static final class StatusWordException extends RuntimeException {
        private final short statusWord;

        public StatusWordException(short statusWord) {
            super("Status word: 0x" + toHex(statusWord));
            this.statusWord = statusWord;
        }

        public short getStatusWord() {
            return statusWord;
        }
    }

    private static String toHex(short value) {
        int unsigned = value & 0xFFFF;
        String hex = Integer.toHexString(unsigned).toUpperCase();
        if (hex.length() >= 4) {
            return hex;
        }
        StringBuilder b = new StringBuilder(4);
        for (int i = hex.length(); i < 4; i++) {
            b.append('0');
        }
        b.append(hex);
        return b.toString();
    }

    private static byte[] safeBytes(byte[] data) {
        return data == null ? EMPTY : data;
    }

    protected static int packU8(byte[] buf, int off, byte value) {
        if (off < 0 || off >= buf.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        buf[off] = value;
        return off + 1;
    }

    protected static int packBool(byte[] buf, int off, boolean value) {
        return packU8(buf, off, (byte) (value ? 0x01 : 0x00));
    }

    protected static int packU16(byte[] buf, int off, short value) {
        if (off < 0 || off+1 >= buf.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        buf[off] = (byte) ((value >>> 8) & 0xFF);
        buf[off+1] = (byte) (value & 0xFF);
        return off + 2;
    }

    protected static int packU32(byte[] buf, int off, int value) {
        if (off < 0 || off+3 >= buf.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        buf[off] = (byte) ((value >>> 24) & 0xFF);
        buf[off+1] = (byte) ((value >>> 16) & 0xFF);
        buf[off+2] = (byte) ((value >>> 8) & 0xFF);
        buf[off+3] = (byte) (value & 0xFF);
        return off + 4;
    }

    protected static int packBytes(byte[] dst, int dstOff, byte[] src, int srcOff, int srcLen) {
        if (srcLen < 0 || dstOff < 0 || srcOff < 0 || dstOff+srcLen > dst.length || srcOff+srcLen > src.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        if (srcLen == 0) {
            return dstOff;
        }
        System.arraycopy(src, srcOff, dst, dstOff, srcLen);
        return dstOff + srcLen;
    }

    private static boolean readBool(byte value) {
        if (value == 0x00) {
            return false;
        }
        if (value == 0x01) {
            return true;
        }
        throw new StatusWordException(SW_WRONG_LENGTH);
    }

    private static byte readU8(byte[] data, int off) {
        if (off < 0 || off >= data.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        return data[off];
    }

    private static boolean readBool(byte[] data, int off) {
        return readBool(readU8(data, off));
    }

    private static short readU16(byte[] data, int off) {
        if (off < 0 || off+1 >= data.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        return (short) (((data[off] & 0xFF) << 8) | (data[off+1] & 0xFF));
    }

    private static int readU32(byte[] data, int off) {
        if (off < 0 || off+3 >= data.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        return ((data[off] & 0xFF) << 24)
            | ((data[off+1] & 0xFF) << 16)
            | ((data[off+2] & 0xFF) << 8)
            | (data[off+3] & 0xFF);
    }

    private static byte[] slice(byte[] data, int off, int len) {
        if (off < 0 || len < 0 || off+len > data.length) {
            throw new StatusWordException(SW_WRONG_LENGTH);
        }
        if (len == 0) {
            return EMPTY;
        }
        byte[] out = new byte[len];
        System.arraycopy(data, off, out, 0, len);
        return out;
    }
}
`

type responseKind int

const (
	responseKindNone responseKind = iota
	responseKindPrimitive
	responseKindBytes
	responseKindPacked
)

type javaTemplateData struct {
	PackageName            string
	AppletName             string
	ClassName              string
	TransportInterfaceName string
	SchemaFileName         string
	CLAHex                 string
	CLAConstName           string
	MethodCommentBlock     string
	INSConstantsBlock      string
	StatusConstantsBlock   string
	DispatchCasesBlock     string
	HandlersBlock          string
	AbstractMethodsBlock   string
}

type javaMethodRender struct {
	Name           string
	INS            byte
	INSConstName   string
	HandlerName    string
	AbstractName   string
	Signature      string
	HasHandler     bool
	HandlerLines   []string
	AbstractReturn string
	AbstractParams []string
	ResponseKind   responseKind
}

type requestHandling struct {
	Lines      []string
	ArgExprs   []string
	ParamDecls []string
	Comment    string
}

// JavaGenerationResult holds the generated Java source files.
type JavaGenerationResult struct {
	TransportSource []byte // CounterTransport.java
	SkeletonSource  []byte // CounterSkeleton.java
	TransportName   string // e.g. "CounterTransport"
	SkeletonName    string // e.g. "CounterSkeleton"
}

// GenerateJavaSkeleton renders a Java Card abstract applet skeleton from a validated schema.
func GenerateJavaSkeleton(s *Schema, packageName string) (*JavaGenerationResult, error) {
	if s == nil {
		return nil, fmt.Errorf("schema is nil")
	}
	if strings.TrimSpace(packageName) == "" {
		return nil, fmt.Errorf("package name is empty")
	}

	methods, err := renderMethods(s)
	if err != nil {
		return nil, err
	}

	data := javaTemplateData{
		PackageName:            strings.TrimSpace(packageName),
		AppletName:             strings.TrimSpace(s.Applet.Name),
		ClassName:              toPascal(s.Applet.Name) + "Skeleton",
		TransportInterfaceName: toPascal(s.Applet.Name) + "Transport",
		SchemaFileName:         strings.ToLower(strings.TrimSpace(s.Applet.Name)) + ".toml",
		CLAHex:                 fmt.Sprintf("%02X", s.Applet.CLA),
		CLAConstName:           "CLA_" + toUpperSnake(s.Applet.Name),
		MethodCommentBlock:     buildMethodCommentBlock(methods),
		INSConstantsBlock:      buildINSConstantsBlock(methods),
		StatusConstantsBlock:   buildStatusConstantsBlock(s.StatusWords),
		DispatchCasesBlock:     buildDispatchCasesBlock(methods),
		HandlersBlock:          buildHandlersBlock(methods),
		AbstractMethodsBlock:   buildAbstractMethodsBlock(methods),
	}

	// Render transport interface
	transportTpl, err := template.New("java_transport").Parse(javaTransportTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse java transport template: %w", err)
	}
	var transportOut bytes.Buffer
	if err := transportTpl.Execute(&transportOut, data); err != nil {
		return nil, fmt.Errorf("render java transport: %w", err)
	}

	// Render skeleton
	skeletonTpl, err := template.New("java_skeleton").Parse(javaSkeletonTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse java skeleton template: %w", err)
	}
	var skeletonOut bytes.Buffer
	if err := skeletonTpl.Execute(&skeletonOut, data); err != nil {
		return nil, fmt.Errorf("render java skeleton: %w", err)
	}

	return &JavaGenerationResult{
		TransportSource: transportOut.Bytes(),
		SkeletonSource:  skeletonOut.Bytes(),
		TransportName:   data.TransportInterfaceName,
		SkeletonName:    data.ClassName,
	}, nil
}

func renderMethods(s *Schema) ([]javaMethodRender, error) {
	type methodEntry struct {
		Name   string
		Method *Method
	}

	entries := make([]methodEntry, 0, len(s.Methods))
	for key, method := range s.Methods {
		if method == nil {
			return nil, fmt.Errorf("methods.%s: method definition is nil", key)
		}
		name := strings.TrimSpace(method.Name)
		if name == "" {
			name = key
		}
		entries = append(entries, methodEntry{Name: name, Method: method})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Method.INS == entries[j].Method.INS {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Method.INS < entries[j].Method.INS
	})

	rendered := make([]javaMethodRender, 0, len(entries))
	for _, entry := range entries {
		m, err := renderMethod(entry.Name, entry.Method)
		if err != nil {
			return nil, fmt.Errorf("methods.%s: %w", entry.Name, err)
		}
		rendered = append(rendered, m)
	}

	return rendered, nil
}

func renderMethod(name string, m *Method) (javaMethodRender, error) {
	request, err := buildRequestHandling(m.Request)
	if err != nil {
		return javaMethodRender{}, err
	}

	mr := javaMethodRender{
		Name:         name,
		INS:          m.INS,
		INSConstName: "INS_" + toUpperSnake(name),
		HandlerName:  "handle" + toPascal(name),
		AbstractName: "on" + toPascal(name),
		Signature:    methodSignature(name, m),
	}

	responseFields := responseFields(m.Response)
	switch {
	case len(responseFields) == 0:
		mr.AbstractReturn = "void"
		mr.AbstractParams = request.ParamDecls
		mr.ResponseKind = responseKindNone
		if len(request.Lines) == 0 {
			mr.HasHandler = false
			return mr, nil
		}
		mr.HasHandler = true
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("%s(%s);", mr.AbstractName, strings.Join(request.ArgExprs, ", ")),
			"return EMPTY;",
		)
		return mr, nil
	case len(responseFields) == 1 && responseFields[0].Type == FieldTypeU8:
		mr.AbstractReturn = "byte"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		resultName := responseFields[0].Name
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("byte %s = %s;", resultName, call),
			"byte[] out = new byte[1];",
			fmt.Sprintf("packU8(out, 0, %s);", resultName),
			"return out;",
		)
		mr.ResponseKind = responseKindPrimitive
		return mr, nil
	case len(responseFields) == 1 && responseFields[0].Type == FieldTypeBool:
		mr.AbstractReturn = "boolean"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		resultName := responseFields[0].Name
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("boolean %s = %s;", resultName, call),
			"byte[] out = new byte[1];",
			fmt.Sprintf("packBool(out, 0, %s);", resultName),
			"return out;",
		)
		mr.ResponseKind = responseKindPrimitive
		return mr, nil
	case len(responseFields) == 1 && responseFields[0].Type == FieldTypeU16:
		mr.AbstractReturn = "short"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		resultName := responseFields[0].Name
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("short %s = %s;", resultName, call),
			"byte[] out = new byte[2];",
			fmt.Sprintf("packU16(out, 0, %s);", resultName),
			"return out;",
		)
		mr.ResponseKind = responseKindPrimitive
		return mr, nil
	case len(responseFields) == 1 && responseFields[0].Type == FieldTypeU32:
		mr.AbstractReturn = "int"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		resultName := responseFields[0].Name
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("int %s = %s;", resultName, call),
			"byte[] out = new byte[4];",
			fmt.Sprintf("packU32(out, 0, %s);", resultName),
			"return out;",
		)
		mr.ResponseKind = responseKindPrimitive
		return mr, nil
	case len(responseFields) == 1 && isByteSequenceField(responseFields[0]):
		mr.AbstractReturn = "byte[]"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		lines := []string{
			fmt.Sprintf("byte[] src = safeBytes(%s);", call),
		}
		if fixedLen, ok := byteSequenceFixedLength(responseFields[0]); ok {
			lines = append(lines,
				fmt.Sprintf("if (src.length != %d) {", fixedLen),
				"    throw new StatusWordException(SW_WRONG_LENGTH);",
				"}",
				fmt.Sprintf("byte[] out = new byte[%d];", fixedLen),
				fmt.Sprintf("packBytes(out, 0, src, 0, %d);", fixedLen),
				"return out;",
			)
		} else {
			lines = append(lines,
				"byte[] out = new byte[src.length];",
				"packBytes(out, 0, src, 0, src.length);",
				"return out;",
			)
		}
		mr.HandlerLines = appendHandlerBody(request, lines...)
		mr.ResponseKind = responseKindBytes
		return mr, nil
	default:
		mr.AbstractReturn = "byte[]"
		mr.AbstractParams = request.ParamDecls
		mr.HasHandler = true
		call := fmt.Sprintf("%s(%s)", mr.AbstractName, strings.Join(request.ArgExprs, ", "))
		mr.HandlerLines = appendHandlerBody(
			request,
			fmt.Sprintf("byte[] out = %s;", call),
			"return safeBytes(out);",
		)
		mr.ResponseKind = responseKindPacked
		return mr, nil
	}
}

func buildRequestHandling(msg *Message) (requestHandling, error) {
	rh := requestHandling{}
	if msg == nil || len(msg.Fields) == 0 {
		return rh, nil
	}

	fields := msg.Fields
	rh.Comment = requestComment(fields)

	hasData := false
	variableFieldIndex := -1
	fixedDataLen := 0

	for i, f := range fields {
		switch f.Location {
		case ParameterLocationP1, ParameterLocationP2:
			if !isP1P2FieldType(f.Type) {
				return requestHandling{}, fmt.Errorf("%s field must be u8 or bool", f.Location)
			}
		case ParameterLocationData, ParameterLocationNone:
			hasData = true
			switch f.Type {
			case FieldTypeU8, FieldTypeBool:
				fixedDataLen++
			case FieldTypeU16:
				fixedDataLen += 2
			case FieldTypeU32:
				fixedDataLen += 4
			case FieldTypeBytesFixed:
				if f.FixedLength <= 0 {
					return requestHandling{}, fmt.Errorf("fixed bytes request field %q must have length > 0", f.Name)
				}
				fixedDataLen += f.FixedLength
			case FieldTypeASCII, FieldTypeString, FieldTypeBytes:
				if f.Type == FieldTypeString && f.Length != nil {
					return requestHandling{}, fmt.Errorf("string request field %q does not support fixed length", f.Name)
				}
				if f.Length != nil {
					fixedDataLen += *f.Length
					break
				}
				if variableFieldIndex != -1 {
					return requestHandling{}, fmt.Errorf("multiple variable-length request fields are unsupported")
				}
				variableFieldIndex = i
			default:
				return requestHandling{}, fmt.Errorf("unsupported request field type %q", f.Type)
			}
		default:
			return requestHandling{}, fmt.Errorf("unsupported request field location %q", f.Location)
		}
	}

	if variableFieldIndex != -1 {
		for i := variableFieldIndex + 1; i < len(fields); i++ {
			if fields[i].Location == ParameterLocationData || fields[i].Location == ParameterLocationNone {
				return requestHandling{}, fmt.Errorf("variable-length request field must be last among data fields")
			}
		}
	}

	if hasData {
		if fixedDataLen > 0 {
			rh.Lines = append(rh.Lines,
				fmt.Sprintf("if (requestData.length < %d) {", fixedDataLen),
				"    throw new StatusWordException(SW_WRONG_LENGTH);",
				"}",
			)
		}
	}

	dataOffset := 0
	for _, f := range fields {
		switch f.Location {
		case ParameterLocationP1:
			switch f.Type {
			case FieldTypeU8:
				rh.ParamDecls = append(rh.ParamDecls, "byte "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("byte %s = p1;", f.Name))
			case FieldTypeBool:
				rh.ParamDecls = append(rh.ParamDecls, "boolean "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("boolean %s = readBool(p1);", f.Name))
			default:
				return requestHandling{}, fmt.Errorf("unsupported request field type %q", f.Type)
			}
		case ParameterLocationP2:
			switch f.Type {
			case FieldTypeU8:
				rh.ParamDecls = append(rh.ParamDecls, "byte "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("byte %s = p2;", f.Name))
			case FieldTypeBool:
				rh.ParamDecls = append(rh.ParamDecls, "boolean "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("boolean %s = readBool(p2);", f.Name))
			default:
				return requestHandling{}, fmt.Errorf("unsupported request field type %q", f.Type)
			}
		case ParameterLocationData, ParameterLocationNone:
			switch f.Type {
			case FieldTypeU8:
				rh.ParamDecls = append(rh.ParamDecls, "byte "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("byte %s = readU8(requestData, %d);", f.Name, dataOffset))
				dataOffset++
			case FieldTypeBool:
				rh.ParamDecls = append(rh.ParamDecls, "boolean "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("boolean %s = readBool(requestData, %d);", f.Name, dataOffset))
				dataOffset++
			case FieldTypeU16:
				rh.ParamDecls = append(rh.ParamDecls, "short "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("short %s = readU16(requestData, %d);", f.Name, dataOffset))
				dataOffset += 2
			case FieldTypeU32:
				rh.ParamDecls = append(rh.ParamDecls, "int "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(rh.Lines, fmt.Sprintf("int %s = readU32(requestData, %d);", f.Name, dataOffset))
				dataOffset += 4
			case FieldTypeBytesFixed:
				if f.FixedLength <= 0 {
					return requestHandling{}, fmt.Errorf("fixed bytes request field %q must have length > 0", f.Name)
				}
				rh.ParamDecls = append(rh.ParamDecls, "byte[] "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(
					rh.Lines,
					fmt.Sprintf("byte[] %s = slice(requestData, %d, %d);", f.Name, dataOffset, f.FixedLength),
				)
				dataOffset += f.FixedLength
			case FieldTypeASCII, FieldTypeString:
				rh.ParamDecls = append(rh.ParamDecls, "byte[] "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				if f.Type == FieldTypeString && f.Length != nil {
					return requestHandling{}, fmt.Errorf("string request field %q does not support fixed length", f.Name)
				}
				if f.Length != nil {
					rh.Lines = append(
						rh.Lines,
						fmt.Sprintf("byte[] %s = slice(requestData, %d, %d);", f.Name, dataOffset, *f.Length),
					)
					dataOffset += *f.Length
					break
				}
				rh.Lines = append(
					rh.Lines,
					fmt.Sprintf(
						"byte[] %s = slice(requestData, %d, requestData.length - %d);",
						f.Name,
						dataOffset,
						dataOffset,
					),
				)
			case FieldTypeBytes:
				rh.ParamDecls = append(rh.ParamDecls, "byte[] "+f.Name)
				rh.ArgExprs = append(rh.ArgExprs, f.Name)
				rh.Lines = append(
					rh.Lines,
					fmt.Sprintf(
						"byte[] %s = slice(requestData, %d, requestData.length - %d);",
						f.Name,
						dataOffset,
						dataOffset,
					),
				)
			default:
				return requestHandling{}, fmt.Errorf("unsupported request field type %q", f.Type)
			}
		default:
			return requestHandling{}, fmt.Errorf("unsupported request field location %q", f.Location)
		}
	}

	return rh, nil
}

func appendHandlerBody(request requestHandling, lines ...string) []string {
	out := make([]string, 0, len(request.Lines)+len(lines)+1)
	if request.Comment != "" {
		out = append(out, "// "+request.Comment)
	}
	out = append(out, request.Lines...)
	out = append(out, lines...)
	return out
}

func buildMethodCommentBlock(methods []javaMethodRender) string {
	var b strings.Builder
	for _, method := range methods {
		fmt.Fprintf(&b, " *   INS 0x%02X — %s\n", method.INS, method.Signature)
	}
	return b.String()
}

func buildINSConstantsBlock(methods []javaMethodRender) string {
	maxLen := 0
	for _, method := range methods {
		if len(method.INSConstName) > maxLen {
			maxLen = len(method.INSConstName)
		}
	}

	var b strings.Builder
	for _, method := range methods {
		padding := strings.Repeat(" ", maxLen-len(method.INSConstName)+2)
		fmt.Fprintf(&b, "    private static final byte %s%s= (byte) 0x%02X;\n", method.INSConstName, padding, method.INS)
	}
	if b.Len() > 0 {
		trimmed := strings.TrimSuffix(b.String(), "\n")
		return trimmed
	}
	return ""
}

func buildStatusConstantsBlock(statusWords map[string]StatusWord) string {
	type statusEntry struct {
		Name string
		Code uint16
	}
	entries := make([]statusEntry, 0, len(statusWords))
	for key, sw := range statusWords {
		name := strings.TrimSpace(sw.Name)
		if name == "" {
			name = key
		}
		entries = append(entries, statusEntry{Name: name, Code: sw.Code})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Code < entries[j].Code
		}
		return entries[i].Name > entries[j].Name
	})

	maxLen := 0
	for _, entry := range entries {
		if len(entry.Name) > maxLen {
			maxLen = len(entry.Name)
		}
	}

	var b strings.Builder
	for _, entry := range entries {
		padding := strings.Repeat(" ", maxLen-len(entry.Name)+1)
		fmt.Fprintf(&b, "    public static final short %s%s= (short) 0x%04X;\n", entry.Name, padding, entry.Code)
	}
	if b.Len() > 0 {
		trimmed := strings.TrimSuffix(b.String(), "\n")
		return trimmed
	}
	return ""
}

func buildDispatchCasesBlock(methods []javaMethodRender) string {
	var b strings.Builder
	for _, method := range methods {
		fmt.Fprintf(&b, "            case %s:\n", method.INSConstName)
		if method.HasHandler {
			fmt.Fprintf(&b, "                return %s(p1, p2, requestData);\n", method.HandlerName)
		} else {
			fmt.Fprintf(&b, "                %s();\n", method.AbstractName)
			b.WriteString("                return EMPTY;\n")
		}
	}
	return b.String()
}

func buildHandlersBlock(methods []javaMethodRender) string {
	var b strings.Builder
	for _, method := range methods {
		if !method.HasHandler {
			continue
		}
		fmt.Fprintf(&b, "    private byte[] %s(byte p1, byte p2, byte[] requestData) {\n", method.HandlerName)
		for _, line := range method.HandlerLines {
			fmt.Fprintf(&b, "        %s\n", line)
		}
		b.WriteString("    }\n\n")
	}
	return b.String()
}

func buildAbstractMethodsBlock(methods []javaMethodRender) string {
	var b strings.Builder
	for i, method := range methods {
		switch method.ResponseKind {
		case responseKindPacked:
			fmt.Fprintf(&b, "    /**\n")
			fmt.Fprintf(&b, "     * %s\n", method.Signature)
			fmt.Fprintf(&b, "     * Return encoded response bytes in schema field order.\n")
			fmt.Fprintf(&b, "     */\n")
		case responseKindBytes:
			fmt.Fprintf(&b, "    /**\n")
			fmt.Fprintf(&b, "     * %s\n", method.Signature)
			fmt.Fprintf(&b, "     * Return response bytes.\n")
			fmt.Fprintf(&b, "     */\n")
		default:
			fmt.Fprintf(&b, "    /** %s */\n", method.Signature)
		}
		fmt.Fprintf(
			&b,
			"    protected abstract %s %s(%s);\n",
			method.AbstractReturn,
			method.AbstractName,
			strings.Join(method.AbstractParams, ", "),
		)
		if i != len(methods)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func requestComment(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}

	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		location := "data"
		switch f.Location {
		case ParameterLocationP1:
			location = "P1"
		case ParameterLocationP2:
			location = "P2"
		case ParameterLocationData, ParameterLocationNone:
			if isByteSequenceField(f) {
				location = "request data"
			}
		}
		parts = append(parts, fmt.Sprintf("%s(%s) in %s", f.Name, f.Type, location))
	}
	return "request: " + strings.Join(parts, ", ")
}

func methodSignature(name string, m *Method) string {
	var b strings.Builder

	b.WriteString(name)
	b.WriteString("(")
	requestFields := requestFields(m.Request)
	for i, f := range requestFields {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: %s", f.Name, f.Type)
	}
	b.WriteString(")")

	responseFields := responseFields(m.Response)
	switch len(responseFields) {
	case 0:
		return b.String()
	case 1:
		fmt.Fprintf(&b, " → %s: %s", responseFields[0].Name, responseFields[0].Type)
		return b.String()
	default:
		b.WriteString(" → {")
		for i, f := range responseFields {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s: %s", f.Name, f.Type)
		}
		b.WriteString("}")
		return b.String()
	}
}

func requestFields(msg *Message) []Field {
	if msg == nil {
		return nil
	}
	return msg.Fields
}

func responseFields(msg *Message) []Field {
	if msg == nil {
		return nil
	}
	return msg.Fields
}

func isByteSequenceField(f Field) bool {
	return f.Type == FieldTypeASCII || f.Type == FieldTypeString || f.Type == FieldTypeBytes || f.Type == FieldTypeBytesFixed
}

func byteSequenceFixedLength(f Field) (int, bool) {
	switch f.Type {
	case FieldTypeBytesFixed:
		return f.FixedLength, f.FixedLength > 0
	case FieldTypeASCII, FieldTypeBytes:
		if f.Length != nil && *f.Length > 0 {
			return *f.Length, true
		}
	}
	return 0, false
}

func toPascal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	parts := splitIdentifierParts(s)
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

func toUpperSnake(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	parts := splitIdentifierParts(s)
	if len(parts) == 0 {
		return ""
	}

	for i, part := range parts {
		parts[i] = strings.ToUpper(part)
	}
	return strings.Join(parts, "_")
}

func splitIdentifierParts(s string) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}

	parts := make([]string, 0, 4)
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			parts = append(parts, string(runes[start:end]))
		}
		start = -1
	}

	for i, r := range runes {
		isAlphaNum := unicode.IsLetter(r) || unicode.IsDigit(r)
		if !isAlphaNum {
			flush(i)
			continue
		}
		if start == -1 {
			start = i
			continue
		}

		prev := runes[i-1]
		boundary := false
		if unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			boundary = true
		} else if unicode.IsDigit(r) && unicode.IsLetter(prev) {
			boundary = true
		} else if unicode.IsLetter(r) && unicode.IsDigit(prev) {
			boundary = true
		} else if unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			boundary = true
		}

		if boundary {
			flush(i)
			start = i
		}
	}
	flush(len(runes))
	return parts
}
