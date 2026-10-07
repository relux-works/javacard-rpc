# javacard-rpc plugin API

Module: `github.com/relux-works/javacard-rpc/pluginapi`, Go 1.24 or later.
The released baseline is **v0.1.0**, from Git tag **pluginapi/v0.1.0**
in the javacard-rpc repository. **v0.1.1** is prepared here for parent publication
under **pluginapi/v0.1.1**; it is not yet published. Before publication use a
local `replace` against the reviewed candidate. This module
contains only the shared model, options, ordered in-memory files and backend
interface. It has no parser, semantic validator, target runtime, templates,
registration registry or third-party module dependencies.

Implement `pluginapi.Plugin` in an independent Go module:

```go
var _ pluginapi.Plugin = Backend{}

func (Backend) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
    // Render the complete target package in memory from the validated model.
    return []pluginapi.File{
        {Name: "build.manifest", Data: []byte(s.Applet.Version)},
        {Name: "src/backend.txt", Data: []byte(o.Namespace)},
    }, nil
}
```

Registration means explicit compile-time composition: the facade imports a
backend and supplies its `Plugin` implementation to the selected target slot.
The bundled CLI composes `javacard.Plugin{}`, `swift.Plugin{}` and
`kotlin.Plugin{}` in `run`, calling them via `runWithPlugins`. These CLI helpers
are internal, not a public registration function. External backends implement
only the public `Plugin.Generate` interface; using them in the stock CLI
requires a separately reviewed composition change. Dynamic discovery and
third-party CLI registration are not supported by v0.1.0.

| Input | Semantics |
| --- | --- |
| `Schema` | Parsed, normalized, semantically validated model supplied by the facade; treat it as read-only. No parser belongs in a backend; direct backend calls still need validation of the policies that backend implements. |
| `Options.Namespace` | Java/Kotlin package or Swift module choice, including facade-resolved defaults. Source layout belongs to the backend. |
| `Options.StreamMemory` | Java transient lifecycle: `clear_on_deselect` or `clear_on_reset`; an empty value uses the existing Java default. Other adapters ignore it. |
| `Options.SimulatorDependency` | Facade-trimmed and validated `group:artifact:version` coordinate for Java's compile-only dependency, including the resolved CLI default `com.klinec:jcardsim:3.0.5.9`. Direct callers must supply the resolved coordinate. Other adapters ignore it. |
| `Schema.Applet.StreamWorkspace` | Bulk stream storage: empty/`transient`, or `persistent`. Lifecycle remains independent; persistent storage does not promise atomic interrupted wiping. |
| `Schema.Applet.StreamWorkspaceCleanup` | Plain string cleanup selector. Empty preserves released generation, including the existing persistent default. Explicit modes apply only with `StreamWorkspace == "persistent"`. Unknown selectors and nonpersistent combinations are validation responsibilities of the facade and direct backend entry point. |

The two explicit cleanup names are centralized in this module:

| Constant | Value | Persistent cleanup contract |
| --- | --- | --- |
| `StreamWorkspaceCleanupWholeReplyArea` | `"whole-reply-area"` | Track request stores exactly; after handler execution wipe the authorized reply area. Skip untouched workspace. |
| `StreamWorkspaceCleanupWrittenBytesOnly` | `"written-bytes-only"` | Supply a bounded tracked writer with bulk `Util.arrayCopy` methods and no raw writable workspace escape; wipe its actual written range. |

This API carries metadata only. It does not implement either cleanup mode,
validate selectors, or change generated behavior. Parser/facade and Java Card
implementation follow downstream after the API release. Adding the field
preserves zero values, keyed literals and the `Plugin` interface; positional
`Applet` literals must add the new trailing field. Interrupted persistent wiping
still has no atomic secure-erasure guarantee.

`Generate` returns the whole target package: source files and root Gradle/SPM
manifests, in write/verbose order. `File.Name` is a canonical slash-separated
relative path inside the facade-selected target root. It is neither a bare
source basename nor an output-directory override. Empty paths, `.`, `..`, dot
segments, repeated/trailing separators, absolute paths, NUL, nonlocal native
paths, native separator aliases, exact duplicate names and file/directory
conflicts are invalid. Paths must satisfy `fs.ValidPath`, be other than `.`,
remain local under `filepath.IsLocal(filepath.FromSlash(name))` and use `/`
as their only directory separator on the writing host. On the supported POSIX
filesystem, `:` and `\` are literal filename characters: `C:/file` is a relative
directory/file pair and `src\file` is one filename. They are not rejected as
Windows path syntax on POSIX. Java/Kotlin namespace acceptance remains that of
v0.4.5, including literal colon/backslash inputs; this is a generation/path
contract, not a promise that those namespaces compile in Java/Kotlin. `Data` must be
non-nil; an empty non-nil slice represents an empty file. An empty package is
invalid. Existing symlinks at/below the target root are refused before writes.
The contract is not a sandbox against a concurrently modified filesystem, a
malicious Go implementation doing its own I/O, or case-folding path collisions.

The facade selects `<stem>-server-javacard`, `<stem>-client-swift` and
`<stem>-client-kotlin` under `--out-dir`; adapters own paths below those roots.
Java/Kotlin sources use `src/main/<language>/<package/path>`; Swift retains
`Sources/<AppletClient>`. These layouts and runtime ecosystem names are fixed.

The facade validates the entire returned file set before creating or writing
that target package. Invalid plugin output and generation errors return CLI
exit 2 without changing that target's files, including when an error accompanies
nonempty files. Ordinary filesystem failures return exit 3 and can leave partial
output. Previously written successful targets remain if a later target fails.
There is no atomic multi-target or arbitrary I/O rollback guarantee.

The separate module fixture in `codegen/testdata/external-plugin` requires only
this API. `TestPluginAPIExternalConsumer` copies it outside the repository,
uses `GOWORK=off`, disables network module resolution, replaces only this
candidate API and checks the effective module/import graphs before building
and testing it. Run:

```sh
cd codegen
go test . -run '^(TestPluginAPIExternalConsumer|TestTargetAdaptersReturnWholePackages)$' -count=1 -v
```

## Tools and focused checks

| Tool | Purpose and command | Outputs |
| --- | --- | --- |
| Go | Model tests plus an independent API-only consumer: `go -C pluginapi test ./... -count=1 -v` | Go temporary test directories/cache; capture task logs under `.temp/` |
| Go | Compile and lint: `go -C pluginapi build ./...`; `go -C pluginapi vet ./...` | Go build cache |
| gofmt | Format Go sources: `gofmt -w pluginapi/*.go pluginapi/testdata/*.go` | Updated source files |

`TestCleanupAPIIndependentConsumer` copies the API-local legacy backend fixture and new
cleanup checks into a separate module, uses `GOWORK=off`, `GOPROXY=off` and
`GOSUMDB=off`, replaces only the candidate API, verifies the two-module consumer
graph and dependency-free production API import graph, and builds/tests it.
This proves local source compatibility, not public availability of v0.1.1.

See [v0.1.1 release preparation](RELEASE-v0.1.1.md) for the parent publication
handoff, and [original API release preparation](../.spec/plugin-api-release.md)
for the historical v0.1.0 preparation record.
