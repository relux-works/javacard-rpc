# Independent API release preparation

Proposed module version: `github.com/relux-works/javacard-rpc/pluginapi v0.1.0`.
Proposed signed submodule tag: `pluginapi/v0.1.0`. The `pluginapi/` directory
prefix belongs to the repository tag, not the module path or Go requirement.
This is a v0 module, so no `/v1` path suffix applies. There were no advertised
`pluginapi/*` tags in the authoritative origin read on 2026-10-07; the parent
must re-read immediately before publication. A failed tag read is not absence.

The core checkpoint entering this leaf is
`29342edf80af910f09cca43b12a26817d7dedfa1`, tree
`25a70579d4ce3d55893ea395e5462fc7e5834b66`. It contains the accepted v0.4.5
port and reviewed composition. The leaf outcome binds its candidate tree and
file hashes to this checkpoint. That candidate is not an accepted commit or a
landed release. The parent obtains the exact signed core commit from the managed
checkpoint/PR flow after this leaf is accepted; no producer commits/tags here.

Immutable compatibility baseline: signed release `v0.4.5`, commit
`cfed4182356a4f4609c88f58924aac79c05ae5b6`. Consumer baseline: committed
bsimId `2d23abdafa1e0f68c6003ab56274b2ac38378ef9`,
`utils/idl/bsim-auth.toml`, SHA-256
`1be1ed52ac9a85a62d5c5e371a9e22d38f834282681528476ca071e7bfc2cb66`.
Use only the staged immutable snapshot, never the live consumer working tree.

The parent must:

1. Accept this API contract and preserve all predecessor tests; run compatibility
   properties for the exact core head, including independent consumer compilation
   and the full v0.4.5 generated-byte/CLI matrix. Keep local `replace` provenance.
2. Create/review the canonical core PR and land the exact verified human-signed
   commits with required legitimate checks, preserving their signed objects.
3. Re-read authoritative tags, verify the proposed name is unused, verify the
   landed core SHA and its signature, and verify its independent `pluginapi/go.mod`
   path. Bind a fresh consumer result to that exact landed source identity.
4. Create `git tag -s pluginapi/v0.1.0 <exact-landed-core-sha> -F <release-message>`;
   verify `git verify-tag pluginapi/v0.1.0` and the peeled commit before publication.
   The release message includes module path/version, accepted PR, exact core SHA,
   consumer evidence, parity evidence and local-replace bounds.
5. Publish only that signed API tag. Validate a separate module with
   `require github.com/relux-works/javacard-rpc/pluginapi v0.1.0` and **no replace**
   after publication; unpublished local evidence cannot prove proxy availability.

Prepared release-message fields:

```text
Release pluginapi v0.1.0
Module: github.com/relux-works/javacard-rpc/pluginapi
Contract: model/options/Plugin.Generate/ordered package files; compile-time composition
Core: <exact accepted and landed signed SHA>
PR: <accepted canonical PR URL>
Evidence: <API leaf accepted CR + exact-head consumer/parity resource identifiers>
Validation: separate module/import graph; source+manifest package contract; 720-cell v0.4.5 parity
Prepublication bound: local API replacement; remote module download verified after publication
```

No target-repository extraction, runtime release, live consumer/card change or
full facade release is authorized by this leaf. The full facade release waits
for all target plugins and compatibility CI.
