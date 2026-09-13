---
name: javacard-rpc
description: RPC framework for Java Card smart card applets. TOML IDL -> codegen -> typed Swift and Kotlin clients plus Java skeletons with DI transport abstraction.
triggers:
  - javacard-rpc
  - applet rpc
  - java card codegen
  - apdu codegen
  - smart card rpc
---

# javacard-rpc

RPC framework for Java Card smart card applets. Define APDU contracts in TOML, generate typed host clients and Java Card skeletons.

## Architecture

```
counter.toml (IDL)
    │
    ▼
jcrpc-gen (codegen CLI)
    │
    ├─→ counter-client-swift/     (generated, zero-dep)
    │     CounterTransport        (protocol — DI)
    │     CounterClientProtocol   (business interface)
    │     CounterClient           (actor)
    │
    ├─→ counter-client-kotlin/    (generated, zero-dep)
    │     CounterTransport        (interface — DI)
    │     CounterClientProtocol   (business interface)
    │     CounterClient           (host client)
    │
    └─→ counter-server-javacard/  (generated, zero-dep)
          CounterTransport        (interface — DI)
          CounterSkeleton         (abstract class)
```

Generated packages have **zero external dependencies**. Transport is injected via DI:

- **Client (Swift):** host conforms concrete transport to generated `<Name>Transport` protocol via extension
- **Client (Kotlin):** host wraps concrete transport in adapter implementing generated `<Name>Transport` interface
- **Server (Java):** host wraps concrete transport in adapter implementing generated `<Name>Transport` interface

## Quick Start

```bash
# Build codegen
cd codegen && go build -o jcrpc-gen ./cmd/jcrpc-gen

# Generate from IDL
jcrpc-gen --all --out-dir ./examples/counter/generated ./examples/counter/counter.toml

# Or use Makefile
make generate

# Full dual-client e2e
make e2e
```

Canonical generated outputs for the counter example:

- `examples/counter/generated/counter-client-swift`
- `examples/counter/generated/counter-client-kotlin`
- `examples/counter/generated/counter-server-javacard`

## IDL Format

See [.spec/idl.md](.spec/idl.md) for the full specification.

TOML IDL defines: applet metadata, methods (INS, request/response fields), status words.

**Field types:** `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`, `stream`

`stream` is a bounded opaque byte array transported through six consecutive
instructions: write/invoke, close write, recover pending result descriptor,
read result chunk, close read, and abort. It is half-duplex and supports a
request stream, a response stream, or both. Use it for payloads that do not fit
one short APDU; do not add a general-purpose serializer. The application owns
the byte format inside the stream. Stream generation currently supports the
Java Card server and Kotlin client, not Swift; use explicit `--java` and
`--kotlin` outputs instead of `--all` for a stream schema.

Stream lifecycle code is generated, not supplied by an applet or application
implementation:

- the Java skeleton owns one applet-level session manager shared by all streamed
  methods, one transient workspace sized to the largest declared stream, digest
  scratch, reset detection, and reusable failure objects;
- the generated Java APDU adapter receives every incoming fragment before
  dispatch and sends from preallocated transient storage;
- applet code implements only the generated typed buffer handler and uses
  `failStream(statusWord)` for a business failure;
- the Kotlin client owns upload, idempotent recovery, pull, digest verification,
  close, abort, cancellation cleanup, and one atomic owner guard across all
  generated streamed methods;
- the shared Kotlin `APDUTransport` implements `invalidateSession()` by
  synchronously closing its logical channel or
  otherwise forcing a fresh select.

Do not place another stream session manager around generated clients or inside
individual handlers. That creates competing owners and defeats cross-method
serialization and reset cleanup.

The concrete applet only wires lifecycle into the generated adapter. Construct
one adapter for the applet instance, call `processIfStream(apdu)` before the
ordinary generated dispatcher, and forward `deselect()` to the adapter. Do not
copy its runtime or state machine into the applet:

```java
private final ServiceLogic logic = new ServiceLogic();
private final ServiceStreamAPDUAdapter streams =
    new ServiceStreamAPDUAdapter(logic);

public void process(APDU apdu) {
    if (selectingApplet()) return;
    if (streams.processIfStream(apdu)) return;
    logic.processOrdinary(apdu);
}

public void deselect() {
    streams.deselect();
}
```

```toml
[applet]
name = "Counter"
version = "1.0.0"
aid = "F000000101"
cla = 0xB0

[methods.increment]
ins = 0x01
[methods.increment.request]
fields = [{ name = "amount", type = "u8" }]
[methods.increment.response]
fields = [{ name = "value", type = "u16" }]

[status_words]
SW_OVERFLOW = { code = 0x6986, description = "Counter would exceed limit" }
```

## DI Transport Pattern

### Swift Client (host wiring)

```swift
import JavaCardRPCClient   // concrete transport
import CounterClient      // generated (owns CounterTransport protocol)

extension TCPTransport: CounterTransport {
    func transmit(cla: UInt8, ins: UInt8, p1: UInt8, p2: UInt8,
                  data: Data?) async throws -> (sw: UInt16, data: Data) {
        let cmd = APDUCommand(cla: cla, ins: ins, p1: p1, p2: p2, data: data)
        let resp = try await transmit(cmd)
        return (sw: resp.sw, data: resp.data)
    }
}

let client = CounterClient(transport: TCPTransport(host: "127.0.0.1", port: 9025))
```

### Kotlin Client (host wiring)

```kotlin
import counter.CounterClient
import io.jcrpc.client.TCPTransport

val client = CounterClient(transport = TCPTransport())
```

### Java Server (host wiring)

```java
// CounterApplet extends generated CounterSkeleton
// AppletBase (from javacard-rpc-server-javacard) handles APDU dispatch
// Adapter bridges AppletBase → CounterTransport interface
```

## Runtime Packages

| Package | GitHub | Purpose |
|---------|--------|---------|
| `javacard-rpc-client-swift` | relux-works/javacard-rpc-client-swift | Swift transport: APDUCommand, APDUResponse, TCPTransport, DataPacker |
| `javacard-rpc-client-kotlin` | relux-works/javacard-rpc-client-kotlin | Kotlin/JVM shared transport and lifecycle: APDUCommand, APDUResponse, APDUTransport, TCPTransport, DataPacker |
| `javacard-rpc-server-javacard` | relux-works/javacard-rpc-server-javacard | Java Card base: AppletBase with APDU dispatch + type helpers |

## Bridge Consumers (CardProvider SPI)

The bridge (version `0.3.1`) only needs a `CardSimulator`; who builds it is pluggable:

```java
public interface io.jcrpc.bridge.card.CardProvider { CardSimulator create(); }
```

- `--card-provider <fqcn>`: public no-arg constructor, on the bridge classpath.
  Without the flag a single ServiceLoader registration is used, else the
  default provider (plain `CardSimulator` + `--config` applets, pre-SPI behaviour).
- The provider owns runtime choice, GP secure channel, install and
  personalization; the bridge never learns a key.
- `--card-scope connection` (default): fresh `create()` per TCP connection.
- `--card-scope shared`: one card at server start, every connection under one
  lock; the RESET frame resets that shared card for all connections (selection
  cleared, persistent state kept); disconnect is not a reset.
- Missing class, non-provider class, no no-arg ctor, throwing ctor/`create()`,
  or a bad scope: typed startup refusal, exit 2, nothing bound.
- Wire protocol and Kotlin/Swift clients are untouched.
- Verify with `make test-bridge`. Full table: README "Bridge consumers".

## CLI Reference

```
jcrpc-gen [flags] <input.toml>

Flags:
  --out-dir string    Output directory (default ".")
  --java string       Generate Java skeleton with given package name
  --swift string      Generate Swift client with given module name
  --kotlin string     Generate Kotlin client with given package name
  --all               Generate Java, Swift, and Kotlin outputs (uses applet name for defaults)
  --simulator-dependency string
                      Maven coordinate the generated stream server build compiles
                      against (default "com.klinec:jcardsim:3.0.5.9")
  --validate-only     Parse + validate only
  --verbose           Print progress to stderr
```

Simulator coordinate override (consumer pinned to a jCardSim fork, e.g. bsimId
on `works.relux:jcardsim:3.0.5.9-relux.1`): never edit generated output or
`bridge/build.gradle`; pass it in instead. Both declare `mavenCentral()` then
`mavenLocal()`, so a fork from `publishToMavenLocal` resolves as-is. Default
output is byte-identical apart from that repositories line.

```bash
codegen/jcrpc-gen --all --out-dir ./gen --simulator-dependency works.relux:jcardsim:3.0.5.9-relux.1 applet.toml
cd bridge && ./gradlew build -PjcardsimDependency=works.relux:jcardsim:3.0.5.9-relux.1   # or gradle.properties
```

Anything that is not a `group:artifact:version` triple is refused (jcrpc-gen
exit 2, nothing written; Gradle `invalid jcardsimDependency`).

Recommended generation commands:

```bash
# Validate only
codegen/jcrpc-gen --validate-only examples/counter/counter.toml

# Generate everything
codegen/jcrpc-gen --all --out-dir examples/counter/generated examples/counter/counter.toml

# Generate individual targets
codegen/jcrpc-gen --swift CounterClient --out-dir examples/counter/generated examples/counter/counter.toml
codegen/jcrpc-gen --kotlin counter --out-dir examples/counter/generated examples/counter/counter.toml
codegen/jcrpc-gen --java counter --out-dir examples/counter/generated examples/counter/counter.toml
```

## Canonical Agent Workflow

- Edit `examples/counter/counter.toml` or your target IDL.
- Run `make generate` to refresh generated Swift, Kotlin, and Java outputs.
- If only codegen changed, run `make test-codegen`.
- If host wiring changed, run `make e2e` to verify one bridge with both Swift and Kotlin clients.
- If applet behavior changed, run `make test-applet` and then `make e2e`.
- Do not hand-edit files under `examples/counter/generated/`; regenerate them.

## References

- [IDL Specification](.spec/idl.md) — full TOML IDL format spec
- [Counter Example](examples/counter/) — complete E2E example with one bridge, one applet, and Swift + Kotlin host clients
