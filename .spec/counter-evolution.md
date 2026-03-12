# Counter Example Evolution

## Context

`examples/counter` is now the main end-to-end baseline for `javacard-rpc`.
The example should demonstrate the intended transport/IDL boundary clearly:

- typed flat domain models should be expressed in TOML and generated into typed client/server APIs;
- opaque byte payloads should remain raw bytes and must not be force-modeled;
- gaps in the IDL type system should be visible through the example and then closed in the core toolchain.

## Goals

1. Keep opaque payloads opaque.
2. Move flat domain data into TOML-defined models where the current IDL already supports it.
3. Expose real IDL limitations through the example and then address them in `javacard-rpc` itself.
4. Make the client/service build flow teachable from inside this repo.

## Target Boundary

### Must stay raw bytes

- `load() -> Data`
- `getSpki() -> Data`
- `signChallenge(challenge: Data) -> Data`

Reason: these methods return opaque blobs or DER-encoded crypto payloads. The transport layer should not pretend they are richer domain objects.

### Must become typed flat models

- `getAppletInfo()`

Reason: applet metadata is structured domain data and the current IDL already supports flat multi-field responses that generate typed structs.

### Highlights and now covers textual scalars

- `getImsi()`
- `getDisplayName()`
- `echoMessage(message: String)`

Reason: textual payloads split into two transport shapes:
- narrow ASCII-only values like IMSI stay on `ascii`
- general user-facing text uses dynamic UTF-8 `string`

## Scope

### Story 1: Counter example uses typed flat applet metadata

- Redefine `getAppletInfo` in `examples/counter/counter.toml` as a flat response model instead of a TLV byte blob.
- Update the applet implementation, generated client/skeleton, and end-to-end tests accordingly.
- Keep `getSpki`, `signChallenge`, and `load` as raw byte APIs.

### Story 2: Add a string-like scalar to the IDL

- Design and implement a dynamic UTF-8 `string` field type alongside `ascii`.
- Extend parser, validator, Swift generator, Java generator, and tests.
- Update `counter` so it exercises both `ascii` and UTF-8 `string` end to end.

### Story 3: Teach the build path inside this repo

- Document the exact generate/build/run flow for generated client and service artifacts.
- Use `examples/counter` as the concrete walkthrough.
- Ensure the learning path still matches the real scripted entrypoints.

## Acceptance Criteria

- `getAppletInfo()` returns a generated typed model instead of `Data`.
- `getSpki()`, `signChallenge(...)`, and `load()` remain raw byte APIs.
- The repo has a tracked task board decomposition for this work.
- The example and docs distinguish `ascii` from dynamic UTF-8 `string`, and both are exercised by the example.
- `make test` and `make e2e` stay green after each shipped step.
