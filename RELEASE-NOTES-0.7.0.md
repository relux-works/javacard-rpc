# javacard-rpc v0.7.0 — authenticated-invalidate and per-method stream modes

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
"authenticated-invalidate"`. A streamed response under
`authenticated-invalidate` is now admitted when, and only when, its method
declares a mode. Unknown values, a mode on an ordinary
method or under another applet policy are refused by method path before any
output, for every target selection.

The JavaCard target generates the matching runtime (see its `STREAM-MODES.md`):
`public-invalidate` stages accepted public chunks, writes nothing persistent on
any refused APDU and produces a streamed result into RAM on every read;
`chunked` delivers chunks to the handler as they arrive and writes nothing
persistent on any APDU, producing a streamed result into RAM on every read.
Owner assignment for Auth: 0x10 `public-invalidate`; 0x20/0x30/0x40/0x70
`chunked`. The Kotlin client output is byte-identical to a schema
without modes; Swift still does not generate stream schemas.

## Release dependencies

The facade composes signed immutable releases only; production modules have
no `replace` and no pseudo-version:

| Component | Release | Peeled commit | Signed tag object |
| --- | --- | --- | --- |
| pluginapi | `pluginapi/v0.1.2` | `4e10e2cfd25483bb4e009f8ac8eafb1d8d0f2a87` | `94cbf339afbd6260a7100b906a59f2178bc9b399` |
| JavaCard backend/runtime | `v0.6.0` (`io.jcrpc:javacard-rpc-server-javacard:0.6.0`) | `46fa6ecb45135be0ba9b1d4b9cd9aa8663718510` | `b43b8d6d2834f57d2a1e899b99fe22996e2709ee` |
| Kotlin backend/runtime | `v0.3.0` (unchanged) | `df7a85ea9715ae00827971ddb6608704b2f6ff29` | `6362729d42eeb8e893a169589ab018de04a38297` |
| Swift backend/runtime | `v0.2.2` (unchanged) | `3db025b025238ab21b2b458bd0cf0cad285223f4` | `e0052b54b590a32faa96baad7b0a8d3fbc2efe0d` |

`codegen/go.mod`, `compatibility/releases/javacard.json` and
`compatibility/runtime-manifest.json` (facade metadata `0.7.0`) carry that
tuple. `jcrpc-compat --mode check` requires the resolved graph to select exactly
these versions and pluginapi v0.1.2, and now also refuses any manifest tag that
is not a plain release version, so a pseudo-version cannot pass by being written
into both the manifest and its receipt. Bootstrap still verifies each tag object,
peeled commit and signature against `compatibility/allowed_signers`.

The hosted workflow additionally caches pluginapi v0.1.1, JavaCard v0.5.0 and
the two candidate pseudo-versions
(`v0.1.2-0.20261014223732-4e10e2cfd254`, `v0.5.1-0.20261014224032-50f13333ded2`)
as negative-test fixtures for the previous-release and pseudo-version refusals.
They are not production requirements.

Independent review, exact-head hosted JVM/Swift checks and the signed `v0.7.0`
tag remain release-owner gates; this document is not a publication receipt.
