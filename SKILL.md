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

**Field types:** `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`

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
import counter.CounterTransport
import counter.CounterTransportResult
import io.jcrpc.client.APDUCommand
import io.jcrpc.client.APDUTransport
import io.jcrpc.client.TCPTransport

private class CounterBridgeTransport(
    private val transport: APDUTransport,
) : CounterTransport {
    override suspend fun transmit(
        cla: UByte,
        ins: UByte,
        p1: UByte,
        p2: UByte,
        data: ByteArray?,
    ): CounterTransportResult {
        val response = transport.transmit(APDUCommand(cla = cla, ins = ins, p1 = p1, p2 = p2, data = data))
        return CounterTransportResult(sw = response.sw, data = response.data)
    }
}

val client = CounterClient(transport = CounterBridgeTransport(TCPTransport()))
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
| `javacard-rpc-client-kotlin` | relux-works/javacard-rpc-client-kotlin | Kotlin/JVM transport: APDUCommand, APDUResponse, TCPTransport, DataPacker |
| `javacard-rpc-server-javacard` | relux-works/javacard-rpc-server-javacard | Java Card base: AppletBase with APDU dispatch + type helpers |

## CLI Reference

```
jcrpc-gen [flags] <input.toml>

Flags:
  --out-dir string    Output directory (default ".")
  --java string       Generate Java skeleton with given package name
  --swift string      Generate Swift client with given module name
  --kotlin string     Generate Kotlin client with given package name
  --all               Generate Java, Swift, and Kotlin outputs (uses applet name for defaults)
  --validate-only     Parse + validate only
  --verbose           Print progress to stderr
```

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
