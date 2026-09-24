# javacard-rpc 0.4.2

A generated Java Card skeleton for a streamed applet is smaller. Nothing about the
wire protocol, the instruction map or the refusals changes.

## Changed

- The stream dispatcher is table driven. A streamed method owns six consecutive
  instructions, `base + 0..5`, so the instruction alone decodes to a method id and
  an operation. Where the generator used to emit one `switch` case per instruction
  — six near-identical 18-argument calls per streamed method, plus the same list
  again in `isStreamInstruction` — it now emits three constant tables and one call
  site. On a nine-method IDL that is 54 cases replaced by 9 table rows.
- The private decode helpers a schema never calls (`readU8`, `readU16`, `readU32`,
  `readBool`, `slice`, `copyBytes`) are no longer emitted. The pass repeats until
  nothing more drops out, because removing one helper can orphan another's only
  caller.

## Not changed

- The `protected` `packU8`/`packBool`/`packU16`/`packU32`/`packBytes` helpers are
  always emitted. They are part of the skeleton's surface for the hand-written
  applet that extends it — the counter example packs its own response with them —
  and the generator cannot see those call sites.
- `dispatchStreamTo`, `isStreamInstruction`, `abortStreams` and the stream runtime
  and endpoint keep their signatures. A consumer recompiles and relinks; no source
  change is required.

## Evidence

- `TestGeneratedJavaStreamDispatchDecodesEveryInstruction` compiles the generated
  skeleton against a recording runtime and checks all 256 instruction values: each
  of the six instructions of every streamed family must arrive with that family's
  method id, operation and limits, and every other instruction must be refused with
  `6D00` without reaching the runtime. The fixture places two families back to back
  and leaves holes elsewhere, so an off-by-one in the range check cannot pass.
- `TestGeneratedJavaSkeletonKeepsOnlyThePrivateHelpersItCalls` and
  `TestGeneratedJavaSkeletonPruningIsVisibleBetweenSchemas` pin the pruning in both
  directions: a called helper must survive, an uncalled one must not, and the two
  fixtures must actually disagree.
- Measured on the BSimID Auth applet (nine streamed methods, Java Card Classic
  3.0.5, `jc320v25.1_kit`): Load File Data Block 32 244 → 28 441 octets, −3 803.
- `make release-check` green, including `TestGeneratedJavaStreamPackageConvertsToCAP`.
