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

Notes:

- TOML integer forms are accepted (`0xB0`, `176`, etc.); hex is recommended for APDU values.
- Validation rejects `cla = 0x00` as ISO 7816 reserved.

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
client then synchronously calls `transport.invalidateStreamSession()` so the
transport can close its logical channel or otherwise require a fresh select.
This invalidation still runs when coroutine cancellation prevents the suspend
abort from reaching the card. Cleanup is best effort: an abort or invalidation
failure cannot replace the original protocol failure or cancellation.
Coroutine cancellation is propagated rather than
retried. One generated atomic owner guard covers the complete typed operation.
A concurrent streamed call fails locally with `StreamBusy` and does not send an
abort or any other APDU that could disturb the active operation.

The Java output owns the session lifecycle inside generated code. The generated
skeleton constructs one runtime, one maximum-sized transient workspace, digest
scratch, a reset marker, SHA-256, and reusable status exceptions. The generated
APDU adapter reads all incoming short-APDU fragments, invokes the generated
dispatcher, and sends the result from preallocated transient storage. The
developer implements only typed `(buffer, offset, length, output, capacity)`
handlers and calls the generated `failStream(statusWord)` helper for a business
failure; the developer does not implement a session state machine.

The concrete `Applet` owns only lifecycle wiring: it constructs one generated
APDU adapter, calls `processIfStream(apdu)` before ordinary dispatch, and
forwards `deselect()` to the adapter. The generated skeleton and adapter remain
the sole owners of stream session state.

The command path does not allocate an object or array. Reset, applet deselect,
explicit abort and every terminal validation failure wipe the active workspace.
The transient reset marker also prevents persistent scalar metadata from being
mistaken for a live session after card reset or deselect. Cryptographic purpose
and authorization remain properties of the typed method implementation; a
stream field does not authorize generic signing or decryption.

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
15. A stream field requires `max_length` and `chunk_size`, must fit in at most 255 chunks, cannot occupy `p1`/`p2`, and must be the only field in that message; the containing method cannot assign ordinary request fields to `p1`/`p2` either.
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
