# Released plugin composition

The `pluginapi` module owns the IDL model and the in-memory `Plugin.Generate`
contract. It has no third-party dependencies. Parsing TOML, semantic validation,
target selection, canonical target roots, and filesystem writes belong to the
`codegen` facade. Existing exported facade model names are type aliases;
generator entry points delegate to the published target Plugin.Generate APIs.

The CLI explicitly composes the released Java Card, Swift and Kotlin `codegen.Plugin` packages. Each consumes `pluginapi.Schema` and options and
returns the complete ordered source/build-manifest package or an error.
Adapters own source layout and Gradle/SPM templates; the CLI validates returned
relative paths before writing a selected target package. The frozen API carries
namespace, stream lifecycle and the resolved simulator coordinate. See
[API contract](../pluginapi/README.md) and [release preparation](plugin-api-release.md). Their transitive compiled dependency graph
must exclude the root facade package and its TOML parser. Target rendering and package templates live exclusively in their released
repositories. The facade pins their exact public modules; see the
[compatibility manifest and offline consumer flows](../compatibility/README.md).

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
coordinates are represented in the API freeze, including target-owned
build manifests. Parser and validator
remain in the facade. Default, explicit transient and persistent outputs are
compared exactly, including both lifecycle modes and CLI refusal controls.
The API freeze preserves the accepted composition/port checkpoint and
compatibility wrappers. Independent API publication and later backend releases
follow the parent-owned release sequence.

Source builds resolve pluginapi v0.1.1 and the three exact published backend
modules without local replacements. Mutation fixtures may replace copied modules
only inside disposable test trees. Production dependencies remain replace-free.
