# javacard-rpc 0.4.6

The facade composes released Java Card, Kotlin and Swift generator backends
through pluginapi v0.1.0. TOML parsing, validation, target selection, filesystem
writes and compatible Go entry points remain in javacard-rpc; each target owns
its renderer, build templates and native runtime.

| Target | Backend and runtime tag | Peeled commit | Native identity |
| --- | --- | --- | --- |
| Java Card | v0.3.0 | `0a41fc2cd30f0b1e0871a0e2ff74583a8c04128c` | `io.jcrpc:javacard-rpc-server-javacard:0.3.0` |
| Kotlin | v0.3.0 | `df7a85ea9715ae00827971ddb6608704b2f6ff29` | `io.jcrpc:javacard-rpc-client-kotlin:0.3.0` |
| Swift | v0.2.2 | `3db025b025238ab21b2b458bd0cf0cad285223f4` | `JavaCardRPCClient`, package `javacard-rpc-client-swift` |

The API tag is `pluginapi/v0.1.0`, peeled commit
`ef6e04bfae8dab0f8e1ac40c9bb10713dbf09250`. Exact signed target tag objects,
backend imports and runtime identities are recorded in the
[runtime manifest](compatibility/runtime-manifest.json).

Ordered CLI results and generated package bytes are identical to signed v0.4.5
across repository examples, generator fixtures and the immutable B6 schema at
`2d23abdafa1e0f68c6003ab56274b2ac38378ef9` (SHA-256
`1be1ed52ac9a85a62d5c5e371a9e22d38f834282681528476ca071e7bfc2cb66`).
Public native names, default output, stream protocol and workspace policy are
preserved. Counter supports all three targets; B6 supports Java Card and Kotlin.
Swift stream selection still refuses before writing output.

Use the facade's manifest to prepare exact local runtime checkouts. Gradle
consumers substitute the native roots with `includeBuild`; Swift consumers use
local package paths and a host adapter implementing the generated transport
protocol. Java Card builds use the pinned server checkout's
`src/main/java/io/jcrpc/server` sources. Generated manifests retain their v0.4.5
bytes, including Kotlin's historical runtime version; the verified local
substitution selects the released runtime.

[Compatibility and offline consumers](compatibility/README.md) provides the
`jcrpc-compat` check, bootstrap, prepare, JVM, Swift and toolchain entry points.
Initial bootstrap requires network access and populates build dependencies;
ordinary prepared builds use local checkouts and the OS network-denial wrapper.
Local validation covers JVM consumers, generated simulator/CAP behavior and
Swift/iOS packages. Simulator and generated-array evidence does not establish
physical-card RAM, NVM endurance or sharing/firewall behavior.
