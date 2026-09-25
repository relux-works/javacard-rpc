# javacard-rpc 0.4.4

`--stream-memory clear_on_reset` now also creates the stream digest with
`externalAccess` true. Nothing else changes.

## Fixed

- In 0.4.3, `clear_on_reset` moved the stream state to `CLEAR_ON_RESET` but kept
  `MessageDigest.getInstance(ALG_SHA_256, false)` for the digest the runtime checks
  every close against. The Java Card API allows a `MessageDigest` created with
  `externalAccess` false to be used only while its owner is the selected applet, so
  a stream run from `Personalization.processData` would still fail at the first
  close. With `clear_on_reset` the digest is now created with `true`.
- The default, `clear_on_deselect`, keeps `false`; its output is identical to 0.4.3.

## Platform requirement

`clear_on_reset` needs a card that supports shared access for SHA-256 digests:
`MessageDigest.getInstance(ALG_SHA_256, true)` throws `NO_SUCH_ALGORITHM` on a card
that does not, and the skeleton is constructed at install.

## Evidence

- `TestStreamMemoryOptionSelectsTheTransientEvent` now also asserts the one stream
  digest per skeleton and its `externalAccess` value for each option.
- `make release-check` exit 0.
