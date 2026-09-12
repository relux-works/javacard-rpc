# javacard-rpc bridge v0.3.1

## Pinning the simulator artefact

Both the generated Java Card server build and the bridge JVM compile against
`com.klinec:jcardsim:3.0.5.9` by default. A consumer pinned to another
simulator artefact — for example `bsimId`, which uses the
`relux-works/jcardsim` fork `works.relux:jcardsim:3.0.5.9-relux.1` because
upstream refuses `externalAccess=true` engines — never edits generated output
or the bridge build; it passes the coordinate in:

| Where | How | Default |
| --- | --- | --- |
| generated `<applet>-server-javacard/build.gradle` (`compileOnly`) | `jcrpc-gen --simulator-dependency works.relux:jcardsim:3.0.5.9-relux.1 ...` | `com.klinec:jcardsim:3.0.5.9` |
| `bridge/build.gradle` (`implementation`) | `cd bridge && ./gradlew build -PjcardsimDependency=works.relux:jcardsim:3.0.5.9-relux.1`, or `jcardsimDependency=...` in `gradle.properties` / `~/.gradle/gradle.properties` | `com.klinec:jcardsim:3.0.5.9` |

The value must be a plain `group:artifact:version` triple; anything else is
refused before generation (`jcrpc-gen` exit code 2, nothing written) or at
Gradle configuration time (`invalid jcardsimDependency`). The generated
`build.gradle` is the record of which coordinate a package was generated
against — there is no separate manifest. `bridge/build.gradle` now also adds
`mavenLocal()` to its repositories, so a fork published with
`./gradlew publishToMavenLocal` resolves without further repository setup.
With the default coordinate, generated output and the resolved bridge
dependency are byte-identical to 0.3.0.

This is how `bsimId` builds:

```bash
jcrpc-gen --all --out-dir ./gen --simulator-dependency works.relux:jcardsim:3.0.5.9-relux.1 keyvault.toml
cd bridge && ./gradlew build -PjcardsimDependency=works.relux:jcardsim:3.0.5.9-relux.1
```

## Compatibility

- No wire protocol or IDL change.
- CardProvider SPI and `--card-scope` from 0.3.0 are unchanged.
- `javacard-rpc-client-kotlin` stays on `0.2.0`; existing Kotlin and Swift
  clients work against the 0.3.1 bridge without changes.
- With no `--simulator-dependency` flag and no `jcardsimDependency` Gradle
  property, behaviour is byte-identical to 0.3.0.

## Verification at the release commit

- `make test` — codegen unit tests, including the new
  `--simulator-dependency` coverage.
- `make test-bridge` — bridge JUnit 5 suite, including the new
  `jcardsimDependency` Gradle property validation.
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
