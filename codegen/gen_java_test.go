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
