# javacard-rpc IDL Specification (TOML)

## 1. Overview

The javacard-rpc IDL defines Java Card APDU contracts in TOML.

- One schema file describes one applet contract.
- The expected file extension is `.toml`.
- The schema drives typed client/server code generation.

Top-level sections:

- `[applet]` (required)
- `[methods.<name>]` (required; at least one method must exist)
- `[status_words]` (optional)

Unknown TOML keys are rejected by the parser.

## 2. `[applet]` Section (Required)

`[applet]` defines applet-level metadata.

| Key | Type | Required | Rules |
|---|---|---|---|
| `name` | string | yes | Must be non-empty after trim. |
| `description` | string | no | Free text. |
| `version` | string | yes | Must match strict semver `X.Y.Z` (numeric only). |
| `aid` | string | yes | Hex string, 5-16 bytes (10-32 hex chars), even length, no surrounding whitespace. |
| `cla` | integer | yes | Must be in byte range `0x00..0xFF`; `0x00` is forbidden. |
| `stream_workspace` | string | no | `"transient"` (default, also an empty setting) or `"persistent"`; only bulk Java stream workspace changes storage. Unknown values are rejected. |

Notes:

- TOML integer forms are accepted (`0xB0`, `176`, etc.); hex is recommended for APDU values.
- Validation rejects `cla = 0x00` as ISO 7816 reserved.
- Persistent stream workspace keeps all control state and scratch transient;
  it is independent of `--stream-memory`. Explicit abort/deselect/failure and
  closing a read overwrite workspace bytes. Reset invalidates the session but
  leaves persistent bytes until the next stream dispatch cleans them. NVM
  writes consume endurance; interrupted wiping is not atomic or secure erasure.
- Stream adapter CLA matching follows Java Card RE §4.3 channel coding and
  preserves chaining/security indicators. `0xFF` is reserved and refused;
  `0xFX` cannot encode channel 19. Proprietary first-coding bit 6 has no
  further-coding counterpart. RFU `0x2X/0x3X` keeps exact matching.

## 3. `[methods.<name>]` Section

Each method is declared under a method key:

```toml
[methods.increment]
ins = 0x01
description = "Increment counter"
```

`<name>` rules:

- Must match identifier regex: `^[A-Za-z][A-Za-z0-9_]*$`.

Method fields:

| Key | Type | Required | Rules |
|---|---|---|---|
| `ins` | integer | yes | Byte range `0x00..0xFF`; unique across methods; reserved ranges forbidden. A streamed method reserves this value plus the next five values. |
| `description` | string | no | Free text. |
| `[methods.<name>.request]` | table | no | Request message definition. |
| `[methods.<name>.response]` | table | no | Response message definition. |

Message shape:

- `request`/`response` tables define `fields`.
- `fields` is an array of field objects.
- A method with no request and no response is valid.

`INS` constraints:

- A non-streamed method reserves exactly its declared value.
- A streamed method reserves `ins..ins+5`; the complete range must fit in one
  byte, avoid ISO-reserved ranges and not overlap any other method range.
- Reserved ranges (invalid): `0x60..0x6F`, `0x90..0x9F`.

## 4. Field Types (Full Set)

| Type | Size | Description |
|---|---|---|
| `u8` | 1 byte | Unsigned 8-bit integer. |
| `u16` | 2 bytes | Unsigned 16-bit integer, big-endian. |
| `u32` | 4 bytes | Unsigned 32-bit integer, big-endian. |
| `bool` | 1 byte | Boolean encoded as `0x00=false`, `0x01=true`. |
| `ascii` | variable | ASCII string encoded as raw 7-bit bytes. |
| `string` | variable | Variable-length UTF-8 string encoded in response/request data. |
| `bytes` | variable | Variable-length byte array. |
| `bytes[N]` | `N` bytes | Fixed-length byte array (`N > 0`). |
| `stream` | bounded, multi-APDU | One request or response value transferred through the generated half-duplex stream lifecycle. |

## 5. Field Definition

Each field object in `fields = [...]` uses:

| Key | Type | Required | Rules |
|---|---|---|---|
| `name` | string | yes | Must match identifier regex: `^[A-Za-z][A-Za-z0-9_]*$`. |
| `type` | string | yes | One of `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`, `stream`. |
| `location` | string | no | `p1`, `p2`, or `data` (case-insensitive). |
| `length` | integer | no | Only valid when `type = "bytes"` or `type = "ascii"`; must be `> 0`. |
| `max_length` | integer | for `stream` | Maximum complete value size, `1..32767` for Java Card array addressing, and no more than `chunk_size * 255`. |
| `chunk_size` | integer | for `stream` | Maximum chunk data size, `1..255`. |

Notes:

- `bytes[N]` and `bytes + length` are both parsed as fixed-size byte payloads.
- `ascii + length` is parsed as a fixed-size ASCII payload.
- `string` is always dynamic-length UTF-8 and does not support `length`.
- `length` is invalid for non-`bytes` and non-`ascii` types.
- For `bytes[N]`, `N` must be greater than zero.
- `stream` does not use `length`. It requires both `max_length` and
  `chunk_size`, cannot use `p1` or `p2`, and must be the only field in its
  request or response message. A method may declare at most one request stream
  and at most one response stream.
- `P1` and `P2` are reserved for the stream lifecycle for the whole method. A
  response-only stream therefore carries ordinary request fields in APDU data,
  not in `P1` or `P2`.

### 5.1 Stream example

```toml
[methods.processPacket]
ins = 0x20
description = "Process one bounded packet and return one bounded result"

[methods.processPacket.request]
fields = [
  { name = "requestPacket", type = "stream", max_length = 1792, chunk_size = 192 }
]

[methods.processPacket.response]
fields = [
  { name = "resultPacket", type = "stream", max_length = 1792, chunk_size = 192 }
]
```

This method owns `INS 0x20..0x25`. `P1` and `P2` are unsigned packet index and
packet count for chunk commands. Packet zero implicitly opens the stream; there
is no wire stream identifier. Code generation assigns each streamed method an
internal method identifier and routes every streamed method through one
applet-level session manager. A selected applet can therefore own only one
active stream lifecycle at a time. An operation for another streamed method is
rejected without destroying the current owner's recoverable state.

| Offset | Command | Command data | Response data |
| ---: | --- | --- | --- |
| `+0` | write request chunk, or invoke a response-only stream method | chunk, or ordinary bounded request | empty, or 35-byte result descriptor |
| `+1` | close request stream | `totalLength:u16be || sha256:bytes[32]` | ordinary short result or 35-byte result descriptor |
| `+2` | get pending result info | empty | 35-byte result descriptor |
| `+3` | read result chunk | empty | chunk |
| `+4` | close result stream | `totalLength:u16be || sha256:bytes[32]` | empty |
| `+5` | abort | empty | empty |

The descriptor is
`packetCount:u8 || totalLength:u16be || sha256:bytes[32]`. Input packets must be
sequential and repeat the same count. Repeating the immediately preceding
packet with identical bytes is idempotent. Every packet except the final one is
exactly `chunk_size` bytes; the final packet is non-empty and no larger than
`chunk_size`. A gap, conflicting replay, changed count, invalid chunk layout,
wrong direction or digest/length mismatch fails closed. The first valid
write close executes the typed handler once. For an ordinary short result, an
identical write-close retry returns the retained result without executing the
handler again. For a streamed result, the client recovers a lost write-close
response through `+2`. Response transfer is client-pulled. An identical read
close retry succeeds from a descriptor-only receipt after result bytes have
already been wiped.

The generated Kotlin client uploads request chunks, closes the request with its
length and digest, pulls the described response chunks, verifies the complete
response digest and closes the response. It retries a write chunk, pending-info
read, result chunk, or close once with identical command bytes after a transport
failure; all of those operations are idempotent in the corresponding state. It
does not retry a response-only invocation or a write close that produces a
streamed result. If either response is lost, the client recovers the already
prepared descriptor through `+2`, so the typed handler is not executed again.
A repeated response-only `+0` while that descriptor remains pending fails with
the wrong-state status and does not clear or replace the pending result.
Any remaining host failure triggers a best-effort `+5` abort. The generated
client then synchronously calls the shared `APDUTransport.invalidateSession()` so the
transport can close its logical channel or otherwise require a fresh select.
This invalidation still runs when coroutine cancellation prevents the suspend
abort from reaching the card. Cleanup is best effort: an abort or invalidation
failure cannot replace the original protocol failure or cancellation.
Coroutine cancellation is propagated rather than
retried. One generated atomic owner guard covers the complete typed operation.
A concurrent streamed call fails locally with `StreamBusy` and does not send an
abort or any other APDU that could disturb the active operation.

The Java output owns the session lifecycle inside generated code. The generated
skeleton constructs one runtime, one maximum-sized shared workspace (transient
by default), transient scalar state, a transient handler reference, digest
scratch, SHA-256, and reusable status exceptions. Persistent workspace adds a
one-byte transient reset marker. The generated
APDU adapter reads all incoming short-APDU fragments, invokes the generated
dispatcher, and sends the result from preallocated transient storage. The
developer implements only typed `(buffer, offset, length, output, capacity)`
handlers and calls the generated `failStream(statusWord)` helper for a business
failure; the developer does not implement a session state machine.
For a streamed method with an ordinary fixed-width response, the dispatcher
passes the exact declared width as the handler's output capacity and rejects a
returned length that differs. Every ordinary response of a streamed method must
fit the 255-byte short-response ceiling. A variable ordinary response receives
that full capacity; a fixed response may declare at most 255 bytes.

The concrete `Applet` owns only lifecycle wiring: it constructs one generated
APDU adapter, calls `processIfStream(apdu)` before ordinary dispatch, and
forwards `deselect()` to the adapter. The generated skeleton and adapter remain
the sole owners of stream session state.

The generated command path does not allocate an object or array. With transient
workspace, reset clears every session array. With persistent workspace, reset
clears control state and its reset marker, invalidating the session immediately;
retained workspace bytes are overwritten on the next stream dispatch before
reuse or stale-session rejection. Explicit abort, applet deselect, first
successful read close and terminal validation failure overwrite the full
workspace. Class rejection and foreign-method refusal preserve a live owner's
session. Persistent scalar metadata is never used. Cryptographic purpose
and authorization remain properties of the typed method implementation; a
stream field does not authorize generic signing or decryption.

### 5.2 Bulk workspace storage policy

`[applet] stream_workspace = "persistent"` opts the entire applet's one shared
bulk byte workspace into NVM. Omission, an empty setting, or `"transient"` keeps
the default. Unknown values fail validation before CLI output is written.
`--stream-memory clear_on_deselect|clear_on_reset` independently selects the
clearing event for transient control/scratch arrays; reusable failure status
always uses `CLEAR_ON_RESET`. Only the persistent policy adds a transient reset
marker. The wire protocol and shared ownership remain unchanged.

Use persistent workspace only for large, infrequent frames containing open
data. Secrets must never be staged in NVM. The current IDL field/message model
has no secrecy annotation: the generator cannot infer semantic secrecy or
enforce this restriction. It does not guess from field or method names. The
applet author must audit every streamed method before enabling an applet-wide
persistent policy; a streamed secret makes this policy unsuitable.

For workspace capacity `W`, construction zeroes `W` bytes once. Successful
upload of `L` bytes assigns those `L` workspace bytes; an identical chunk retry
compares bytes without rewriting them. Handler output writes are owned by the
handler: writing each of `R` output bytes once costs `R` byte assignments, but
handlers may write more than that. Result reads, descriptor recovery and
digest computation read NVM and write transient scratch/output. Each full
cleanup assigns zero to all `W` bytes, including unused/already-zero bytes:
abort, deselect, terminal failure, first successful read close, detected reset,
or starting again from a closed-session receipt. A repeated read close does not
wipe again. Starting after read close does wipe again. Count each cleanup
separately; this is logical byte-assignment volume, not physical programming
cycles or an endurance estimate.

Fixed short results retain their write-close receipt/result bytes for idempotent
write-close retries. They have no streamed read-close cleanup; the next session,
abort, deselect or post-reset dispatch performs the full workspace wipe.

For `W = 2048`, a 1792-byte upload, a handler writing 1792 result bytes once,
and one read-close cleanup issue `1792 + 1792 + 2048 = 5632` logical NVM byte
assignments. Starting the next session adds another 2048-byte wipe; construction
and reset/deselect/abort cleanups add their own full-capacity writes. Choose a
card's endurance budget and operation frequency accordingly. Runtime and
handler writes/wiping are not transactional: interruption or power loss can
leave torn/residual workspace bytes. Reset prevents that session from being
recovered; subsequent stream dispatch retries full cleanup. No automatic NVM
zeroing on reset, atomic wipe, or secure erasure is promised.

Simulator construction accounting for generated bsim-auth (`W = 2048`):

| Generated array payload | v0.4.4 / default | Persistent policy |
| --- | ---: | ---: |
| Transient byte/short payload | 2375 B | 328 B |
| Persistent byte/short payload | 108 B | 2156 B |
| Transient handler references | 1 slot | 1 slot |

Moving the workspace saves 2048 B and adds a 1 B transient reset marker, a net
2047 B reduction. This counts generated arrays, including static dispatch tables
and three mutable failure-status words. Object headers, reference widths,
digest/provider/JCRE allocations and physical-card memory totals are excluded.

### 5.3 Stream adapter CLA comparison

The stream adapter checks instruction membership before CLA, preserving ordinary
fallback. First coding removes only channel bits with `CLA & 0xFC` and compares
the remaining class/chaining/security fields. Further coding removes channel
bits with `CLA & 0xF0`. For a first-coded configured CLA, the further-coding
comparison value is `(base & 0x90) | 0x40`, plus `0x20` when `(base & 0x0C)` is
nonzero. For a further-coded base, the first-coding comparison value is
`(base & 0x90)`, plus canonical `0x0C` when `(base & 0x20)` is nonzero.
Proprietary first-coding bit 6 is application-defined and has no further-coding
counterpart; two first-coding SM bits collapse to one further-coding indicator.
That representation is non-injective and does not preserve an arbitrary
application class bit or distinguish first-coding SM modes in further coding.

For base `0xB6` (or channel-normalized `0xB4`), the exact condition is
`CLA != 0xFF && ((CLA & 0xFC) == 0xB4 || (CLA & 0xF0) == 0xF0)`:
channels 0–3 use `B4..B7`, channels 4–18 use `F0..FE`. Channel 19 would require
reserved `FF` and is unavailable for this class. Other applicable classes can
encode channels 4–19 (for example `80..83` / `C0..CF`). Interindustry RFU
`2X/3X` configured values retain exact legacy comparison; configured `FF` is
refused. CLA comparison does not authorize secure messaging, implement command
chaining, open a logical channel, or select an applet: those remain JCRE/app
responsibilities. Physical channel/selection support is a separate platform
constraint. These encodings follow [Oracle Java Card RE 3.2 §4.3, Tables
4-2/4-3](https://docs.oracle.com/en/java/javacard/3.2/jc-re-spec/F74157_03.pdf).

## 6. Parameter Location Inference Rules

### 6.1 Request fields (`[methods.<name>.request]`)

If any field has explicit `location`:

1. Explicit `p1`/`p2`/`data` is respected.
2. Any field without explicit location is set to `data`.
3. Duplicate `p1` or duplicate `p2` is invalid.

If no field has explicit `location`:

1. If request has exactly one field and it is `u8` or `bool`: it is inferred as `p1`.
2. If request has exactly two fields and both are `u8`/`bool`: first is `p1`, second is `p2`.
3. Otherwise, all request fields are inferred as `data`.

### 6.2 Response fields (`[methods.<name>.response]`)

- No auto-location inference is performed for response fields.
- In practice, generated decoders read response fields from response data sequentially.

### 6.3 Location/type compatibility

- `p1` and `p2` locations are only valid for `u8` or `bool`.
- `u16`, `u32`, `ascii`, `string`, `bytes`, `bytes[N]` must be carried in data.
- A method containing a request or response `stream` cannot use `p1` or `p2`
  for ordinary request fields because the lifecycle owns both bytes.

### 6.4 Wire encoding model

javacard-rpc does not embed Protocol Buffers or another general-purpose object
serializer. Generated code reads and writes declared fields directly in IDL
order: unsigned integers are big-endian, booleans use one byte, fixed byte
arrays keep their declared size, and the final variable-length field consumes
the remaining APDU data. A `stream` carries one bounded opaque byte array over
several APDUs; serialization inside that byte array belongs to the application
contract, not to the stream transport.

When every ordinary response field has a fixed width, that width is exact.
Generated Kotlin decoding rejects both shorter and longer response data, and
generated Java dispatch rejects an implementation result whose encoded length
does not equal the declared total. A variable final field is the only ordinary
response form that may consume an otherwise unconstrained suffix.

Generated Kotlin modules depend on
`io.jcrpc:javacard-rpc-client-kotlin:0.2.0`. Their clients accept the runtime's
shared `APDUTransport` directly; the generator does not declare a schema-local
transport interface or result wrapper.

## 7. `[status_words]` Section (Optional)

`[status_words]` maps symbolic names to APDU status word codes.

```toml
[status_words]
SW_OVERFLOW = { code = 0x6986, description = "Counter would exceed limit" }
```

Entry format:

| Part | Type | Required | Rules |
|---|---|---|---|
| key (e.g. `SW_OVERFLOW`) | identifier | yes | Must match `^[A-Za-z][A-Za-z0-9_]*$`. |
| `code` | integer | yes | Parsed as `uint16`; valid ranges: `0x6000..0x6FFF` or `0x9000..0x9FFF`. |
| `description` | string | no | Free text. |

Additional constraints:

- Status word codes must be unique.

## 8. Validation Rules (Consolidated)

The following semantic checks are enforced:

1. `applet.name` must be non-empty.
2. `applet.version` must match strict `X.Y.Z` semver.
3. `applet.aid` must be valid hex, even length, and 5-16 bytes.
4. `applet.cla` must be byte-sized and not `0x00`.
5. At least one method must be declared.
6. Method names must be valid identifiers.
7. A normal `ins` value, or every value in a streamed method's derived six-INS range, must be byte-sized, collision-free, and outside reserved ranges `0x60..0x6F` / `0x90..0x9F`.
8. Field names must be valid identifiers.
9. Field type must be one of `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`, `stream`.
10. `length` is allowed only for `bytes` and `ascii`, and must be `> 0`.
11. For `bytes[N]`, `N` must be `> 0`.
12. `location` must be one of `p1`, `p2`, `data` when present.
13. `p1`/`p2` fields must be `u8` or `bool`.
14. Duplicate `p1` or duplicate `p2` fields are invalid.
15. A stream field requires `max_length` and `chunk_size`, must fit in at most 255 chunks, cannot occupy `p1`/`p2`, and must be the only field in that message; the containing method cannot assign ordinary request fields to `p1`/`p2` either. If that method returns an ordinary fixed-width response, its complete declared width must fit the 255-byte short-response ceiling.
16. Status word names must be valid identifiers.
17. Status word codes must be unique.
18. Status word code must be in `0x6000..0x6FFF` or `0x9000..0x9FFF`.
19. Unknown TOML keys are rejected during parse.

## 9. Complete Annotated `counter.toml` Example

`examples/counter/counter.toml` in this repository currently contains a subset.
For broad type coverage (`u32`, `bool`, `bytes[N]`), parser/validator tests use `codegen/testdata/counter.toml`, while targeted parser/validator tests cover `ascii` and `string`.
The annotated example below is the full tested variant.

```toml
# Applet metadata: required identity and APDU class byte.
[applet]
name = "Counter"
description = "Simple counter with increment, decrement, get, reset"
version = "1.0.0"
aid = "F000000101"
cla = 0xB0

# Method with request in P1 (single u8 infers p1) and u16 response data.
[methods.increment]
ins = 0x01
description = "Increment counter by amount"
[methods.increment.request]
fields = [{ name = "amount", type = "u8" }]
[methods.increment.response]
fields = [{ name = "value", type = "u16" }]

# Same shape as increment.
[methods.decrement]
ins = 0x02
description = "Decrement counter by amount"
[methods.decrement.request]
fields = [{ name = "amount", type = "u8" }]
[methods.decrement.response]
fields = [{ name = "value", type = "u16" }]

# Method with response only.
[methods.get]
ins = 0x03
description = "Get current counter value"
[methods.get.response]
fields = [{ name = "value", type = "u16" }]

# Method with no request and no response.
[methods.reset]
ins = 0x04
description = "Reset counter to zero"

# Request u16 goes to APDU data.
[methods.setLimit]
ins = 0x05
description = "Set upper limit for counter"
[methods.setLimit.request]
fields = [{ name = "limit", type = "u16" }]

# Multi-field response payload in APDU response data.
[methods.getInfo]
ins = 0x06
description = "Get counter state: value, limit, version"
[methods.getInfo.response]
fields = [
    { name = "value", type = "u16" },
    { name = "limit", type = "u16" },
    { name = "version", type = "u8" }
]

# Variable-length bytes request field in APDU data.
[methods.store]
ins = 0x07
description = "Store arbitrary data blob (up to 128 bytes)"
[methods.store.request]
fields = [{ name = "data", type = "bytes" }]

# Variable-length bytes response field.
[methods.load]
ins = 0x08
description = "Load previously stored data blob"
[methods.load.response]
fields = [{ name = "data", type = "bytes" }]

# u32 request in APDU data.
[methods.setCount]
ins = 0x09
description = "Set current counter value"
[methods.setCount.request]
fields = [{ name = "value", type = "u32" }]

# bool request in P1 (single bool infers p1).
[methods.setEnabled]
ins = 0x0A
description = "Enable or disable counter"
[methods.setEnabled.request]
fields = [{ name = "enabled", type = "bool" }]

# Variable-length ASCII response.
[methods.getImsi]
ins = 0x0B
description = "Return IMSI digits"
[methods.getImsi.response]
fields = [{ name = "imsi", type = "ascii" }]

# Variable-length UTF-8 string response.
[methods.getDisplayName]
ins = 0x0C
description = "Return a localized display name"
[methods.getDisplayName.response]
fields = [{ name = "displayName", type = "string" }]

# Fixed-size bytes response using bytes[N].
[methods.getHash]
ins = 0x0D
description = "Get SHA-256 hash of stored data"
[methods.getHash.response]
fields = [{ name = "hash", type = "bytes[32]" }]

# Optional status word catalog.
[status_words]
SW_UNDERFLOW = { code = 0x6985, description = "Counter would go below zero" }
SW_OVERFLOW = { code = 0x6986, description = "Counter would exceed limit" }
SW_NO_DATA = { code = 0x6A88, description = "No data stored" }
SW_DATA_TOO_LONG = { code = 0x6A80, description = "Data exceeds max size" }
```

## 10. Highlights / Key Takeaways

- The IDL surface is compact: `applet`, `methods`, optional `status_words`.
- Request location inference is intentionally narrow: only the exact `1x` or `2x` (`u8`/`bool`) cases map to `p1`/`p2`; all other implicit cases go to data.
- `bool`, `u32`, `ascii`, `string`, and `bytes[N]` are first-class types.
- Status words are constrained to ISO 7816 ranges `0x6000..0x6FFF` and `0x9000..0x9FFF`.
- Parser rejects unknown keys, so schemas are closed-world and typo-resistant.

## 11. Fact-Checking Matrix (Claims -> Sources)

| Claim | Verified In |
|---|---|
| Root sections and model structure (`applet`, `methods`, `status_words`) | `codegen/model.go:4-8` |
| Supported field types include `u8/u16/u32/bool/ascii/string/bytes/bytes[N]` | `codegen/model.go`, `codegen/parser.go`, `codegen/validator.go` |
| Unknown TOML keys are rejected | `codegen/parser.go:67-74` |
| Request location inference algorithm (explicit mode, 1-field, 2-field, fallback-to-data) | `codegen/parser.go:198-256` |
| `location` accepted values are `p1`, `p2`, `data` (case-insensitive) | `codegen/parser.go:281-291` |
| AID validation and semver format | `codegen/validator.go:47-57`, `codegen/validator.go:251-259` |
| `cla` cannot be `0x00` | `codegen/validator.go:51-53` |
| `ins` uniqueness + reserved ranges | `codegen/validator.go:96-104`, `codegen/validator.go:243-245` |
| `p1/p2` allowed only for `u8`/`bool` and duplicate `p1/p2` is invalid | `codegen/validator.go:145-166`, `codegen/validator.go:239-241` |
| Status word valid ranges are `0x6000..0x6FFF` or `0x9000..0x9FFF`, codes unique | `codegen/validator.go:196-204`, `codegen/validator.go:247-249` |
| Full counter example with `u32`, `bool`, `bytes[32]` is present in test fixture, and targeted tests cover `ascii` and `string` | `codegen/testdata/counter.toml`, `codegen/parser_test.go`, `codegen/validator_test.go` |
