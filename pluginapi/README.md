# javacard-rpc plugin API

Module: `github.com/relux-works/javacard-rpc/pluginapi`, Go 1.24 or later.
The first proposed release is **v0.1.0**, from Git tag **pluginapi/v0.1.0**
in the javacard-rpc repository. The tag is prepared, not published. Before
publication use a local `replace` against the reviewed candidate. This module
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
| `Schema` | Parsed, normalized, semantically validated model supplied by the facade; treat it as read-only. No parser or validator belongs in a backend. |
| `Options.Namespace` | Java/Kotlin package or Swift module choice, including facade-resolved defaults. Source layout belongs to the backend. |
| `Options.StreamMemory` | Java transient lifecycle: `clear_on_deselect` or `clear_on_reset`; an empty value uses the existing Java default. Other adapters ignore it. |
| `Options.SimulatorDependency` | Facade-trimmed and validated `group:artifact:version` coordinate for Java's compile-only dependency, including the resolved CLI default `com.klinec:jcardsim:3.0.5.9`. Direct callers must supply the resolved coordinate. Other adapters ignore it. |
| `Schema.Applet.StreamWorkspace` | Bulk stream storage: empty/`transient`, or `persistent`. Lifecycle remains independent; persistent storage does not promise atomic interrupted wiping. |

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
unpublished API and checks the effective module/import graphs before building
and testing it. Run:

```sh
cd codegen
go test . -run '^(TestPluginAPIExternalConsumer|TestTargetAdaptersReturnWholePackages)$' -count=1 -v
```

See [release preparation](../.spec/plugin-api-release.md) for publication gates.
