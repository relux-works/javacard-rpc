package codegen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateJavaSkeletonCounterGolden(t *testing.T) {
	schemaPath := filepath.Join("testdata", "counter.toml")
	s, err := ParseFile(schemaPath)
	if err != nil {
		t.Fatalf("ParseFile(%s) returned error: %v", schemaPath, err)
	}

	if errs := Validate(s); len(errs) != 0 {
		t.Fatalf("Validate returned %d errors: %v", len(errs), errs)
	}

	result, err := GenerateJavaSkeleton(s, "io.jcrpc.counter.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	// Check transport interface golden
	transportGoldenPath := filepath.Join("testdata", "CounterTransport.java.golden")
	transportWant, err := os.ReadFile(transportGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", transportGoldenPath, err)
	}
	if !bytes.Equal(result.TransportSource, transportWant) {
		t.Fatalf("generated java transport does not match golden:\n%s", lineDiff(transportWant, result.TransportSource))
	}

	// Check skeleton golden
	skeletonGoldenPath := filepath.Join("testdata", "CounterSkeleton.java.golden")
	skeletonWant, err := os.ReadFile(skeletonGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", skeletonGoldenPath, err)
	}
	if !bytes.Equal(result.SkeletonSource, skeletonWant) {
		t.Fatalf("generated java skeleton does not match golden:\n%s", lineDiff(skeletonWant, result.SkeletonSource))
	}
}

func TestGenerateJavaSkeletonCounterTransportShape(t *testing.T) {
	schemaPath := filepath.Join("testdata", "counter.toml")
	s, err := ParseFile(schemaPath)
	if err != nil {
		t.Fatalf("ParseFile(%s) returned error: %v", schemaPath, err)
	}

	if errs := Validate(s); len(errs) != 0 {
		t.Fatalf("Validate returned %d errors: %v", len(errs), errs)
	}

	result, err := GenerateJavaSkeleton(s, "io.jcrpc.counter.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	// Transport interface checks
	transportSrc := string(result.TransportSource)
	transportRequired := []string{
		"public interface CounterTransport {",
		"byte[] transmit(byte ins, byte p1, byte p2, byte[] data);",
	}
	for _, needle := range transportRequired {
		if !strings.Contains(transportSrc, needle) {
			t.Fatalf("generated java transport is missing required fragment %q", needle)
		}
	}

	// Skeleton checks
	skeletonSrc := string(result.SkeletonSource)
	skeletonRequired := []string{
		"public abstract class CounterSkeleton {",
		"protected final CounterTransport transport;",
		"protected CounterSkeleton(CounterTransport transport)",
		"public final byte[] dispatch(byte ins, byte p1, byte p2, byte[] data)",
	}
	for _, needle := range skeletonRequired {
		if !strings.Contains(skeletonSrc, needle) {
			t.Fatalf("generated java skeleton is missing required fragment %q", needle)
		}
	}

	forbidden := []string{
		"extends AppletBase",
		"import javacard.framework",
		"io.jcrpc.server.AppletBase",
	}
	for _, needle := range forbidden {
		if strings.Contains(skeletonSrc, needle) {
			t.Fatalf("generated java skeleton contains forbidden fragment %q", needle)
		}
	}
}

func TestGenerateJavaSkeletonSupportsASCIIFields(t *testing.T) {
	s := &Schema{
		Applet: Applet{
			Name: "Demo",
			AID:  "A000000001",
			CLA:  0x80,
		},
		Methods: map[string]*Method{
			"setImsi": {
				Name: "setImsi",
				INS:  0x01,
				Request: &Message{Fields: []Field{
					{Name: "imsi", Type: FieldTypeASCII, Length: intPtr(15), Location: ParameterLocationData},
				}},
			},
			"getImsi": {
				Name: "getImsi",
				INS:  0x02,
				Response: &Message{Fields: []Field{
					{Name: "imsi", Type: FieldTypeASCII},
				}},
			},
		},
	}

	result, err := GenerateJavaSkeleton(s, "io.example.demo")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	src := string(result.SkeletonSource)
	if !strings.Contains(src, "byte[] imsi = slice(requestData, 0, 15);") {
		t.Fatalf("generated java skeleton missing fixed-length ascii request decoding:\n%s", src)
	}
	if !strings.Contains(src, "protected abstract void onSetImsi(byte[] imsi);") {
		t.Fatalf("generated java skeleton missing ascii request abstract method:\n%s", src)
	}
	if !strings.Contains(src, "protected abstract byte[] onGetImsi();") {
		t.Fatalf("generated java skeleton missing ascii response abstract method:\n%s", src)
	}
}

func TestGenerateJavaSkeletonSupportsStringFields(t *testing.T) {
	s := &Schema{
		Applet: Applet{
			Name: "Demo",
			AID:  "A000000001",
			CLA:  0x80,
		},
		Methods: map[string]*Method{
			"echoMessage": {
				Name: "echoMessage",
				INS:  0x01,
				Request: &Message{Fields: []Field{
					{Name: "message", Type: FieldTypeString, Location: ParameterLocationData},
				}},
				Response: &Message{Fields: []Field{
					{Name: "message", Type: FieldTypeString},
				}},
			},
		},
	}

	result, err := GenerateJavaSkeleton(s, "io.example.demo")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	src := string(result.SkeletonSource)
	if !strings.Contains(src, "byte[] message = slice(requestData, 0, requestData.length - 0);") {
		t.Fatalf("generated java skeleton missing variable-length string request decoding:\n%s", src)
	}
	if !strings.Contains(src, "protected abstract byte[] onEchoMessage(byte[] message);") {
		t.Fatalf("generated java skeleton missing string abstract method:\n%s", src)
	}
}

func lineDiff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	maxLines := len(wantLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}

	var b strings.Builder
	b.WriteString("--- want\n")
	b.WriteString("+++ got\n")

	diffs := 0
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

		diffs++
		fmt.Fprintf(&b, "@@ line %d @@\n", i+1)
		fmt.Fprintf(&b, "- %s\n", w)
		fmt.Fprintf(&b, "+ %s\n", g)
		if diffs >= 40 {
			b.WriteString("... diff truncated ...\n")
			break
		}
	}

	return b.String()
}
