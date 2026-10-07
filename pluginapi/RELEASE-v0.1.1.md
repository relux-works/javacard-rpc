# pluginapi v0.1.1 release preparation

Status: source preparation for review; no v0.1.1 publication is claimed.
Module: `github.com/relux-works/javacard-rpc/pluginapi` (Go 1.24+).
Proposed signed repository tag: `pluginapi/v0.1.1` (no module-path suffix).
Baseline: released `pluginapi/v0.1.0`.

## Release notes

- Add `Applet.StreamWorkspaceCleanup string` and the centralized constants
  `StreamWorkspaceCleanupWholeReplyArea = "whole-reply-area"` and
  `StreamWorkspaceCleanupWrittenBytesOnly = "written-bytes-only"`.
- Empty preserves released generation, including the persistent default;
  explicit modes apply only to persistent workspace. The API stores metadata;
  unknown modes/storage combinations are facade/direct-backend validation policy.
- Existing model helpers, options and `Plugin.Generate` remain unchanged.
  Zero values and keyed consumers remain compatible; positional `Applet`
  literals need the added trailing field. No dependencies, parser, validator,
  target runtime or templates are added.

The contracts for both modes are in [the API README](README.md). Downstream
parser/target implementations are separate work and require this API release.

## Parent publication handoff

1. Accept the managed uncommitted candidate through review. Bind API model,
   existing consumer and independent cleanup-consumer evidence to the accepted
   source/test/configuration/environment identity. Local API replacement proves
   candidate compilation, not remote module availability.
2. Publish/review the core PR and land the exact accepted human-signed commits
   with legitimate required checks. Preserve reviewed signed commit objects.
3. Immediately before tagging, re-read origin tags, require `pluginapi/v0.1.1`
   to be unused, verify the exact landed commit signature and module path, and
   bind fresh consumer evidence to that landed source. A failed read is unknown.
4. Create `git tag -s pluginapi/v0.1.1 <landed-sha> -F <release-message-file>`.
   Include the module/version, landed SHA, accepted PR and exact-head evidence
   references. Verify `git verify-tag pluginapi/v0.1.1` and its peeled SHA,
   then publish that signed tag.
5. Verify a separate API-only consumer requiring v0.1.1 with no local `replace`.
   Notify the target keeper immediately with the signed tag, peeled SHA and
   consumer evidence. Only then may the target pin v0.1.1.

This producer leaves the managed candidate uncommitted and attaches task-scoped
evidence; the parent owns commits, PR/review/checks, landing, tag and keeper notice.
