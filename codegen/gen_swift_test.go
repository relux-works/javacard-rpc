package codegen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSwiftClientCounterGolden(t *testing.T) {
	schemaPath := filepath.Join("testdata", "counter.toml")
	s, err := ParseFile(schemaPath)
	if err != nil {
		t.Fatalf("ParseFile(%s) returned error: %v", schemaPath, err)
	}

	if errs := Validate(s); len(errs) > 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}

	got, err := GenerateSwiftClient(s, "counter")
	if err != nil {
		t.Fatalf("GenerateSwiftClient returned error: %v", err)
	}

	goldenPath := filepath.Join("testdata", "CounterClient.swift.golden")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("generated Swift mismatch (-want +got):\n%s", simpleLineDiff(want, got))
	}
}

func TestGenerateSwiftClientSortsMethodsByINS(t *testing.T) {
	s := &Schema{
		Applet: Applet{
			Name: "Demo",
			AID:  "A000000001",
			CLA:  0x80,
		},
		Methods: map[string]*Method{
			"third":  {Name: "third", INS: 0x03},
			"first":  {Name: "first", INS: 0x01},
			"second": {Name: "second", INS: 0x02},
		},
		StatusWords: map[string]StatusWord{
			"SW_OK": {Name: "SW_OK", Code: 0x9000},
		},
	}

	got, err := GenerateSwiftClient(s, "demo")
	if err != nil {
		t.Fatalf("GenerateSwiftClient returned error: %v", err)
	}

	src := string(got)
	idxFirst := strings.Index(src, "// MARK: - first")
	idxSecond := strings.Index(src, "// MARK: - second")
	idxThird := strings.Index(src, "// MARK: - third")
	if idxFirst == -1 || idxSecond == -1 || idxThird == -1 {
		t.Fatalf("generated source missing expected methods:\n%s", src)
	}
	if !(idxFirst < idxSecond && idxSecond < idxThird) {
		t.Fatalf("methods are not sorted by INS:\n%s", src)
	}
}

func TestGenerateSwiftClientCounterAPDUConstruction(t *testing.T) {
	s := parseCounter(t)
	if errs := Validate(s); len(errs) > 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}

	got, err := GenerateSwiftClient(s, "counter")
	if err != nil {
		t.Fatalf("GenerateSwiftClient returned error: %v", err)
	}

	src := string(got)

	requireContains(t, src, "import Foundation")
	requireNotContains(t, src, "import JavaCardRPCClient")
	requireContains(t, src, "public protocol CounterTransport: Sendable {")
	requireContains(t, src, "func transmit(cla: UInt8, ins: UInt8, p1: UInt8, p2: UInt8, data: Data?) async throws -> (sw: UInt16, data: Data)")
	requireContains(t, src, "private let transport: any CounterTransport")
	requireContains(t, src, "public init(transport: any CounterTransport)")
	requireContains(t, src, "public static let aid = Data([0xF0, 0x00, 0x00, 0x01, 0x01])")
	requireContains(t, src, "private static let cla: UInt8 = 0xB0")
	requireContains(t, src, "let (sw, _) = try await transport.transmit(cla: 0x00, ins: 0xA4, p1: 0x04, p2: 0x00, data: Self.aid)")
	requireContains(t, src, "let (sw, respData) = try await transport.transmit(cla: Self.cla, ins: 0x01, p1: amount, p2: 0x00, data: nil)")
	requireContains(t, src, "let (sw, _) = try await transport.transmit(cla: Self.cla, ins: 0x05, p1: 0x00, p2: 0x00, data: data)")
	requireContains(t, src, "try Self.checkStatusWord(sw)")
	requireContains(t, src, "var data = Data(count: 2)")
	requireContains(t, src, "data[0] = UInt8(limit >> 8)")
	requireContains(t, src, "data[1] = UInt8(limit & 0xFF)")
	requireContains(t, src, "public func setCount(value: UInt32) async throws")
	requireContains(t, src, "data[0] = UInt8((value >> 24) & 0xFF)")
	requireContains(t, src, "data[1] = UInt8((value >> 16) & 0xFF)")
	requireContains(t, src, "data[2] = UInt8((value >> 8) & 0xFF)")
	requireContains(t, src, "data[3] = UInt8(value & 0xFF)")
	requireContains(t, src, "public func setEnabled(enabled: Bool) async throws")
	requireContains(t, src, "ins: 0x0A, p1: (enabled ? 0x01 : 0x00), p2: 0x00")
	requireContains(t, src, "public func getHash() async throws -> Data")
	requireContains(t, src, "return try Self.readBytes(from: respData, at: 0, count: 32)")
	requireContains(t, src, "return try Self.readU16(from: respData, at: 0)")
	requireContains(t, src, "version: try Self.readU8(from: respData, at: 4)")
	requireContains(t, src, "public struct CounterInfo: Sendable, Equatable")
	requireContains(t, src, "public enum CounterError")
	requireContains(t, src, "public static let swUnderflow: UInt16 = 0x6985")
}

func TestBuildRequestSpecBytesFixedSingleFieldValidation(t *testing.T) {
	params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildRequestSpec("setHash", &Message{
		Fields: []Field{
			{
				Name:        "hash",
				Type:        FieldTypeBytesFixed,
				FixedLength: 32,
				Location:    ParameterLocationData,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRequestSpec returned error: %v", err)
	}

	if len(params) != 1 {
		t.Fatalf("params length mismatch: got %d want 1", len(params))
	}
	if params[0].Name != "hash" || params[0].Type != "Data" {
		t.Fatalf("parameter mismatch: got %#v", params[0])
	}
	if p1Expr != "0x00" || p2Expr != "0x00" {
		t.Fatalf("p1/p2 mismatch: got p1=%q p2=%q", p1Expr, p2Expr)
	}
	if !hasData {
		t.Fatalf("hasData mismatch: got false want true")
	}
	if dataExpr != "hash" {
		t.Fatalf("dataExpr mismatch: got %q want %q", dataExpr, "hash")
	}
	wantLines := []string{
		"if hash.count != 32 { throw TransportError.invalidResponse }",
	}
	if !equalStrings(dataPrepLines, wantLines) {
		t.Fatalf("dataPrepLines mismatch:\nwant: %#v\ngot:  %#v", wantLines, dataPrepLines)
	}
}

func TestBuildRequestSpecBytesFixedMultiFieldValidation(t *testing.T) {
	params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err := buildRequestSpec("pack", &Message{
		Fields: []Field{
			{Name: "prefix", Type: FieldTypeU8, Location: ParameterLocationData},
			{Name: "hash", Type: FieldTypeBytesFixed, FixedLength: 32, Location: ParameterLocationData},
		},
	})
	if err != nil {
		t.Fatalf("buildRequestSpec returned error: %v", err)
	}

	if len(params) != 2 {
		t.Fatalf("params length mismatch: got %d want 2", len(params))
	}
	if p1Expr != "0x00" || p2Expr != "0x00" {
		t.Fatalf("p1/p2 mismatch: got p1=%q p2=%q", p1Expr, p2Expr)
	}
	if !hasData {
		t.Fatalf("hasData mismatch: got false want true")
	}
	if dataExpr != "data" {
		t.Fatalf("dataExpr mismatch: got %q want %q", dataExpr, "data")
	}
	wantLines := []string{
		"var data = Data()",
		"data.append(prefix)",
		"if hash.count != 32 { throw TransportError.invalidResponse }",
		"data.append(hash)",
	}
	if !equalStrings(dataPrepLines, wantLines) {
		t.Fatalf("dataPrepLines mismatch:\nwant: %#v\ngot:  %#v", wantLines, dataPrepLines)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func requireContains(t *testing.T, src, needle string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Fatalf("generated source missing expected snippet: %q", needle)
	}
}

func requireNotContains(t *testing.T, src, needle string) {
	t.Helper()
	if strings.Contains(src, needle) {
		t.Fatalf("generated source unexpectedly contains snippet: %q", needle)
	}
}

func simpleLineDiff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	maxLines := len(wantLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}

	var b strings.Builder
	diffCount := 0
	for i := 0; i < maxLines; i++ {
		var w string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		var g string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w == g {
			continue
		}
		diffCount++
		fmt.Fprintf(&b, "@@ line %d @@\n- %s\n+ %s\n", i+1, w, g)
		if diffCount >= 40 {
			b.WriteString("... (diff truncated)\n")
			break
		}
	}

	if diffCount == 0 && len(want) != len(got) {
		fmt.Fprintf(&b, "byte lengths differ: want=%d got=%d\n", len(want), len(got))
	}

	if b.Len() == 0 {
		return "(no line-level diff available)"
	}
	return b.String()
}
