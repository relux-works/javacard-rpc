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
| `ins` | integer | yes | Byte range `0x00..0xFF`; unique across methods; reserved ranges forbidden. |
| `description` | string | no | Free text. |
| `[methods.<name>.request]` | table | no | Request message definition. |
| `[methods.<name>.response]` | table | no | Response message definition. |

Message shape:

- `request`/`response` tables define `fields`.
- `fields` is an array of field objects.
- A method with no request and no response is valid.

`INS` constraints:

- Must be unique across all methods.
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

## 5. Field Definition

Each field object in `fields = [...]` uses:

| Key | Type | Required | Rules |
|---|---|---|---|
| `name` | string | yes | Must match identifier regex: `^[A-Za-z][A-Za-z0-9_]*$`. |
| `type` | string | yes | One of `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`. |
| `location` | string | no | `p1`, `p2`, or `data` (case-insensitive). |
| `length` | integer | no | Only valid when `type = "bytes"` or `type = "ascii"`; must be `> 0`. |

Notes:

- `bytes[N]` and `bytes + length` are both parsed as fixed-size byte payloads.
- `ascii + length` is parsed as a fixed-size ASCII payload.
- `string` is always dynamic-length UTF-8 and does not support `length`.
- `length` is invalid for non-`bytes` and non-`ascii` types.
- For `bytes[N]`, `N` must be greater than zero.

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
7. `ins` values must be byte-sized, unique, and not in reserved ranges `0x60..0x6F` / `0x90..0x9F`.
8. Field names must be valid identifiers.
9. Field type must be one of `u8`, `u16`, `u32`, `bool`, `ascii`, `string`, `bytes`, `bytes[N]`.
10. `length` is allowed only for `bytes` and `ascii`, and must be `> 0`.
11. For `bytes[N]`, `N` must be `> 0`.
12. `location` must be one of `p1`, `p2`, `data` when present.
13. `p1`/`p2` fields must be `u8` or `bool`.
14. Duplicate `p1` or duplicate `p2` fields are invalid.
15. Status word names must be valid identifiers.
16. Status word codes must be unique.
17. Status word code must be in `0x6000..0x6FFF` or `0x9000..0x9FFF`.
18. Unknown TOML keys are rejected during parse.

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
