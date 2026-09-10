# javacard-rpc bridge v0.3.0

## CardProvider SPI

The bridge now delegates card construction to a pluggable SPI instead of
always building a plain `CardSimulator` itself:

```java
package io.jcrpc.bridge.card;

public interface CardProvider {
    CardSimulator create();
}
```

`create()` is the whole contract. Runtime choice (a custom `SimulatorRuntime`),
GlobalPlatform secure-channel setup, applet install, and issuer
personalization all happen inside the provider; the bridge never sees an
issuer key or authority. The provider class must be on the bridge classpath
and have a public no-arg constructor.

Provider loading order:

| Situation | Provider used |
| --- | --- |
| `--card-provider <fqcn>` given | that class (explicit wins over ServiceLoader) |
| no flag, exactly one `META-INF/services/io.jcrpc.bridge.card.CardProvider` registration | the registered class |
| no flag, no registration | `DefaultCardProvider`: `new CardSimulator()` + install the `--config` applets in order with their install params (pre-SPI behaviour, byte-identical) |
| no flag, several registrations | startup refusal `PROVIDER_AMBIGUOUS` |

## `--card-scope`: connection | shared

New flag, default `connection`:

| Scope | Cards | Lock | RESET frame |
| --- | --- | --- | --- |
| `connection` | `create()` per TCP connection; state never crosses connections | none needed | resets only that connection's card |
| `shared` | one `create()` at server start; every connection talks to the same card | single lock around every APDU/RESET/ATR, so concurrent connections are serialized, never interleaved | resets the shared card for everyone: applet selection is cleared, persistent applet state survives |

Under `shared` scope, applet selection is card state: connection B inherits
whatever connection A selected, and a client disconnect is *not* an implicit
reset. The only way to reset the shared card is the explicit RESET frame
(`0x02`), and that reset is visible to all connections. The wire protocol and
existing Kotlin/Swift transports are unchanged and need no update to use
either scope.

## Typed startup refusals

Bridge startup now validates the provider and scope before binding a port.
Any failure below produces exit code 2, a `[bridge] startup refused:
<REASON>: ...` line on stderr, and no port bound — never a hang:

| Reason | Cause |
| --- | --- |
| `PROVIDER_NOT_FOUND` | `--card-provider` class not on the classpath |
| `PROVIDER_NOT_A_CARD_PROVIDER` | class does not implement `CardProvider` |
| `PROVIDER_NO_NOARG_CTOR` | no public no-arg constructor |
| `PROVIDER_INSTANTIATION_FAILED` | constructor threw / abstract class / ServiceLoader error |
| `PROVIDER_AMBIGUOUS` | more than one ServiceLoader registration and no `--card-provider` |
| `CARD_CREATE_FAILED` | `create()` threw or returned null (validated once at start in both scopes) |
| `INVALID_SCOPE` | `--card-scope` not `connection` or `shared` |

## Compatibility

- Wire protocol is unchanged.
- `javacard-rpc-client-kotlin` stays on `0.2.0` — no client release accompanies
  this bridge release; existing Kotlin and Swift clients work against the
  0.3.0 bridge without changes.
- With no `--card-provider` flag and no ServiceLoader registration, bridge
  behaviour is byte-identical to pre-SPI releases (`DefaultCardProvider`).

## Verification at the release commit

- `cd bridge && ./gradlew test` — exit 0 (JUnit 5: default-provider
  byte-identity, both card scopes over real TCP, every startup refusal above).
- `make release-check` — exit 0, run with `JCRPC_JCKIT_DIR` pointed at the
  local `jc320v25.1` JavaCard kit and `JCRPC_ANT_JAVACARD_JAR` pointed at the
  matching `ant-javacard.jar` (codegen unit tests, bridge tests, counter
  applet tests, generated Kotlin stream client harness, and generated Java
  stream package → CAP conversion).
