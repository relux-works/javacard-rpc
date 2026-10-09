# Counter Walkthrough

`examples/counter` is the reference end-to-end example for `javacard-rpc`.

It now covers both text flavors:

- `ascii` for narrow text payloads like IMSI digits
- dynamic UTF-8 `string` for user-facing text roundtrips

It also ships two host-side executables against the same bridge and applet:

- Swift executable in `examples/counter/cli`
- Kotlin/JVM executable in `examples/counter/kotlin-cli`

## Source of Truth

- Contract schema: `examples/counter/counter.toml`
- Generated client package: `examples/counter/generated/counter-client-swift`
- Generated Kotlin client package: `examples/counter/generated/counter-client-kotlin`
- Generated server package: `examples/counter/generated/counter-server-javacard`

The generated directory is intentionally gitignored. Recreate it with `make generate`.

The Java Card v0.4.0 backend uses ordinary `dispatchTo` and caller-owned output
spans. `CounterJCApplet.process` captures headers, receives every supported short
input fragment into the APDU buffer, and sends only the successful produced span.
Byte/packed callbacks write directly; byte inputs are borrowed span triples.
The mock signature consumes needed input into install-time owned scratch before
overlap. No command-time request/reply arrays or retained APDU binding are used.
See [migration and capacity bounds](../../RELEASE-NOTES-0.5.0.md).

Classic qualification converts the generated `counter` library and the real
`io.jcrpc.counter.example` wrapper/business package separately, preserving both
package identities and importing the library's JAR/EXP. With the pinned kit and
Ant task, JDK 11, and an explicit JVM lease:

```sh
go -C codegen test . -run '^TestCounterWriterClassicCAP$' -count=1 -v
```

Set `JCRPC_ANT_JAVACARD_JAR` and `JCRPC_JCKIT_DIR` to the prepared toolchain;
optional `JCRPC_COUNTER_CAP_OUT=.temp/TASK-ID/caps` preserves CAP artifacts.
The test compiles against the exact SDK API (erasing only the source-only
`@Override` annotation in temporary API-check copies), plants a host-only Math
call and requires a specific missing-symbol refusal. Ant converts the original
sources and verifies both CAPs before and after exact plant restoration.
Oracle 3.0.5u4 crashes internally on unresolved Math calls; that crash is not
counted as a successful control. A separate static-builder plant requires the
converter's specific unsupported `invokestatic in clinit` refusal, then restores
the exact source and reconverts/verifies successfully. Mock SPKI construction occurs at installation,
because Classic static initializers cannot invoke builders. Simulator and
physical qualifications remain distinct.

## From TOML to Generated Artifacts

Run:

```bash
make generate
```

This does two things:

1. Builds `codegen/jcrpc-gen`
2. Generates:
   - `examples/counter/generated/counter-client-swift`
   - `examples/counter/generated/counter-client-kotlin`
   - `examples/counter/generated/counter-server-javacard`

## Client Side

The executable lives in `examples/counter/cli`.

Its dependencies are:

- generated package `../generated/counter-client-swift`
- runtime package `../../../../javacard-rpc-client-swift`

Build it with:

```bash
make build-cli
```

Run it directly with:

```bash
make run-example
```

The Kotlin/JVM executable lives in `examples/counter/kotlin-cli`.

Its dependencies are:

- generated package `../generated/counter-client-kotlin`
- runtime package `../../../../javacard-rpc-client-kotlin`

Build it with:

```bash
make build-kotlin-cli
```

Run it directly with:

```bash
make run-kotlin-example
```

## Service Side

The service path is:

1. generated server skeleton package in `examples/counter/generated/counter-server-javacard`
2. hand-written applet in `examples/counter/applet`
3. jCardSim TCP bridge in `bridge`

Build the bridge jar with:

```bash
make build-bridge
```

Build the applet with:

```bash
make build-applet
```

Start the bridge with the counter applet loaded:

```bash
make run-bridge
```

`make build-bridge` publishes the actual Gradle archive and resolved runtime
dependencies in `bridge/build/launch/classpath.txt`, with SHA-256 checksums in
`checksums.sha256`. `examples/counter/run-bridge.sh` uses that classpath plus:

- `examples/counter/applet/build/libs/counter-applet-0.1.0.jar`
- `examples/counter/generated/counter-server-javacard/build/libs/counter-server-javacard-1.0.0.jar`

The launcher refuses missing build metadata, missing/multiple bridge archives,
or changed build inputs, archives and dependencies with a named diagnostic and
exit 2 before starting Java. Remove obsolete archives from `bridge/build/libs`
and rebuild if the directory is ambiguous. `JCRPC_SKIP_BUILD=1` skips both builds
and performs the same checks without invoking Gradle. CLI arguments such as
`--port`, `--card-provider` and `--card-scope` are forwarded unchanged.
The receipt binds the existing main-source files and build descriptors at build
time; it does not detect newly added source files or authenticate local files
against a malicious writer. Rebuild after source changes. Build paths are local
to the checkout; rebuild after moving a built checkout.

Run the bounded launcher controls and their narrowing mutants with:

```bash
cd codegen && go test . -run '^TestCounterLauncher' -count=1 -v
```

The controls use disposable files and a Java argv spy; `make e2e` exercises the
actual built bridge and both real runtime clients.

## One-Shot E2E

For the full happy path, use:

```bash
make e2e
```

This runs `examples/counter/run-e2e.sh`, which:

1. regenerates and builds the example
2. starts the bridge
3. waits for bridge readiness
4. runs the Swift executable against the live bridge
5. runs the Kotlin executable against the same live bridge

## Suggested Learning Path

1. Read `examples/counter/counter.toml`
2. Run `make generate`
3. Inspect generated Swift client in `examples/counter/generated/counter-client-swift/Sources/CounterClient/CounterClient.swift`
4. Inspect generated Kotlin client in `examples/counter/generated/counter-client-kotlin/src/main/kotlin/counter/CounterClient.kt`
5. Inspect generated Java skeleton in `examples/counter/generated/counter-server-javacard/src/main/java/counter/CounterSkeleton.java`
6. Read the hand-written applet in `examples/counter/applet/src/main/java/io/jcrpc/example/CounterApplet.java`
7. Run `make e2e`
