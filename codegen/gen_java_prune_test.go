package codegen

import (
	"path/filepath"
	"strings"
	"testing"
)

// The skeleton template carries every private decode helper; a schema calls only
// some of them. The unused bodies are removed from the generated file and the
// called ones are not, or the consuming project stops compiling.
func TestGeneratedJavaSkeletonKeepsOnlyThePrivateHelpersItCalls(t *testing.T) {
	for _, schemaFile := range []string{"counter.toml", "stream.toml"} {
		t.Run(schemaFile, func(t *testing.T) {
			schema, err := ParseFile(filepath.Join("testdata", schemaFile))
			if err != nil {
				t.Fatalf("ParseFile returned error: %v", err)
			}
			result, err := GenerateJavaSkeleton(schema, "io.jcrpc.demo.server")
			if err != nil {
				t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
			}
			skeleton := string(result.SkeletonSource)
			for _, name := range javaCodecHelpers {
				declared := len(javaHelperBlocks(skeleton, name)) > 0
				called := codecHelperIsCalled(skeleton, name)
				if called && !declared {
					t.Errorf("helper %q is called but was pruned", name)
				}
				if !called && declared {
					t.Errorf("helper %q is never called but survived pruning", name)
				}
			}
			// The protected pack* helpers stay whatever the IDL does: a hand-written
			// applet extending the skeleton may pack its own response with them, and
			// the generator cannot see those call sites.
			for _, kept := range []string{
				"    protected final int packU8(",
				"    protected final int packBool(",
				"    protected final int packU16(",
				"    protected final int packU32(",
				"    protected final int packBytes(",
			} {
				if !strings.Contains(skeleton, kept) {
					t.Errorf("a protected helper that consumers may call was pruned: %q", kept)
				}
			}
		})
	}
}

// The two fixtures must actually disagree, or the test above would pass on a
// generator that prunes nothing.
func TestGeneratedJavaSkeletonPruningIsVisibleBetweenSchemas(t *testing.T) {
	counter := generatedSkeletonFor(t, "counter.toml")
	stream := generatedSkeletonFor(t, "stream.toml")

	// counter.toml reads a u32 request field, a bool in p1 and a trailing bytes
	// field; stream.toml reads none of them, so those helpers must be gone there.
	for _, name := range []string{"readU32", "readBool", "readU8", "slice"} {
		if len(javaHelperBlocks(counter, name)) == 0 {
			t.Errorf("counter.toml calls %q, so it must be declared", name)
		}
		if len(javaHelperBlocks(stream, name)) != 0 {
			t.Errorf("stream.toml never calls %q, so it must not be declared", name)
		}
	}
}

func generatedSkeletonFor(t *testing.T, schemaFile string) string {
	t.Helper()
	schema, err := ParseFile(filepath.Join("testdata", schemaFile))
	if err != nil {
		t.Fatalf("ParseFile %s returned error: %v", schemaFile, err)
	}
	result, err := GenerateJavaSkeleton(schema, "io.jcrpc.demo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton %s returned error: %v", schemaFile, err)
	}
	return string(result.SkeletonSource)
}

// codecHelperIsCalled reports whether anything outside the helper's own
// declarations mentions it.
func codecHelperIsCalled(source, name string) bool {
	blocks := javaHelperBlocks(source, name)
	remainder := source
	for i := len(blocks) - 1; i >= 0; i-- {
		remainder = remainder[:blocks[i][0]] + remainder[blocks[i][1]:]
	}
	return strings.Contains(remainder, name+"(")
}
