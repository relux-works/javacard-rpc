# javacard-rpc

RPC framework for Java Card applets. Define your applet's APDU interface in TOML, generate type-safe Swift and Kotlin clients plus Java Card skeletons.

## What it does

```
counter.toml  ──►  jcrpc-gen  ──►  CounterClient.swift      (Swift host-side)
                               ──►  CounterClient.kt         (Kotlin host-side)
                               ──►  CounterSkeleton.java     (card-side)
                               ──►  CounterTransport.java    (transport interface)
                               ──►  Package.swift / build.gradle.kts
```

You write your applet logic by extending the generated skeleton. The framework handles APDU encoding/decoding, field packing, and error mapping.

## Quick start

```bash
# Install CLI
cd codegen && go build -o jcrpc-gen ./cmd/jcrpc-gen
cp jcrpc-gen ~/.local/bin/    # or: make build-codegen

# Generate from IDL
jcrpc-gen --all --out-dir ./gen counter.toml

# Or generate per language
jcrpc-gen --swift CounterClient --kotlin counter --java io.example.counter counter.toml
```

## IDL format

Applet interfaces are defined in TOML:

```toml
[applet]
name = "Counter"
aid = "F000000101"
version = "1.0.0"
cla = "0x80"

[methods.increment]
ins = "0x01"
request = [{ name = "amount", type = "u8" }]
response = [{ name = "value", type = "u16" }]

[methods.get]
ins = "0x03"
response = [{ name = "value", type = "u16" }]

[status_words]
UNDERFLOW = { code = "0x6985", description = "Counter would go negative" }
```

Supported types: `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`.

See [IDL specification](references/idl-spec.md) for the full format.

## Architecture

Generated code uses dependency injection — no framework imports in your applet logic:

```
┌─────────────────────────┐     ┌──────────────────────────┐
│ CounterClient (host)    │     │  CounterSkeleton (Java)  │
│  encode args → APDU     │────▶│  APDU → dispatch → handler│
│  APDU response → types  │◀────│  handler result → APDU    │
└─────────────────────────┘     └──────────────────────────┘
        │                                   ▲
        ▼                                   │
  CounterTransport               YourApplet extends Skeleton
  (protocol/interface)           override onIncrement(), etc.
```

**Java side** — extend the skeleton, implement your methods:

```java
public class MyCounterApplet extends CounterSkeleton {
    @Override
    protected byte[] onIncrement(short amount) {
        counter += amount;
        return packU16(counter);
    }
}
```

**Host side** — use the generated client:

```swift
let counter = CounterClient(transport: transport)
let value = try await counter.increment(amount: 5)
```

## Project structure

```
javacard-rpc/
├── codegen/              # Go codegen (TOML parser + generators)
│   └── cmd/jcrpc-gen/    # CLI entry point
├── bridge/               # jCardSim TCP bridge for testing
├── examples/counter/     # Full working example
│   ├── counter.toml      # IDL definition
│   ├── applet/           # Java applet + jCardSim tests
│   ├── cli/              # Swift E2E test runner
│   └── kotlin-cli/       # Kotlin/JVM E2E test runner
├── references/           # IDL specification
└── scripts/              # Setup/teardown
```

See [examples/counter/README.md](examples/counter/README.md) for the step-by-step walkthrough from `counter.toml` to generated client/server artifacts and the final E2E run.

## Runtime packages

Generated code depends on thin runtime libraries:

| Package | Description |
|---------|-------------|
| [javacard-rpc-client-swift](https://github.com/relux-works/javacard-rpc-client-swift) | Swift: `APDUCommand`, `APDUResponse`, `TCPTransport`, `DataPacker` |
| [javacard-rpc-client-kotlin](https://github.com/relux-works/javacard-rpc-client-kotlin) | Kotlin/JVM: `APDUCommand`, `APDUResponse`, `TCPTransport`, `DataPacker` |
| [javacard-rpc-server-javacard](https://github.com/relux-works/javacard-rpc-server-javacard) | Java Card: `AppletBase` with APDU dispatch + type helpers |

## Testing

```bash
# Codegen unit tests (48 tests)
make test-codegen

# Full E2E (build everything + run Swift + Kotlin integration harnesses)
make e2e
```

## Build targets

| Target | Command |
|--------|---------|
| Build codegen CLI | `make build-codegen` |
| Generate from IDL | `make generate` |
| Build jCardSim bridge | `make build-bridge` |
| Build example applet | `make build-applet` |
| Build Swift E2E CLI | `make build-cli` |
| Build Kotlin E2E CLI | `make build-kotlin-cli` |
| Run codegen tests | `make test-codegen` |
| Full E2E pipeline | `make e2e` |

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
