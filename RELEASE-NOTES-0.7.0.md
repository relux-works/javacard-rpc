# v0.7.0 source candidate

The facade admits `stream_workspace_cleanup = "authenticated-invalidate"` with
`stream_workspace = "persistent"` and refuses, at semantic validation and by
method path, every streamed response under that selector. Validation runs for
every target selection, so a Kotlin-only run refuses the same schema. Other
selectors and their diagnostics are unchanged except that the unknown-selector
message now lists both accepted values.

Generated JavaCard sources come from the target's new selector: the stream
dispatch entries take a trailing `boolean authenticated`, unauthenticated stream
APDUs are refused `6982` before any effect, and refusals never write the
persistent workspace. See the target's `AUTHENTICATED-INVALIDATE.md`. The APDU
format and the Kotlin and Swift clients are unchanged.

## Release dependency

This candidate pins the JavaCard target by the pseudo-version of its candidate
head (`v0.5.1-0.20261014155439-511b8ba480e5`). Before landing, the target must
be independently reviewed, landed and tagged `v0.6.0` with a signed tag; then
`codegen/go.mod`, `compatibility/releases/javacard.json`,
`compatibility/runtime-manifest.json` and the CI workflow pins move to that tag.
Until then the manifest-pin tests (`TestManifestRefusals`,
`TestManifestBackendDisagreement`, `TestManifestAPIRefusals`,
`TestPinnedInputDigestRefusal`, `TestManifestReleasedWriterReceiptRefusals`,
`TestCleanupNarrowingMutants/manifest-api-old`) fail with
`backend module disagreement: javacard`, exactly as they do for a bare
pseudo-version bump with no facade change.

Independent review, signed tag and release remain release-owner work.
