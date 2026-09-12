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

For a schema containing `stream`, Java generation additionally emits one
applet-level `*BoundedStreamRuntime`, its `*StreamEndpoint` contract, and a
`*StreamAPDUAdapter`. Kotlin generation puts upload, recovery, download, close,
abort, and transport invalidation directly in the typed generated client. The
application supplies only business handlers and an APDU transport; it does not
implement the stream session state machine.

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

For the repo's canonical example, use:

```bash
make generate
make e2e
```

That regenerates:

- `examples/counter/generated/counter-client-swift`
- `examples/counter/generated/counter-client-kotlin`
- `examples/counter/generated/counter-server-javacard`

And then runs one bridge with two host clients against the same applet.

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

Supported types: `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`, and bounded multi-APDU `stream` values.

See [IDL specification](.spec/idl.md) for the full format and six-command
stream lifecycle. Ordinary values use generated fixed-order byte packing, not
Protocol Buffers. Stream payloads are opaque `ByteArray` values whose internal
format belongs to the calling application. Stream generation currently targets
the Java Card server and Kotlin client; generate those explicitly because the
Swift stream client is not implemented yet.

All streamed methods in one selected applet share exactly one generated session
manager and one transient workspace. This serializes large operations, prevents
cross-method state corruption, and keeps reset/deselect cleanup inside generated
code. The generated Java adapter consumes fragmented incoming APDU data before
dispatch. On the host, any failed or cancelled stream operation performs a
best-effort abort and then calls `invalidateSession()` on the shared transport.
An exception thrown during either cleanup step is ignored so it cannot replace
the authoritative protocol failure or coroutine cancellation.
The generated client also rejects a concurrent streamed call locally with
`StreamBusy`, without sending an APDU that could abort the active call.

The applet itself still owns normal Java Card lifecycle wiring. Instantiate the
generated stream adapter once, let it inspect the APDU before ordinary dispatch,
and forward deselection to it. The session manager remains generated:

```java
public final class MyApplet extends Applet {
    private final MyServiceLogic logic = new MyServiceLogic();
    private final MyServiceStreamAPDUAdapter streams =
        new MyServiceStreamAPDUAdapter(logic);

    public void process(APDU apdu) {
        if (selectingApplet()) return;
        if (streams.processIfStream(apdu)) return;
        logic.processOrdinary(apdu); // Existing non-stream dispatch.
    }

    public void deselect() {
        streams.deselect();
    }
}
```

This wrapper does not implement stream state or recovery. It only connects the
generated code to the applet lifecycle.

## Architecture

Generated code uses dependency injection; no framework imports in your applet logic:

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

**Java side**: extend the skeleton, implement your methods:

```java
public class MyCounterApplet extends CounterSkeleton {
    @Override
    protected byte[] onIncrement(short amount) {
        counter += amount;
        return packU16(counter);
    }
}
```

**Host side**: use the generated client:

```swift
let counter = CounterClient(transport: transport)
let value = try await counter.increment(amount: 5)
```

Kotlin/JVM generated clients accept the shared `APDUTransport` from the
standalone `javacard-rpc-client-kotlin` runtime directly. The generated module
depends on runtime version `0.2.0`; no applet-specific transport bridge is
generated or required.

For a stream-capable Kotlin transport, implement `invalidateSession()`. It must
synchronously make the selected applet
session unusable, normally by closing the logical channel; the next operation
then starts from a fresh select. The generated client invokes it after every
non-terminal failure, including coroutine cancellation.

## Bridge consumers: CardProvider SPI and card scope

The bridge (`bridge/`, currently version `0.3.0`) exposes one consumer SPI so a host project
can bring its own card without the bridge learning anything about it:

```java
package io.jcrpc.bridge.card;

public interface CardProvider {
    CardSimulator create();
}
```

`create()` is the whole contract. Runtime choice (a custom `SimulatorRuntime`),
GlobalPlatform secure-channel setup, applet install and issuer personalization
all happen inside the provider; the bridge never sees an issuer key or authority.
The provider class must be on the bridge classpath and have a public no-arg
constructor.

```bash
java --add-modules java.smartcardio -cp bridge.jar:jcardsim.jar:my-provider.jar \
    io.jcrpc.bridge.Main \
    --card-provider com.example.MyKeyVaultProvider \
    --card-scope shared \
    --port 9025
```

Provider resolution order:

| Situation | Provider used |
| --- | --- |
| `--card-provider <fqcn>` given | that class (explicit wins over ServiceLoader) |
| no flag, exactly one `META-INF/services/io.jcrpc.bridge.card.CardProvider` registration | the registered class |
| no flag, no registration | `DefaultCardProvider`: `new CardSimulator()` + install the `--config` applets in order with their install params (pre-SPI behaviour, byte-identical) |
| no flag, several registrations | startup refusal `PROVIDER_AMBIGUOUS` |

Card scope (`--card-scope`, default `connection`):

| Scope | Cards | Lock | RESET frame |
| --- | --- | --- | --- |
| `connection` | `create()` per TCP connection; state never crosses connections | none needed | resets only that connection's card |
| `shared` | one `create()` at server start; every connection talks to the same card | single lock around every APDU/RESET/ATR, so concurrent connections are serialized, never interleaved | resets the shared card for everyone: applet selection is cleared, persistent applet state survives |

Under `shared` scope selection is card state: connection B inherits whatever
connection A selected, and a client disconnect is *not* an implicit reset.
The only way to reset the shared card is the explicit RESET frame (`0x02`),
and it is visible to all connections. Wire protocol and clients are unchanged;
the Kotlin/Swift transports need no update to use either scope.

Startup refusals (exit code 2, `[bridge] startup refused: <REASON>: ...` on
stderr, no port bound, never a hang):

| Reason | Cause |
| --- | --- |
| `PROVIDER_NOT_FOUND` | `--card-provider` class not on the classpath |
| `PROVIDER_NOT_A_CARD_PROVIDER` | class does not implement `CardProvider` |
| `PROVIDER_NO_NOARG_CTOR` | no public no-arg constructor |
| `PROVIDER_INSTANTIATION_FAILED` | constructor threw / abstract class / ServiceLoader error |
| `PROVIDER_AMBIGUOUS` | more than one ServiceLoader registration and no `--card-provider` |
| `CARD_CREATE_FAILED` | `create()` threw or returned null (validated once at start in both scopes) |
| `INVALID_SCOPE` | `--card-scope` not `connection` or `shared` |

Bridge tests: `cd bridge && ./gradlew test` (JUnit 5; covers default-provider
byte-identity, both scopes over real TCP, and every refusal above).

### Pinning the simulator artefact

Both the generated Java Card server build and the bridge JVM compile against
`com.klinec:jcardsim:3.0.5.9` by default. A consumer pinned to another simulator
artefact — for example `bsimId`, which uses the `relux-works/jcardsim` fork
`works.relux:jcardsim:3.0.5.9-relux.1` because upstream refuses
`externalAccess=true` engines — never edits generated output or the bridge
build; it passes the coordinate in:

| Where | How | Default |
| --- | --- | --- |
| generated `<applet>-server-javacard/build.gradle` (`compileOnly`) | `jcrpc-gen --simulator-dependency works.relux:jcardsim:3.0.5.9-relux.1 ...` | `com.klinec:jcardsim:3.0.5.9` |
| `bridge/build.gradle` (`implementation`) | `cd bridge && ./gradlew build -PjcardsimDependency=works.relux:jcardsim:3.0.5.9-relux.1`, or `jcardsimDependency=...` in `gradle.properties` / `~/.gradle/gradle.properties` | `com.klinec:jcardsim:3.0.5.9` |

The value must be a plain `group:artifact:version` triple; anything else is
refused before generation (`jcrpc-gen` exit code 2, nothing written) or at
Gradle configuration time (`invalid jcardsimDependency`). The generated
`build.gradle` is the record of which coordinate a package was generated
against — there is no separate manifest. The bridge resolves the coordinate
from `mavenCentral()` and then `mavenLocal()`, so a fork published with
`./gradlew publishToMavenLocal` works without further repository setup. With
the default the generated output is byte-identical to earlier releases.

This is how `bsimId` builds:

```bash
jcrpc-gen --all --out-dir ./gen --simulator-dependency works.relux:jcardsim:3.0.5.9-relux.1 keyvault.toml
cd bridge && ./gradlew build -PjcardsimDependency=works.relux:jcardsim:3.0.5.9-relux.1
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
├── .spec/                # IDL and protocol specifications
├── UNRESOLVED_QUESTIONS.md # Deferred cross-platform decisions
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
# Codegen plus generated Java and Kotlin stream harnesses
make test-codegen

# Bridge: default provider byte-identity, card scopes over TCP, startup refusals
make test-bridge

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
| Run bridge tests (CardProvider SPI, card scopes, refusals) | `make test-bridge` |
| Run the mandatory generated Kotlin transport/lifecycle harness | `make test-kotlin-contract` |
| Convert generated stream applet to CAP | `JCRPC_ANT_JAVACARD_JAR=... JCRPC_JCKIT_DIR=... make test-cap` |
| Run release validation including CAP conversion | `JCRPC_ANT_JAVACARD_JAR=... JCRPC_JCKIT_DIR=... make release-check` |
| Full E2E pipeline | `make e2e` |

## Tooling

| Tool | Purpose | Command | Output |
| --- | --- | --- | --- |
| Go | Build codegen and run parser, generator, JVM harness, and CLI tests | `cd codegen && go test ./...` | Go test cache; task-local smoke files use `.temp/` |
| Gradle | Compile generated Java/Kotlin packages and run the mandatory Kotlin/JVM transport/lifecycle harness | `make test-kotlin-contract`; `gradle -p <generated-package> build` | Go-managed temporary harness or package-local `build/` |
| `javac` / `java` | Compile and execute generated Java runtime and fragmented-APDU harnesses | Run through `go test ./...` | Go-managed temporary directories |
| Ant + ant-javacard | Convert the generated Java stream applet to a verified CAP | `JCRPC_ANT_JAVACARD_JAR=... JCRPC_JCKIT_DIR=... make test-cap` | Go-managed temporary CAP |
| Make | Stable project entry points and release gate | `make generate`, `make test-codegen`, `make test-bridge`, `make test-applet`, `make test-cap`, `make release-check`, `make e2e` | Generated examples under `examples/counter/generated/`; build products remain local |

Go tests use their own temporary directories. Local task runs and generated
smoke packages belong under `.temp/`. The generated Kotlin stream harness runs
through `gradle test`; the Java stream runtime harness uses `javac` and `java`.

<!-- relux-ecosystem:start -->

## About Relux Works

This project is part of the open-source ecosystem of
[Relux Works](https://relux.works), an AI-native software development studio.
We build fixed-price MVPs, rescue vibe-coded apps, run local AI inference, and
train teams to work with coding agents. Much of the infrastructure behind that
work is open source.

- Full catalog: [relux.works/en/open-source](https://relux.works/en/open-source/)
- Agentic enablement: [agent harnesses & team training](https://relux.works/en/agentic-enablement/)
- Hire us the agent-native way: point your assistant at `https://api.relux.works/mcp`
- Contact: ivan@relux.works

<!-- relux-ecosystem:end -->

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
