package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The stream dispatcher decodes an instruction into a method id and an operation
// from a table instead of one switch case per instruction. This test compiles the
// generated skeleton against a recording runtime and checks every one of the 256
// instruction values: each of the six instructions of every streamed family must
// reach the runtime with that family's method id, operation and limits, and every
// other instruction must be refused with 6D00 without reaching the runtime at all.
// The schema below deliberately places two families back to back (0x50 and 0x56)
// and leaves holes elsewhere, so an off-by-one in the range check cannot pass.
func TestGeneratedJavaStreamDispatchDecodesEveryInstruction(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not available")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not available")
	}

	schema, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	schema.Methods["adjacentLow"] = &Method{
		Name: "adjacentLow",
		INS:  0x50,
		Request: &Message{Fields: []Field{{
			Name: "payload", Type: FieldTypeStream, MaxLength: 1024, ChunkSize: 128,
		}}},
		Response: &Message{Fields: []Field{
			{Name: "signatureLength", Type: FieldTypeU8},
			{Name: "signatureDerPadded", Type: FieldTypeBytesFixed, FixedLength: 72},
		}},
	}
	schema.Methods["adjacentHigh"] = &Method{
		Name: "adjacentHigh",
		INS:  0x56,
		Response: &Message{Fields: []Field{{
			Name: "result", Type: FieldTypeStream, MaxLength: 1536, ChunkSize: 192,
		}}},
	}
	if errs := Validate(schema); len(errs) != 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}
	result, err := GenerateJavaSkeleton(schema, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	frameworkDir := filepath.Join(root, "javacard", "framework")
	securityDir := filepath.Join(root, "javacard", "security")
	serverDir := filepath.Join(root, "io", "jcrpc", "streamdemo", "server")
	for _, dir := range []string{frameworkDir, securityDir, serverDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}
	files := map[string][]byte{
		filepath.Join(serverDir, result.TransportName+".java"):      result.TransportSource,
		filepath.Join(serverDir, result.SkeletonName+".java"):       result.SkeletonSource,
		filepath.Join(serverDir, result.StreamEndpointName+".java"): result.StreamEndpointSource,
		// The generated runtime is replaced by a recorder: this test is about what
		// the skeleton hands the runtime, not about what the runtime then does.
		filepath.Join(serverDir, result.StreamRuntimeName+".java"): []byte(streamDispatchRecordingRuntime),
		filepath.Join(serverDir, "StreamDispatchHarness.java"):     []byte(streamDispatchHarness),
		filepath.Join(frameworkDir, "ISOException.java"):           []byte(apduISOExceptionStub),
		filepath.Join(frameworkDir, "JCSystem.java"):               []byte(apduJCSystemStub),
		filepath.Join(securityDir, "MessageDigest.java"):           []byte(apduMessageDigestStub),
	}
	paths := make([]string, 0, len(files))
	for path, source := range files {
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if output, err := exec.Command(javac, append([]string{"-d", root}, paths...)...).CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, output)
	}
	output, err := exec.Command(java, "-cp", root, "io.jcrpc.streamdemo.server.StreamDispatchHarness").CombinedOutput()
	if err != nil {
		t.Fatalf("dispatch harness failed: %v\n%s", err, output)
	}

	want := expectedStreamDispatchRows(t, schema)
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ",", 2)
		if len(parts) != 2 {
			t.Fatalf("harness printed an unreadable row %q", line)
		}
		got[parts[0]] = parts[1]
	}
	if len(got) != 256 {
		t.Fatalf("harness reported %d instructions, want 256", len(got))
	}
	for ins := 0; ins <= 0xFF; ins++ {
		key := fmt.Sprintf("%02X", ins)
		if got[key] != want[key] {
			t.Errorf("INS %s decoded to %q, want %q", key, got[key], want[key])
		}
	}
}

// expectedStreamDispatchRows states, independently of the generator, what every
// instruction must do: method ids are assigned in ascending INS order, a streamed
// method owns exactly base..base+5, and the six operations are 0..5 in order.
func expectedStreamDispatchRows(t *testing.T, schema *Schema) map[string]string {
	t.Helper()
	type family struct {
		base   int
		fields []string
	}
	names := make([]string, 0, len(schema.Methods))
	for name := range schema.Methods {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := schema.Methods[names[i]], schema.Methods[names[j]]
		if a.INS == b.INS {
			return names[i] < names[j]
		}
		return a.INS < b.INS
	})

	families := make([]family, 0, len(names))
	for _, name := range names {
		method := schema.Methods[name]
		if !method.HasStream() {
			continue
		}
		requestEnabled, requestMax, requestChunk := javaStreamFieldConfig(streamFieldOf(method.Request))
		responseEnabled, responseMax, responseChunk := javaStreamFieldConfig(streamFieldOf(method.Response))
		exact := -1
		if !responseEnabled {
			exact = fixedResponseWidth(t, method)
		}
		families = append(families, family{
			base: int(method.INS),
			fields: []string{
				fmt.Sprintf("%t", requestEnabled),
				fmt.Sprintf("%d", requestMax),
				fmt.Sprintf("%d", requestChunk),
				fmt.Sprintf("%t", responseEnabled),
				fmt.Sprintf("%d", responseMax),
				fmt.Sprintf("%d", responseChunk),
				fmt.Sprintf("%d", exact),
			},
		})
	}

	rows := map[string]string{}
	for ins := 0; ins <= 0xFF; ins++ {
		rows[fmt.Sprintf("%02X", ins)] = "false,SW6D00"
	}
	for index, f := range families {
		for operation := 0; operation < 6; operation++ {
			key := fmt.Sprintf("%02X", f.base+operation)
			rows[key] = fmt.Sprintf("true,%d,%d,%s", index+1, operation, strings.Join(f.fields, ","))
		}
	}
	return rows
}

func streamFieldOf(message *Message) *Field {
	if message == nil {
		return nil
	}
	for i := range message.Fields {
		if message.Fields[i].Type == FieldTypeStream {
			return &message.Fields[i]
		}
	}
	return nil
}

// fixedResponseWidth mirrors the exact short-response length the generator passes
// for a streamed request whose response is a fixed-width record.
func fixedResponseWidth(t *testing.T, method *Method) int {
	t.Helper()
	rendered, err := renderMethod(method.Name, method)
	if err != nil {
		t.Fatalf("renderMethod %s: %v", method.Name, err)
	}
	return rendered.ExactShortResponseLength
}

const streamDispatchRecordingRuntime = `package io.jcrpc.streamdemo.server;

public final class StreamDemoBoundedStreamRuntime implements StreamDemoStreamEndpoint {
    public static final short SCALAR_COUNT = (short) 8;
    public static final short HANDLER_SLOT_COUNT = (short) 1;

    public static int calls;
    public static byte methodId;
    public static byte operation;
    public static boolean requestEnabled;
    public static short requestLimit;
    public static short requestChunk;
    public static boolean responseEnabled;
    public static short responseLimit;
    public static short responseChunk;
    public static short shortResponseLength;

    public StreamDemoBoundedStreamRuntime(
            byte[] workspace, byte[] digestScratch, short[] scalars,
            Object[] handlerSlot, Sha256 sha256) {
    }

    public short dispatch(
            byte methodIdIn, byte operationIn, boolean requestEnabledIn,
            short requestLimitIn, short requestChunkIn, boolean responseEnabledIn,
            short responseLimitIn, short responseChunkIn, short shortResponseLengthIn,
            Handler handler, byte p1, byte p2,
            byte[] requestBuffer, short requestOffset, short requestLength,
            byte[] responseBuffer, short responseOffset, short responseCapacity) {
        calls++;
        methodId = methodIdIn;
        operation = operationIn;
        requestEnabled = requestEnabledIn;
        requestLimit = requestLimitIn;
        requestChunk = requestChunkIn;
        responseEnabled = responseEnabledIn;
        responseLimit = responseLimitIn;
        responseChunk = responseChunkIn;
        shortResponseLength = shortResponseLengthIn;
        return (short) 0;
    }

    public void abort(byte reason) {
    }
}
`

const streamDispatchHarness = `package io.jcrpc.streamdemo.server;

import javacard.framework.ISOException;

public final class StreamDispatchHarness extends StreamDemoSkeleton {

    private StreamDispatchHarness() {
        super(null);
    }

    protected byte onGetVersion() {
        return (byte) 0;
    }

    protected short onProcessPacketStream(
            byte[] input, short inputOffset, short inputLength,
            byte[] output, short outputOffset, short outputCapacity) {
        return (short) 0;
    }

    protected short onIssueReportStream(
            byte[] input, short inputOffset, short inputLength,
            byte[] output, short outputOffset, short outputCapacity) {
        return (short) 0;
    }

    protected short onAdjacentLowStream(
            byte[] input, short inputOffset, short inputLength,
            byte[] output, short outputOffset, short outputCapacity) {
        return (short) 0;
    }

    protected short onAdjacentHighStream(
            byte[] input, short inputOffset, short inputLength,
            byte[] output, short outputOffset, short outputCapacity) {
        return (short) 0;
    }

    public static void main(String[] args) {
        StreamDispatchHarness harness = new StreamDispatchHarness();
        byte[] buffer = new byte[8];
        StringBuilder out = new StringBuilder();
        for (int value = 0; value <= 0xFF; value++) {
            byte ins = (byte) value;
            boolean isStream = harness.isStreamInstruction(ins);
            StreamDemoBoundedStreamRuntime.calls = 0;
            String row;
            try {
                harness.dispatchStreamTo(ins, (byte) 0, (byte) 0,
                        buffer, (short) 0, (short) 0, buffer, (short) 0, (short) 8);
                if (StreamDemoBoundedStreamRuntime.calls != 1) {
                    row = "reached-the-runtime-" + StreamDemoBoundedStreamRuntime.calls + "-times";
                } else {
                    row = isStream
                            + "," + StreamDemoBoundedStreamRuntime.methodId
                            + "," + StreamDemoBoundedStreamRuntime.operation
                            + "," + StreamDemoBoundedStreamRuntime.requestEnabled
                            + "," + StreamDemoBoundedStreamRuntime.requestLimit
                            + "," + StreamDemoBoundedStreamRuntime.requestChunk
                            + "," + StreamDemoBoundedStreamRuntime.responseEnabled
                            + "," + StreamDemoBoundedStreamRuntime.responseLimit
                            + "," + StreamDemoBoundedStreamRuntime.responseChunk
                            + "," + StreamDemoBoundedStreamRuntime.shortResponseLength;
                }
            } catch (ISOException failure) {
                row = isStream + ",SW" + String.format("%04X", failure.sw & 0xFFFF);
                if (StreamDemoBoundedStreamRuntime.calls != 0) {
                    row = "refused-but-still-reached-the-runtime";
                }
            }
            out.append(String.format("%02X", value)).append(',').append(row).append('\n');
        }
        System.out.print(out);
    }
}
`
