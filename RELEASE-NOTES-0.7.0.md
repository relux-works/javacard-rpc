# v0.7.0 source candidate

The facade admits `stream_workspace_cleanup = "authenticated-invalidate"` with
`stream_workspace = "persistent"` and refuses, at semantic validation and by
method path, every streamed response under that selector. Validation runs for
every target selection, so a Kotlin-only run refuses the same schema. Other
selectors and their diagnostics are unchanged except that the unknown-selector
message now lists both accepted values.

Generated JavaCard sources come from the target's new selector: the stream
dispatch entries take a trailing `boolean authenticated`, unauthenticated stream
APDUs are refused `6982` before any effect, refusals never write the
persistent workspace, every refused authenticated APDU (foreign method
included) invalidates the session, and CLOSE_WRITE admits only a transient or
current-APDU response span and caller workspace. See the target's `AUTHENTICATED-INVALIDATE.md`. The APDU
format and the Kotlin and Swift clients are unchanged.

## Per-method stream modes

The parser transports a per-method `stream_mode` into pluginapi v0.1.2
`Method.StreamMode`, and validation admits `"public-invalidate"` and
`"chunked"` only for a streamed method with a request stream under
`stream_workspace = "persistent"` and `stream_workspace_cleanup =
"authenticated-invalidate"`; `"chunked"` only with a short result. A streamed
response under `authenticated-invalidate` is now admitted when, and only when,
its method declares `"public-invalidate"`. Unknown values, a mode on an ordinary
method or under another applet policy are refused by method path before any
output, for every target selection.

The JavaCard target generates the matching runtime (see its `STREAM-MODES.md`):
`public-invalidate` stages accepted public chunks, writes nothing persistent on
any refused APDU and produces a streamed result into RAM on every read;
`chunked` delivers chunks to the handler as they arrive and writes nothing
persistent on any APDU. The Kotlin client output is byte-identical to a schema
without modes; Swift still does not generate stream schemas.

The repository's `pluginapi/` directory carries the same v0.1.2 change as its
own reviewed pull request, which must land and be tagged `pluginapi/v0.1.2`
first; this candidate requires it by pseudo-version until then.

## Release dependency

This candidate pins the JavaCard target by the pseudo-version of its candidate
head (`v0.5.1-0.20261014221034-a7b4bf50f1ca`) and pluginapi by the pseudo-version
of its candidate (`v0.1.2-0.20261014214040-2d009196fe38`). Before landing,
pluginapi must be tagged `pluginapi/v0.1.2`, and the target must be
independently reviewed, landed and tagged `v0.6.0` with a signed tag; then
`codegen/go.mod`, `compatibility/releases/javacard.json`,
`compatibility/runtime-manifest.json` and the CI workflow pins move to that tag.
Until then the manifest-pin tests (`TestManifestRefusals`,
`TestManifestBackendDisagreement`, `TestManifestAPIRefusals`,
`TestPinnedInputDigestRefusal`, `TestManifestReleasedWriterReceiptRefusals`,
`TestCleanupNarrowingMutants/manifest-api-old`) fail with
`backend module disagreement: javacard`, exactly as they do for a bare
pseudo-version bump with no facade change.

Independent review, signed tag and release remain release-owner work.
