---
name: javacard-rpc
description: RPC framework for Java Card smart card applets. TOML IDL → codegen → typed Swift clients and Java skeletons with DI transport abstraction.
triggers:
  - javacard-rpc
  - applet rpc
  - java card codegen
  - apdu codegen
  - smart card rpc
---

# javacard-rpc

RPC framework for Java Card smart card applets. Define APDU contracts in TOML, generate typed code.

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
    └─→ counter-server-javacard/  (generated, zero-dep)
          CounterTransport        (interface — DI)
          CounterSkeleton         (abstract class)
```

Generated packages have **zero external dependencies**. Transport is injected via DI:

- **Client (Swift):** host conforms concrete transport to generated `<Name>Transport` protocol via extension
- **Server (Java):** host wraps concrete transport in adapter implementing generated `<Name>Transport` interface

## Quick Start

```bash
# Build codegen
cd codegen && go build ./cmd/jcrpc-gen

# Generate from IDL
jcrpc-gen --all --out-dir . counter.toml

# Or use Makefile
make generate
```

## IDL Format

See [.spec/idl.md](.spec/idl.md) for the full specification.

TOML IDL defines: applet metadata, methods (INS, request/response fields), status words.

**Field types:** `u8`, `u16`, `u32`, `bool`, `bytes`, `bytes[N]`

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
import AppletRPCClient   // concrete transport
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

### Java Server (host wiring)

```java
// CounterApplet extends generated CounterSkeleton
// AppletBase (from appletrpc-server-javacard) handles APDU dispatch
// Adapter bridges AppletBase → CounterTransport interface
```

## Runtime Packages

| Package | GitHub | Purpose |
|---------|--------|---------|
| `appletrpc-client-swift` | relux-works/appletrpc-client-swift | Swift transport: APDUCommand, APDUResponse, TCPTransport, DataPacker |
| `appletrpc-server-javacard` | relux-works/appletrpc-server-javacard | Java Card base: AppletBase with APDU dispatch + type helpers |

## CLI Reference

```
jcrpc-gen [flags] <input.toml>

Flags:
  --out-dir string    Output directory (default ".")
  --java string       Generate Java skeleton with given package name
  --swift string      Generate Swift client with given module name
  --all               Generate both (uses applet name for defaults)
  --validate-only     Parse + validate only
  --verbose           Print progress to stderr
```

## References

- [IDL Specification](.spec/idl.md) — full TOML IDL format spec
- [Counter Example](examples/counter/) — complete E2E example with bridge, applet, CLI
