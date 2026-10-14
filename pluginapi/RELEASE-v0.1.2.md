# pluginapi v0.1.2 release preparation

Status: source preparation for review; no v0.1.2 publication is claimed.
Module: `github.com/relux-works/javacard-rpc/pluginapi` (Go 1.24+).
Proposed signed repository tag: `pluginapi/v0.1.2` (no module-path suffix).
Baseline: released `pluginapi/v0.1.1`.

## Release notes

- Add `Method.StreamMode string` and the centralized constants
  `StreamModePublicInvalidate = "public-invalidate"` and
  `StreamModeChunked = "chunked"`.
- Empty keeps the applet-level stream policy. The API stores the value only;
  unknown values and invalid combinations are facade and direct-backend
  validation policy.
- Zero values, keyed literals, struct equality, model helpers, options and
  `Plugin.Generate` are unchanged. Positional `Method` literals need the added
  trailing field. No dependency, parser, validator, runtime or template is added.

The contracts are in [the API README](README.md). The JavaCard backend v0.6.0
implements both modes and the facade v0.7.0 parses and validates
`stream_mode`; both consume this API version.

## Release order

This API must be tagged before the JavaCard backend v0.6.0 tag, because the
backend module requires it, and the facade then pins both.

1. Review and land this API change on the default branch with its signed
   commits preserved.
2. Immediately before tagging, re-read origin tags, require `pluginapi/v0.1.2`
   to be unused and verify the landed commit signature and module path.
3. `git tag -s pluginapi/v0.1.2 <landed-sha>`; verify the tag and its peeled SHA.
4. Move the backend `go.mod` requirement from the candidate pseudo-version to
   `v0.1.2`, then continue the backend v0.6.0 release.
