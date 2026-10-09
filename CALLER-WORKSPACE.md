# Caller workspace migration — facade v0.6.0

Regenerate Java server packages using facade v0.6.0. Its exact JavaCard backend
and runtime are the actual signed target v0.5.0. No old-signature overload is
provided. Kotlin v0.3.0, Swift v0.2.2 and pluginapi v0.1.1 retain their identities;
IDL, INS, ordinary encodings and six-command stream wire are unchanged.

## Exact source pins

| Component | Tag | Peeled commit | Signed annotated tag object |
| --- | --- | --- | --- |
| JavaCard backend/runtime | v0.5.0 | `96bf35555bba8a040eba880622629ff0f25b2556` | `9bbf1db745834f3efefb54846f26bec6310f8c22` |
| Kotlin | v0.3.0 | `df7a85ea9715ae00827971ddb6608704b2f6ff29` | `6362729d42eeb8e893a169589ab018de04a38297` |
| Swift | v0.2.2 | `3db025b025238ab21b2b458bd0cf0cad285223f4` | `e0052b54b590a32faa96baad7b0a8d3fbc2efe0d` |

Use [runtime-manifest.json](compatibility/runtime-manifest.json), including its
`facade_version`, and the checked release receipts. The JavaCard native identity
is `io.jcrpc:javacard-rpc-server-javacard:0.5.0`; use the verified source root
through Gradle `includeBuild`, as described in [compatibility](compatibility/README.md).
The target [stable release](https://github.com/relux-works/javacard-rpc-server-javacard/releases/tag/v0.5.0)
is source delivery, with no uploaded JAR or Maven publication claim.

```sh
go -C codegen mod download
go -C codegen mod verify
make check-compatibility
make bootstrap-compatibility COMPAT_ROOT="$PWD/.temp/consumer"
make prepare-compatibility COMPAT_ROOT="$PWD/.temp/consumer"
make generate
```

## Ordinary callbacks and dispatch

Every ordinary callback appends this independent scratch triple. Scalar/void
return ergonomics and each request/output triple stay unchanged. Parameter names
may acquire hygienic suffixes when IDL fields collide; follow generated types/order.

```java
protected short onGet(byte[] callerWorkspace, short callerWorkspaceOffset,
        short callerWorkspaceCapacity) { return counter; }
protected void onReset(byte[] callerWorkspace, short callerWorkspaceOffset,
        short callerWorkspaceCapacity) { counter = 0; }
protected short onGetInfo(byte[] output, short outputOffset, short outputCapacity,
        byte[] callerWorkspace, short callerWorkspaceOffset, short callerWorkspaceCapacity) {
    // Counter's declared packed reply is exactly seven bytes.
    int at = outputOffset;
    at = packU16(output, at, counter);
    at = packU16(output, at, limit);
    at = packU8(output, at, VERSION);
    at = packBool(output, at, storedDataLen >= 0);
    packBool(output, at, counter == limit);
    return 7;
}
protected short onEchoMessage(byte[] input, short inputOffset, short inputLength,
        byte[] output, short outputOffset, short outputCapacity,
        byte[] callerWorkspace, short callerWorkspaceOffset, short callerWorkspaceCapacity) {
    // Consume live input before overlap; preserve output through the caller's send.
    packBytes(output, outputOffset, input, inputOffset, inputLength);
    return inputLength;
}
```

The maintained [Counter entry](examples/counter/applet/src/main/java/io/jcrpc/example/CounterJCApplet.java)
receives all short-request fragments and captures headers before overlap. Its
actual `process` lends the whole current APDU window with no new command scratch:

```java
byte[] buffer = apdu.getBuffer(); // local to this invocation
short produced = logic.dispatchTo(ins, p1, p2,
        buffer, requestOffset, requestLength,
        buffer, outputOffset, outputCapacity,
        buffer, (short) 0, (short) buffer.length);
if (produced > 0) apdu.setOutgoingAndSend(outputOffset, produced);
```

The surrounding entry must validate/receive request geometry and map the
`StatusWordException` to `ISOException`; send only on successful dispatch.
An SIO `processData` entry supplies its own explicitly authorized window instead:

```java
// ownerWorkspace/offset/capacity are lent by this processData invocation's
// caller contract. They cannot be inferred from the result or a global APDU.
public short processData(byte ins, byte p1, byte p2,
        byte[] input, short inputOffset, short inputLength,
        byte[] result, short resultOffset, short resultCapacity,
        byte[] ownerWorkspace, short ownerWorkspaceOffset, short ownerWorkspaceCapacity) {
    return logic.dispatchTo(ins, p1, p2,
            input, inputOffset, inputLength,
            result, resultOffset, resultCapacity,
            ownerWorkspace, ownerWorkspaceOffset, ownerWorkspaceCapacity);
}
// The entry owner consumes only the successful produced result span.
```

No processData implementation is supplied by this facade; the snippet is wiring
inside a consumer's authorized entry, not a grant of Security Domain access.

## Stream callbacks and entry

```java
protected short onProcessPacketStream(byte[] input, short inputOffset, short inputLength,
        byte[] output, short outputOffset, short outputCapacity,
        byte[] callerWorkspace, short callerWorkspaceOffset, short callerWorkspaceCapacity) {
    // Input/output belong to the existing stream session. Caller scratch is
    // separate and expires with this CLOSE_WRITE or response-only invocation.
    if (inputLength > outputCapacity) throw statusWordFailure((short) 0x6700);
    packBytes(output, outputOffset, input, inputOffset, inputLength);
    return inputLength;
}
short produced = logic.dispatchStreamTo(ins, p1, p2,
        request, requestOffset, requestLength,
        response, responseOffset, responseCapacity,
        ownerWorkspace, ownerWorkspaceOffset, ownerWorkspaceCapacity);

public short processStreamData(byte ins, byte p1, byte p2,
        byte[] request, short requestOffset, short requestLength,
        byte[] response, short responseOffset, short responseCapacity,
        byte[] ownerWorkspace, short ownerWorkspaceOffset, short ownerWorkspaceCapacity) {
    return logic.dispatchStreamTo(ins, p1, p2,
            request, requestOffset, requestLength,
            response, responseOffset, responseCapacity,
            ownerWorkspace, ownerWorkspaceOffset, ownerWorkspaceCapacity);
}
```

`StreamEndpoint.dispatch`, `BoundedStreamRuntime.dispatch`, `Handler.execute`
and every `onMethodStream` require the same trailing triple. Runtime
`executeOnce` forwards it only as command-local parameters. CLOSE_WRITE receives
that entry's scratch, never an earlier WRITE's. Response-only WRITE_OR_INVOKE
receives its own invocation's scratch. Stream input/results retain their existing
session ownership and later READ lifetime.

For ordinary applet wiring, construct the generated stream adapter once, call
`streams.processIfStream(apdu)` before ordinary dispatch, and forward `deselect()`.
The generated adapter passes the current APDU array at offset zero and actual
array capacity, while keeping its existing owned 255-byte I/O and stream bulk.
For processData, use the explicit dispatchStreamTo call above with the window
lent by that processData invocation. Never retain APDU/input/output/scratch in
fields, statics, Object[] slots or ThreadLocal; no global APDU lookup or
constructor injection reconstructs borrowing authority.

## Effects, liveness and deployment limits

Null scratch, negative offset/capacity, outside-array spans and short overflow
refuse with `6700` before ordinary callbacks or stream session effects. A valid
empty span, including offset equal to array length, is legal. Invalid stream
scratch preserves the pending session for a valid retry. Other input/output,
protocol and callback failures keep their existing cleanup semantics.

Fixed replies still offer exactly 127/190/177 bytes when declared and require
exact produced count. Scratch 196/260 never substitutes for reply capacity.
Input/output/scratch may deliberately alias; the trusted business handler must
consume live input before overwrite and preserve the result through final
send/read. There is no handler sandbox or transactional rollback guarantee.

Auth's 196-byte constructor probe and 260-byte phase minimum belong in its
handwritten handler, before that phase's effects. Generic RPC has no Auth
minimum. A physical 133-byte buffer cannot meet those minima or a whole 177/190
reply. A host/simulator buffer does not establish physical capacity, Security
Domain caller permissions, consumer RAM/NVM safety, crypto-provider behavior or
end-to-end Auth acceptance. Stream persistent storage retains its documented
reset/wipe residual and NVM endurance limits and must not hold secrets.
Optional `int` support remains required by this Classic renderer.

After actual facade publication, bsim-id and KeyVault must consume the parent's
exact facade tag/commit/manifest, regenerate their own maintained callbacks,
retain handwritten phase/lazy-init guards, and qualify under their own JVM/device
authority. Local generated edits, compatibility shims and unpublished replaces
are outside the migration contract.
