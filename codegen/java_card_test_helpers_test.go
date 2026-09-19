package codegen

import (
	"os"
	"path/filepath"
	"testing"
)

const javaCardJCSystemStubSource = `package javacard.framework;
import java.util.IdentityHashMap;
public final class JCSystem {
    public static final byte CLEAR_ON_RESET = 0;
    public static final byte CLEAR_ON_DESELECT = 1;
    private static final IdentityHashMap<Object, Boolean> TRANSIENT_ARRAYS =
            new IdentityHashMap<Object, Boolean>();
    private JCSystem() { }
    private static <T> T registerTransient(T array) {
        TRANSIENT_ARRAYS.put(array, Boolean.TRUE);
        return array;
    }
    public static boolean isTransientForTest(Object array) {
        return TRANSIENT_ARRAYS.containsKey(array);
    }
    public static byte[] makeTransientByteArray(short length, byte event) {
        return registerTransient(new byte[length]);
    }
    public static short[] makeTransientShortArray(short length, byte event) {
        return registerTransient(new short[length]);
    }
    public static Object[] makeTransientObjectArray(short length, byte event) {
        return registerTransient(new Object[length]);
    }
}
`

// The generated skeleton and stream endpoint use the real Java Card transient
// allocation API. JVM harnesses provide this narrow behavioral stand-in so the
// generated sources can still be compiled and exercised without a card SDK.
func writeJavaCardJCSystemStub(t *testing.T, root string) string {
	t.Helper()

	return writeJavaCardJCSystemStubAt(t, filepath.Join(root, "javacard", "framework"))
}

func writeJavaCardJCSystemStubAt(t *testing.T, directory string) string {
	t.Helper()

	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("MkdirAll Java Card framework stub: %v", err)
	}
	path := filepath.Join(directory, "JCSystem.java")
	if err := os.WriteFile(path, []byte(javaCardJCSystemStubSource), 0o644); err != nil {
		t.Fatalf("write Java Card framework stub: %v", err)
	}
	return path
}
