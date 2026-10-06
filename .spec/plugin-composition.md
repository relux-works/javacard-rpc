# Phase-one plugin composition

The `pluginapi` module owns the IDL model and the in-memory `Plugin.Generate`
contract. It has no third-party dependencies. Parsing TOML, semantic validation,
target selection, output packaging, and filesystem writes belong to the
`codegen` facade. Existing exported facade model names are type aliases;
generator entry points delegate to the preserved renderers.

The CLI explicitly composes `plugins/javacard.Plugin`, `plugins/swift.Plugin`,
and `plugins/kotlin.Plugin`. Each consumes `pluginapi.Schema` and options and
returns source bytes or an error. Their transitive compiled dependency graph
must exclude the root facade package and its TOML parser. Shared rendering
helpers remain in `codegen/internal/render` for this phase; templates and
rendering behavior are unchanged. Runtime repositories and consumers do not
move or change. Moving backends into target repositories, pinning their versions,
the runtime manifest, and releases are later phases.

The facade preserves all current flags, default names, exits, and generated
bytes. Streamed Swift selection, malformed simulator coordinates, invalid Java
stream-memory choices, and invalid schemas retain their existing refusals. These
preflight refusals must preserve existing files and produce no partial output.
Filesystem failure after writing has begun retains the existing CLI behavior;
this phase does not promise transactional rollback of I/O failures.

`jcrpc-parity` compares independently built baseline and candidate CLIs. It
discovers every example TOML IDL, adds both generator fixtures and a required
consumer IDL, and covers individual targets, combined Java/Kotlin, `--all`,
explicit overrides with `--all`, validation, help aliases, and no-selection
refusals. Generation cases run verbose with omitted/default/override simulator
coordinates and omitted/both explicit stream-memory modes. It compares exit
codes, stdout, diagnostics after replacing output-directory paths, and complete
relative file inventories with SHA-256 byte digests. It records exact commands,
input hashes, binary hashes, and each invocation's exit code in JSON. Missing or
unreadable input/output and non-regular output are errors, never evidence of an
empty tree. Metadata, permission bits, and timestamp parity are outside the
byte-identity contract. The intentional byte-change control must exit 1 with
`output drift`, rather than a setup or CLI failure.

`jcrpc-mutants` copies the two Go modules and example TOML corpus into fresh
disposable task-local fixtures. It first executes each selected named test
unmodified with `-count=1` and requires exit 0 plus that test's JSON pass event.
An invalid control refuses before planting. Then it applies one bounded
weakening and reruns the test. A kill requires exit 1, the named JSON failure
event, and the intended assertion diagnostic attributed to that test. Setup or
compilation failures, skips, malformed receipts, and missing assertion/failure
events cannot attest kills. Its table names every narrowing, failing test, and
survival bound. `--only NAME` selects one catalog entry and rejects unknown
names. Existing fixture directories refuse rather than inheriting stale files.
A token-preserving Java status mutation additionally drives the existing
behavioral JVM dispatch harness. Attestation regressions invoke the public
harness with bounded Go fixtures; those fixtures isolate receipt classification
and do not substitute for the real parity/JVM suites run by the catalog.

The revision-two v0.4.4 parity evidence is historical. Current parity uses signed
v0.4.5 (tag object `da77d07af5dc6866437d3db1004fdeda9738d59c`,
commit `cfed4182356a4f4609c88f58924aac79c05ae5b6`,
tree `cda7d28d89cea229af2603b79e850a9eb1e862d5`).
The independent model carries stream workspace policy. Lifecycle and simulator
coordinates must remain represented in the next API freeze, including target-owned
build manifests. Parser and validator
remain in the facade. Default, explicit transient and persistent outputs are
compared exactly, including both lifecycle modes and CLI refusal controls.
This import preserves the accepted composition and harness checkpoint; API
freeze and backend releases follow in separate leaves.

Source builds require the sibling `pluginapi` directory through the local
`replace` directive. Publishing the independent module and pinning released
plugin versions must precede any later facade release; versioned remote
installation is not asserted by this phase-one source change.
