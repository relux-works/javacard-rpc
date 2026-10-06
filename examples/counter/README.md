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
