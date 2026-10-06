package codegen

import "strings"

// Test-only source inspection; no exported renderer internals are needed.
var javaCodecHelpers = []string{
	"copyBytes",
	"readBool",
	"readU8",
	"readU16",
	"readU32",
	"slice",
}

// pruneUnusedJavaCodecHelpers removes every codec helper the finished skeleton
// never calls, repeating until nothing more drops out: removing one helper can
// orphan the only caller of another.
func javaHelperBlocks(source, name string) [][2]int {
	const closing = "\n    }\n"
	var blocks [][2]int
	for offset := 0; offset < len(source); {
		index := javaHelperDeclaration(source, name, offset)
		if index < 0 {
			return blocks
		}
		end := strings.Index(source[index:], closing)
		if end < 0 {
			return blocks
		}
		end += index + len(closing)
		blocks = append(blocks, [2]int{javaHelperBlockStart(source, index), end})
		offset = end
	}
	return blocks
}

// javaHelperDeclaration finds the next line that declares the named helper: a
// method declaration at class-member indentation whose name is followed by the
// parameter list. A call site is indented deeper, so it never matches.
func javaHelperDeclaration(source, name string, offset int) int {
	needle := " " + name + "("
	for search := offset; search < len(source); {
		index := strings.Index(source[search:], needle)
		if index < 0 {
			return -1
		}
		index += search
		lineStart := strings.LastIndexByte(source[:index], '\n') + 1
		line := source[lineStart:index]
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") &&
			(strings.HasPrefix(line, "    private ") || strings.HasPrefix(line, "    protected ")) {
			return lineStart
		}
		search = index + len(needle)
	}
	return -1
}

// javaHelperBlockStart walks back over the comment lines and the blank line that
// introduce a declaration, so removing the method removes its explanation too.
func javaHelperBlockStart(source string, declaration int) int {
	start := declaration
	for start > 0 {
		previous := strings.LastIndexByte(source[:start-1], '\n') + 1
		line := strings.TrimSpace(source[previous : start-1])
		if !strings.HasPrefix(line, "//") {
			break
		}
		start = previous
	}
	if start > 0 && strings.HasSuffix(source[:start], "\n\n") {
		start--
	}
	return start
}

func javaStreamFieldConfig(field *Field) (bool, int, int) {
	if field == nil {
		return false, 0, 0
	}
	return true, field.MaxLength, field.ChunkSize
}
