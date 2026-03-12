# Counter Walkthrough

`examples/counter` is the reference end-to-end example for `javacard-rpc`.

It now covers both text flavors:

- `ascii` for narrow text payloads like IMSI digits
- dynamic UTF-8 `string` for user-facing text roundtrips

## Source of Truth

- Contract schema: `examples/counter/counter.toml`
- Generated client package: `examples/counter/generated/counter-client-swift`
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

`examples/counter/run-bridge.sh` assembles the classpath from:

- `bridge/build/libs/jcrpc-bridge-0.1.0.jar`
- `examples/counter/applet/build/libs/counter-applet-0.1.0.jar`
- `examples/counter/generated/counter-server-javacard/build/libs/counter-server-javacard-1.0.0.jar`
- `jcardsim` and `smartcardio`

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

## Suggested Learning Path

1. Read `examples/counter/counter.toml`
2. Run `make generate`
3. Inspect generated Swift client in `examples/counter/generated/counter-client-swift/Sources/CounterClient/CounterClient.swift`
4. Inspect generated Java skeleton in `examples/counter/generated/counter-server-javacard/src/main/java/counter/CounterSkeleton.java`
5. Read the hand-written applet in `examples/counter/applet/src/main/java/io/jcrpc/example/CounterApplet.java`
6. Run `make e2e`
