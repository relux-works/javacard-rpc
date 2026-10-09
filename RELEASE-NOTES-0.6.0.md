# javacard-rpc v0.6.0 — explicit caller workspace

This breaking Java source release composes the actual signed JavaCard target
v0.5.0, commit `96bf35555bba8a040eba880622629ff0f25b2556`, annotated tag object
`9bbf1db745834f3efefb54846f26bec6310f8c22`. Current facade metadata is `0.6.0` in
[the runtime manifest](compatibility/runtime-manifest.json). Kotlin v0.3.0,
Swift v0.2.2 and pluginapi v0.1.1 remain exact public requirements.

Ordinary and stream dispatch, all ordinary callbacks, stream endpoint/runtime
and Handler.execute now append independent caller workspace array/offset/capacity.
Scalar/void APIs, input/output spans, reply widths, IDL and client wire remain
unchanged. Regenerate maintained subclasses and adapters; no compatibility shim
or old-signature overload is provided. See [the migration](CALLER-WORKSPACE.md)
for copyable process/processData wiring, signatures, exact pins and bounds.
Historical release notes retain their historical identities and qualification.

The facade owns composition, Counter business/APDU wiring, maintained Java and
Kotlin host fixtures, regenerated Java goldens and compatibility roots. Renderer
ownership stays in the released target; production modules have no replace.
The preserved Kotlin/Swift Counter corpus remains byte-identical. The hosted
workflow's v0.3.0 JavaCard/v0.1.0 pluginapi downloads remain explicitly historical
negative-test cache inputs; production download/prepare follows current go.mod
and runtime-manifest.json. Hosted jobs run Go packages sequentially to share one
JVM lane; local checks are not hosted evidence.

Source acceptance gates include Counter process/APDU scratch identity and
geometry, retained borrowed reference controls, exact packed/bytes/scalar/void
behavior, mixed fixed 127/190/177 dispatch and real simulator send/read, stream
invalid-window retry, cleanup/replay/liveness, facade version and signed receipt
refusals, and narrowing/token-preserving plants. `make test-cap` includes the
mixed fixture, real Classic 3.0.5u4 conversion/verifier and optional ints.
`JCRPC_FACADE_CAP_OUT` retains stream/mixed CAPs and converter logs;
`JCRPC_COUNTER_CAP_OUT` retains Counter library/wrapper CAPs. Task-scoped raw
receipts bind actual execution to the reviewed source candidate; optional lanes
without their explicit inputs remain skipped, not passing evidence.

Qualification establishes source/toolchain behavior. It does not certify Auth
RAM/NVM, physical 133-byte borrowing authority, 196/260-byte business minima,
Security Domain permissions or coordinated consumer end-to-end acceptance.
Independent source review, actual exact-head hosted JVM/Swift/iOS checks and
signed canonical branch/PR/tag/release remain release-owner gates. This document
specifies the v0.6.0 candidate and is not a producer publication receipt.

Build the facade CLI from the root source checkout (`go -C codegen build
-o jcrpc-gen ./cmd/jcrpc-gen`). Root source delivery does not claim a
`codegen/*` module tag, version-suffixed go install, uploaded JAR or Maven artifact.
