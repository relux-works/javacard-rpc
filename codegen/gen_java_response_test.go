package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedJavaSkeletonRequiresExactFixedResponseLength(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not available")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not available")
	}

	schema := &Schema{
		Applet: Applet{Name: "FixedDemo", AID: "A000000001", CLA: 0x80},
		Methods: map[string]*Method{
			"getInfo": {
				Name: "getInfo",
				INS:  0x01,
				Response: &Message{Fields: []Field{
					{Name: "schema", Type: FieldTypeU8},
					{Name: "identity", Type: FieldTypeBytesFixed, FixedLength: 10},
					{Name: "generation", Type: FieldTypeU32},
				}},
			},
		},
	}
	result, err := GenerateJavaSkeleton(schema, "io.jcrpc.fixed")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	packageDir := filepath.Join(root, "io", "jcrpc", "fixed")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("create package directory: %v", err)
	}
	writeTestFile(t, filepath.Join(packageDir, result.TransportName+".java"), result.TransportSource)
	writeTestFile(t, filepath.Join(packageDir, result.SkeletonName+".java"), result.SkeletonSource)
	writeTestFile(t, filepath.Join(packageDir, "FixedResponseHarness.java"), []byte(fixedResponseHarness))

	compile := exec.Command(javac,
		filepath.Join(packageDir, result.TransportName+".java"),
		filepath.Join(packageDir, result.SkeletonName+".java"),
		filepath.Join(packageDir, "FixedResponseHarness.java"),
	)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, output)
	}
	run := exec.Command(java, "-cp", root, "io.jcrpc.fixed.FixedResponseHarness")
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("Java response-length harness failed: %v\n%s", err, output)
	}
}

const fixedResponseHarness = `package io.jcrpc.fixed;

public final class FixedResponseHarness {
    public static void main(String[] args) {
        assertAccepted(15);
        assertRejected(14);
        assertRejected(16);
    }

    private static void assertAccepted(int length) {
        byte[] response = new Logic(length).dispatch((byte) 0x01, (byte) 0, (byte) 0, null);
        if (response.length != length) {
            throw new AssertionError("unexpected response length " + response.length);
        }
    }

    private static void assertRejected(int length) {
        try {
            new Logic(length).dispatch((byte) 0x01, (byte) 0, (byte) 0, null);
            throw new AssertionError("accepted " + length + "-byte fixed response");
        } catch (FixedDemoSkeleton.StatusWordException expected) {
            if (expected.getStatusWord() != (short) 0x6700) {
                throw new AssertionError("unexpected status word");
            }
        }
    }

    private static final class Logic extends FixedDemoSkeleton {
        private final byte[] response;

        Logic(int length) {
            super(new FixedDemoTransport() {
                public byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {
                    return new byte[0];
                }
            });
            response = new byte[length];
        }

        protected byte[] onGetInfo() {
            return response;
        }
    }
}
`
