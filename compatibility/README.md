# Exact runtime compatibility

`runtime-manifest.json` records the canonical repository, signed annotated tag,
peeled commit, backend module/version/import and unchanged native identity for
Java Card v0.5.0, Kotlin v0.3.0 and Swift v0.2.2. `releases/*.json` transcribe the
reviewed release receipts supplied to this integration. `allowed_signers` contains
only the two public signing keys used by those receipts. Bootstrap verifies the
actual tag object, peeled commit, checkout HEAD and signature, then refuses tracked
source changes before every native build. Go's resolved graph must use those
versions and pluginapi v0.1.1 without replacements.

The immutable B6 fixture is `inputs/bsim-auth-2d23abd.toml`, copied from bsimId main
commit `2d23abdafa1e0f68c6003ab56274b2ac38378ef9`, SHA-256
`1be1ed52ac9a85a62d5c5e371a9e22d38f834282681528476ca071e7bfc2cb66`.
It contains schema only; no live consumer changes or device identifiers are used.
Java/Kotlin generation applies to it; Swift stream generation still exits 2 before
writing output. The counter example supports all three targets.

## Initial bootstrap (network allowed)

Use Go 1.25.5, Gradle 9.2.1, JDK 17 for JVM execution, JDK 11 for the older Java
Card kit, and Swift 6.2+ with an iOS SDK on macOS. CI records the installed Swift
and Xcode versions rather than claiming an exact immutable hosted runner image.

```sh
cd codegen
go mod download
go mod verify
go build -o ../.temp/jcrpc-compat ./cmd/jcrpc-compat
cd ..
.temp/jcrpc-compat --repo . --mode bootstrap --root .temp/consumer
.temp/jcrpc-compat --repo . --mode prepare --root .temp/consumer
.temp/jcrpc-compat --repo . --mode jvm --root .temp/consumer
.temp/jcrpc-compat --repo . --mode swift --root .temp/consumer
```

Bootstrap needs Git/network access and downloads exact runtime checkouts under
`.temp/consumer/runtimes/`, preserving their native root layouts and Swift package
identity directories. The first native builds populate Gradle plugin/dependency
caches. Swift packages use local paths and have no remote package dependencies.
Preparing a consumer builds the facade executable and regenerates the repository
examples plus pinned bsim schema. Facade v0.6.0 appends independent caller workspace to ordinary and stream Java
callbacks/entry points. Kotlin/Swift wire and package identities remain unchanged.
The JVM fixture implements packed and borrowed-span callbacks, checks fixed
capacity rejection before callback entry, and preserves the existing encoded wire.

See [v0.6.0 migration and bounds](../CALLER-WORKSPACE.md). Ordinary callers
fully receive supported input, capture headers before overlapping output, pass
validated spans to `dispatchTo`, and send only the exact successful produced
span. APDU/input/output references are command-local. Consume borrowed bytes
before overlap. Whole-only widths 127 at offset 6 and 13 at offset 7 fit 133
bytes; 190/177 need larger caller/transport capacity. Trusted handlers have no
sandbox/rollback guarantee. Optional-int and physical capacity remain deployment
inputs; target simulator/CAP evidence is not migrated-consumer qualification.

Before offline Go properties, both CI jobs also cache the public historical
modules used by `TestManifestAPIRefusals` and the `manifest-api-old` narrowing
mutant. From the repository root, after the production `go mod download`, run:

```sh
go -C codegen mod download github.com/relux-works/javacard-rpc/pluginapi@v0.1.0 github.com/relux-works/javacard-rpc-server-javacard@v0.3.0
```

These are test-fixture cache inputs, not production requirements or replacements.
They let the historical tuple reach the exact obsolete-API refusal offline;
lowering only the direct API requirement still selects v0.1.1 through full MVS.

## Ordinary prepared builds (network denied)

```sh
go -C codegen test ./cmd/jcrpc-compat -run '^TestOfflineGuardRejectsDependencyAccess$' -count=1 -v
.temp/jcrpc-compat --mode offline -- .temp/jcrpc-compat --repo . --mode jvm --offline --root .temp/consumer
.temp/jcrpc-compat --mode offline -- .temp/jcrpc-compat --repo . --mode swift --offline --root .temp/consumer
```

The OS guard allows local build-tool IPC and denies remote dependency access.
On macOS it uses `sandbox-exec`; on Linux it uses a dedicated network namespace
(`sudo -n unshare --net`, with loopback enabled through `ip` and the build
returned to the caller's UID/GID through `setpriv`). Unsupported hosts
refuse. Linux local IPC is between processes launched inside that same namespace;
it does not include a listener in the host namespace. The positive control starts
its listener and curl child inside the guard on Linux. macOS retains a real
host-loopback listener control, and a separate child-listener control exercises
the helper on both platforms. The control actually attempts an HTTPS Maven
dependency fetch and requires
curl's connection refusal; a missing curl or broken sandbox is a failed control.
The valid prepared consumer must also build/test successfully under that guard.
Gradle `--offline` and Swift automatic-resolution disabling supplement the OS
guard. This is a prepared-build guarantee, not a cold-machine dependency claim.

The generated consumer root contains the normal Gradle `includeBuild` substitutions:

```groovy
includeBuild('/absolute/pinned/javacard-rpc-client-kotlin') {
    dependencySubstitution {
        substitute(module('io.jcrpc:javacard-rpc-client-kotlin')).using(project(':'))
    }
}
includeBuild('/absolute/pinned/javacard-rpc-server-javacard') {
    dependencySubstitution {
        substitute(module('io.jcrpc:javacard-rpc-server-javacard')).using(project(':'))
    }
}
includeBuild('/absolute/generated/counter-client-kotlin')
includeBuild('/absolute/generated/counter-server-javacard')
```

Keep the runtime checkouts at native roots. The historical generated Kotlin
manifest still names runtime `0.2.0` for byte parity; the root substitution selects
the manifest's verified Kotlin v0.3.0 checkout for every included project. The
Java Card substitution selects the signed v0.5.0 runtime, including when a
generated ecosystem coordinate retains its historical version for byte parity. Consumers pin
only the facade and use its runtime manifest; they do not independently guess
compatible runtime versions or edit generated Gradle files.

Swift's host Package.swift uses:

```swift
.package(path: "/absolute/generated/counter-client-swift"),
.package(path: "/absolute/pinned/javacard-rpc-client-swift")
// target products:
.product(name: "CounterClient", package: "counter-client-swift"),
.product(name: "JavaCardRPCClient", package: "javacard-rpc-client-swift")
```

The generated `CounterTransport` is a standalone DI protocol. The host supplies
an adapter to the unchanged runtime transport; the prepared Swift consumer tests
compile that adapter and exercise its data/status result.

## Public native CI

`.github/workflows/compatibility.yml` runs JVM/Java Card and Swift/iOS lanes.
`.temp/jcrpc-compat --mode toolchain --root .temp/toolchain` fetches public jars/kit/Ant and
checks full SHA-256 digests before use. Simulator identity is
`works.relux:jcardsim:3.0.5.9-relux.1`, source commit
`9d336521d4e7febc609c629a64f96b39d8c4568c`. Ant task v26.02.22 comes from
commit `3605dc355dd3f3fe77193cd415348183d5d47b4d`; Java Card 3.0.5u4 kit bytes
come from mirror commit `700ec80afdda210a0e62fb6a151a9cddc1acd244`; Apache Ant
is 1.10.15. Digests and exact download URLs are in the Go bootstrap helper.

The Relux simulator accepts `MessageDigest.getInstance(..., true)` as a no-op
sharing flag; stock 3.0.5.9 legitimately refuses that unsupported mode. Passing
fork simulation does not certify physical-card sharing/firewall semantics. CAP
conversion uses JDK 11, explicit `jckit` and the selected kit's default target,
with converter verification enabled. No physical-card install/reset or live bsim
regeneration is part of this lane. Optional historical allocation-baseline tests
still require their own explicit baseline and are not described as passing when
skipped. Public fetch provenance is not evidence of executed Linux CI.

## Tools and outputs

| Entry point | Purpose | Output |
| --- | --- | --- |
| `cd codegen && go run ./cmd/jcrpc-compat --repo .. --mode check` | Manifest/receipt/resolved-version integrity | Refusal or acceptance on stdout/stderr |
| `jcrpc-compat --mode bootstrap --root .temp/consumer` | Exact signed local runtime checkouts | `.temp/consumer/runtimes/` |
| `jcrpc-compat --mode prepare --root .temp/consumer` | Example and immutable bsim generation, consumer roots | `.temp/consumer/generated/`, `jvm/`, `swift/` |
| `jcrpc-compat --mode jvm --root .temp/consumer` | Native Gradle builds and tests with includeBuild | Native `build/` directories |
| `jcrpc-compat --mode swift --root .temp/consumer` | Runtime and path-consumer Swift Testing | Native `.build/` directories |
| `go -C codegen test ./cmd/jcrpc-compat -run '^TestOfflineGuardRejectsDependencyAccess$' -count=1 -v` | Actual dependency network-access rejection | Named Go test output |
| `go -C codegen test ./cmd/jcrpc-compat -run '^TestOfflineGuardAllowsChildLocalIPC$' -count=1 -v` | Real listener and curl child inside the OS guard | Named Go test output |
| `.temp/jcrpc-compat --mode offline -- COMMAND...` | OS network enforcement for prepared commands | Command output, real exit status |
| `.temp/jcrpc-compat --mode toolchain --root .temp/toolchain` | Public digest-checked native inputs | `.temp/toolchain/` |

Validation logs belong under task-scoped `.temp/` directories. No helper installs
shared tools or modifies the source runtime repositories.

Release signature trust is deliberately scoped to `alexis@relux.works` and the
owner-approved public ED25519 fingerprint
`SHA256:1pl4mNUEP62BtX/d2Z28O0B77VlDUqzNTpsPitQzNiA` (Java Card/Swift) and ECDSA
fingerprint `SHA256:60fPOOw38n4bW1moyoPfQ/TZIRwPFLgYf7BfR49npaU` (Kotlin).
The keys in `allowed_signers` are the public keys for those explicitly approved
identities, not a workstation trust-store export. Signature checks establish
that a tag was signed by one of these keys; owner approval supplies the trust
anchor. They do not infer identity from a self-signed tag or certify unrelated
signers. Exact tag-object and commit pins also remain mandatory.

Set `JCRPC_COMPAT_ROOT=/absolute/prepared/consumer` when running the optional
`TestPinnedCheckoutSourceRefusals` lane. It clones these actual signed checkouts
locally and verifies that ignored build outputs remain accepted while tracked
changes, missing tags and new non-ignored production source files refuse before
native build launch. No live runtime source or tag is changed.

On macOS, the offline wrapper selects Java's IPv4 socket stack for loopback IPC.
The sandbox's localhost rule does not admit the mapped IPv6 endpoint used by
Gradle's daemon connection on this host. Existing `JAVA_TOOL_OPTIONS` are retained;
this option changes only the prepared offline build process. The remote Maven
control is rerun with the same profile and still refuses.

The two scoped public keys were reproduced from SSHSIG key blobs in the actual
public release tag objects and matched to the fingerprints in the owner's
`intended-target-release-signers.md` precondition. The shipped trust asset therefore
contains only those approved keys; no workstation trust-store copy is required.
