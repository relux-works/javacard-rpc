# javacard-rpc bridge v0.3.2

## Generated server build.gradle now also declares mavenLocal()

0.3.1 added `mavenLocal()` to `bridge/build.gradle` only. The generated
`<applet>-server-javacard/build.gradle` still emitted:

```gradle
repositories {
    mavenCentral()
}
```

which made `--simulator-dependency` unusable for a consumer pinned to a fork
that is only available via `./gradlew publishToMavenLocal` and never pushed to
Maven Central — for example `bsimId` on `works.relux:jcardsim:3.0.5.9-relux.2`.
Passing `--simulator-dependency works.relux:jcardsim:3.0.5.9-relux.2` produced
a generated build that could not resolve the artefact without hand-editing the
generated output, which contradicts the "never edit generated output" contract
from 0.3.1.

The generated `build.gradle` now emits:

```gradle
repositories {
    mavenCentral()
    mavenLocal()
}
```

This is the exact one-line diff in generated output (`+    mavenLocal()`); the
`bridge/build.gradle` `repositories` block is unchanged from 0.3.1, which
already declared both.

## Tests

- `TestRunStreamBuildGradleCompilesWithMavenLocalOnlyOverride` (new): runs
  `jcrpc-gen` end to end for the stream package with `--simulator-dependency`
  set to a coordinate published only into a temporary, offline-only Maven
  local repository (never touching the real `~/.m2`), then compiles the
  generated `build.gradle` with `--offline` and asserts it resolves the
  dependency purely from `mavenLocal()`. This proves the generated
  `mavenLocal()` line is load-bearing for the flag, not decorative: with the
  fork present only in `mavenLocal()`, resolution fails if that line is
  missing.
- Paired negative: the same test asserts compilation fails when the
  `mavenLocal()`-only coordinate is generated against a build.gradle that
  declares `mavenCentral()` only (the pre-0.3.2 shape), so the test would have
  caught the 0.3.1 gap. This is the narrowing mutant for the gate: it removes
  `mavenLocal()` from the generated repositories block while keeping
  `mavenCentral()` and the rest of the template intact, and the compile step
  fails as required.

## Compatibility

- No wire protocol or IDL change.
- CardProvider SPI and `--card-scope` are unchanged.
- `javacard-rpc-client-kotlin` stays on `0.2.0`; existing Kotlin and Swift
  clients work against the 0.3.2 bridge without changes.
- With no `--simulator-dependency` flag, generated output is byte-identical to
  0.3.1 apart from the added `mavenLocal()` line in the generated
  `repositories` block (present unconditionally, independent of the flag).
  `bridge/build.gradle` is unchanged in content from 0.3.1 (version bump only).

## Verification at the release commit

- `make generate` — builds the counter example into `examples/counter/generated` (gitignored); it does not regenerate the checked-in `codegen/testdata/*.golden` fixtures.
- `make test` — codegen unit tests, including
  `TestRunStreamBuildGradleCompilesWithMavenLocalOnlyOverride` and its negative
  counterpart.
- `make test-bridge` — bridge JUnit 5 suite.
- `make release-check` — full gate (codegen, bridge, counter applet tests,
  generated Kotlin stream client harness, generated Java stream package → CAP
  conversion) run with `JCRPC_JCKIT_DIR` pointed at the local `jc320v25.1`
  JavaCard kit and `JCRPC_ANT_JAVACARD_JAR` pointed at the matching
  `ant-javacard.jar`. `ant` must be on `PATH`. Example, run from the repo
  root:

  ```bash
  JCRPC_ANT_JAVACARD_JAR=/path/to/ant-javacard.jar \
  JCRPC_JCKIT_DIR=/path/to/oracle_javacard_sdks/jc320v25.1_kit \
  make release-check
  ```
