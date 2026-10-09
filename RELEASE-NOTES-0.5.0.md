# javacard-rpc v0.5.0 release notes

This facade composes the released Java Card ordinary writer API v0.4.0.
Regenerate Java skeletons and migrate their subclasses and APDU send sites.
There is no array-return compatibility shim. Scalar/void callbacks, IDL, INS,
field order, big-endian encodings, Kotlin/Swift wire and stream lifecycle remain
unchanged. Stream handlers retain their raw-array contract.

The facade release uses the root tag `v0.5.0`. `codegen` is a nested Go module;
build the CLI from a verified root source checkout. These instructions do not
claim a `codegen/*` tag, version-suffixed `go install` support or publication of
the facade. After the signed root release is available, fetch it into a dedicated
source checkout and verify it with the approved signer before building:

```sh
git fetch origin tag v0.5.0
git verify-tag v0.5.0
git checkout --detach v0.5.0
mkdir -p .temp
cd codegen
go mod download
go build -o ../.temp/jcrpc-gen ./cmd/jcrpc-gen
```

## Actual dependency provenance

| Dependency | Signed tag object | Peeled commit |
| --- | --- | --- |
| Java Card v0.4.0 | `2d8786271c3cb3419ee524c8af7d1c8e5fac2e13` | `625e714c3d4ce9b00fa298b7b4d57208d1417bfb` |
| Kotlin v0.3.0 | `6362729d42eeb8e893a169589ab018de04a38297` | `df7a85ea9715ae00827971ddb6608704b2f6ff29` |
| Swift v0.2.2 | `e0052b54b590a32faa96baad7b0a8d3fbc2efe0d` | `3db025b025238ab21b2b458bd0cf0cad285223f4` |

Java Card's accepted aggregate tree is
`6d53b9ed4ef826ffe0ddba0dd5313800c9b126a7`. Its actual
[release](https://github.com/relux-works/javacard-rpc-server-javacard/releases/tag/v0.4.0)
and [merged PR 3](https://github.com/relux-works/javacard-rpc-server-javacard/pull/3)
identify the published source. The Go backend is
`github.com/relux-works/javacard-rpc-server-javacard v0.4.0`, import `/codegen`;
the runtime coordinate is `io.jcrpc:javacard-rpc-server-javacard:0.4.0` through
verified source bootstrap/Gradle includeBuild, with no Maven publication claim.
Plugin API remains the released `github.com/relux-works/javacard-rpc/pluginapi v0.1.1`:
signed tag `pluginapi/v0.1.1` object `1cffd89daa20a377902cc4670daa36ec8cdac9de`,
peeled commit `9532ca2a3f0fb5e5038f3f5658bad226998a1672`.
Current receipts and the resolved graph are checked through
`jcrpc-compat`; deliberate historical fixtures and downgrade controls remain.

## Ordinary source migration

```java
short produced = logic.dispatchTo(ins, p1, p2,
    requestBuffer, requestOffset, requestLength,
    output, outputOffset, outputCapacity);
// Only successful return supplies sendable bytes.
if (produced > 0) apdu.setOutgoingAndSend(outputOffset, produced);
```

Byte/packed response callbacks write the existing encoded wire into caller-owned
storage and return a produced `short`. Each byte-sequence input is a borrowed
buffer/offset/length triple. Fully receive supported incoming fragments and
capture INS/P1/P2 before overlapping writes. Keep APDU/input/output references
command-local. Consume needed borrowed input before overwriting it; one
`packBytes` call has memmove semantics, but several copies need deliberate
ordering or caller-owned workspace.

`CounterJCApplet.process` now stages the entire supported short request in its
actual APDU buffer and rejects extended, truncated, overreported or oversized
incoming spans before dispatch/send. It offers at most 255 bytes of whole reply
capacity and sends exactly the successfully produced span. Counter packed/bytes
callbacks write to that span; the mock signature consumes its borrowed input
into install-time owned scratch before overlapping writes. No command-time
request/reply arrays or retained APDU reference are introduced.

## Consumer migration handoff

bsimId, Auth and KeyVault owners should pin the verified facade root source and
use `compatibility/runtime-manifest.json` for the exact runtime checkouts above.
Build `jcrpc-gen` from that source, regenerate ordinary Java skeletons, and
migrate their subclasses and APDU send sites together. Keep Kotlin v0.3.0,
Swift v0.2.2 and pluginapi v0.1.1; runtime source bootstrap and Gradle
`includeBuild` are documented in [the compatibility flow](compatibility/README.md).

- Replace array-returning byte/packed callbacks with writer callbacks returning
  the produced `short`. Supply validated request/output spans to `dispatchTo`
  and send only its successful returned span; never send partial output after
  a refusal.
- Remove response-only transient arrays and copy staging made obsolete by the
  ordinary writer path. Write directly into the caller-owned reply window.
  Remove obsolete allocations and fields as well as their call sites; retaining
  an unused response buffer does not recover its RAM. Stream workspaces and
  necessary business/input scratch remain governed by their separate contracts.
- Convert byte-sequence inputs to borrowed buffer/offset/length triples and
  consume them before overlapping output. Keep necessary owned scratch when
  business logic needs input after overlap; retain no command buffer reference.
- Check whole reply capacity for each method. Preserve exact wire and status
  behavior, and verify insufficient capacity refuses before fixed callbacks
  and sends nothing. There is no compatibility shim or implicit response stream.
- Record the facade commit/tag object, runtime manifest hash, regenerated source
  identity and before/after response allocations in each consumer's migration
  evidence. Qualify that consumer's RAM/NVM, APDU capacity and physical behavior
  separately; the target and Counter gates do not certify those deployments.

## Capacity and qualification bounds

Invalid windows/request lengths, insufficient fixed capacity and invalid
produced counts refuse with `6700`; unknown INS refuses with `6D00` after window
validation. Fixed capacity is checked before callback entry, with the exact
width offered. Variable callbacks receive the offered capacity. Replies remain
whole-only: 127 bytes at offset 6 and 13 at offset 7 fit a 133-byte buffer;
190/177-byte replies need larger caller storage and a transport that can send
the whole response. There is no implicit ordinary streaming.

Handlers are trusted: no sandbox, rollback or wipe is promised. Packing helpers
check whole-array bounds; handlers must respect their span. Java Card Classic
optional-int support remains required for existing u32/helpers (`ints="true"`
in the accepted target CAP lane). Simulator buffer sizes do not establish
physical APDU capacity. Physical installation, timing, endurance and consumer
RAM/NVM costs are separate deployment qualifications.

Accepted unchanged target evidence covers 52 tests with zero skips, real
simulator sends and verified Classic CAP conversion at source checkpoint
`981a9f6b8005d7b6f43e6d334115328e94a92ddf`, tree
`b70f061f91d7b62865163ca954f0c1650ebc66f1`, source manifest SHA256
`1478496315841e7b1c73846144150b82214cbcd17ee67d35ea267dd65455c886`.
The actual versioned runtime JAR SHA256 is
`3ef8829c8bdf328929d123ed216f44e44fd685570606e7de792bb611fb0bffd2`.
This evidence does not qualify the migrated facade consumers or a physical card.
The facade has separate Counter APDU/simulator, pinned packed
and borrowed-span consumer, and real Classic two-package CAP gates. The CAP
gate covers the actual business/wrapper classes and initializes mock SPKI at
installation rather than calling builders from unsupported Classic `clinit`.
Its Math control uses an exact-SDK API compilation refusal; only the source-only
`@Override` annotation is erased in temporary API-check copies. The original
sources still pass Ant conversion/verifier before and after restoration.
Oracle 3.0.5u4 instead crashes internally on an unresolved Math invocation;
that converter crash is a diagnostic bound, never a passing negative control.
A separate static-builder plant requires the converter's specific unsupported
`invokestatic in clinit` refusal and positive restored conversion/verifier.
Consumer gate results are bound to recorded source/toolchain identities; hosted
facade JVM and Swift/iOS jobs remain authoritative for publication.
See the released [ownership contract](https://github.com/relux-works/javacard-rpc-server-javacard/blob/v0.4.0/ORDINARY-OUTPUT-SPANS.md)
and [compatibility entry points](compatibility/README.md).
