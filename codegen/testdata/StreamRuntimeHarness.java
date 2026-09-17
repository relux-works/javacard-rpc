package io.jcrpc.streamdemo.server;

import java.security.MessageDigest;
import java.util.Arrays;

public final class StreamRuntimeHarness {
    private static final byte[] EMPTY = new byte[0];

    public static void main(String[] args) throws Exception {
        testBidirectionalHappyPathAndLostCloseAck();
        testResponseOnlyStream();
        testCrossMethodInterleavingPreservesOwner();
        testDeselectClearedTransientStateStartsEmpty();
        testAbortLeavesTransientStateAllZero();
        testStateMachineLivesOnlyInInjectedArrays();
        testResponsePreflightRunsBeforeHandler();
        testConflictingReplayWipesWorkspace();
        testDigestMismatchWipesWorkspace();
        testHandlerStatusFailureWipesWorkspace();
        testNegativeHandlerLengthFailsClosed();
        testShortResponseCloseIsExactlyOnce();
        testExactShortResponseLength();
        testExplicitAbortWipesWorkspace();
    }

    private static void testBidirectionalHappyPathAndLostCloseAck() throws Exception {
        Fixture fixture = new Fixture();
        CountingReverseHandler handler = new CountingReverseHandler();
        Session session = fixture.session((byte) 1, true, true, handler);
        byte[] input = bytes(300);
        byte[] response = new byte[255];

        send(session, 0, 0, 2, Arrays.copyOfRange(input, 0, 192), response);
        byte[] second = Arrays.copyOfRange(input, 192, input.length);
        send(session, 0, 1, 2, second, response);
        send(session, 0, 1, 2, second, response);

        byte[] close = closeData(input);
        int descriptorLength = send(session, 1, 0, 0, close, response);
        require(descriptorLength == 35, "descriptor length");
        require(handler.calls == 1, "handler must execute once");
        byte[] descriptor = Arrays.copyOf(response, descriptorLength);

        Arrays.fill(response, (byte) 0);
        int pendingLength = send(session, 2, 0, 0, EMPTY, response);
        require(Arrays.equals(descriptor, Arrays.copyOf(response, pendingLength)), "pending descriptor");

        int packetCount = descriptor[0] & 0xFF;
        byte[] result = new byte[input.length];
        int resultOffset = 0;
        for (int index = 0; index < packetCount; index++) {
            int length = send(session, 3, index, packetCount, EMPTY, response);
            System.arraycopy(response, 0, result, resultOffset, length);
            resultOffset += length;
        }
        require(Arrays.equals(reverseCopy(input), result), "pulled result");

        byte[] resultClose = closeData(result);
        require(send(session, 4, 0, 0, resultClose, response) == 0, "first closeRead");
        require(send(session, 4, 0, 0, resultClose, response) == 0, "closeRead retry");
        require(allZero(fixture.workspace), "closeRead must wipe result bytes");
        require(handler.calls == 1, "read close must not execute handler");
    }

    private static void testResponseOnlyStream() throws Exception {
        Fixture fixture = new Fixture();
        CountingReverseHandler handler = new CountingReverseHandler();
        Session session = fixture.session((byte) 1, false, true, handler);
        byte[] response = new byte[255];
        byte[] input = new byte[]{1, 2, 3, 4};

        int descriptorLength = send(session, 0, 0, 0, input, response);
        require(descriptorLength == 35, "response-only descriptor length");
        require(handler.calls == 1, "response-only handler must execute once");
        byte[] descriptor = Arrays.copyOf(response, descriptorLength);

        expectStatus(0x6985, () -> send(session, 0, 0, 0, input, response));
        require(handler.calls == 1, "response-only replay must not execute handler");
        int pendingLength = send(session, 2, 0, 0, EMPTY, response);
        require(Arrays.equals(descriptor, Arrays.copyOf(response, pendingLength)),
                "response-only replay must preserve pending descriptor");

        int resultLength = send(session, 3, 0, response[0] & 0xFF, EMPTY, response);
        byte[] result = Arrays.copyOf(response, resultLength);
        require(Arrays.equals(reverseCopy(input), result), "response-only result");
        require(send(session, 4, 0, 0, closeData(result), response) == 0, "response-only close");
        require(allZero(fixture.workspace), "response-only close must wipe workspace");
    }

    private static void testCrossMethodInterleavingPreservesOwner() throws Exception {
        Fixture fixture = new Fixture();
        Session first = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        Session second = fixture.session((byte) 2, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        byte[] firstChunk = new byte[192];
        Arrays.fill(firstChunk, (byte) 7);
        send(first, 0, 0, 2, firstChunk, response);

        expectStatus(0x6985, () -> send(second, 0, 0, 1, new byte[]{9}, response));
        require(fixture.workspace[0] == 7, "foreign method must not wipe owner state");

        byte[] tail = new byte[]{8};
        send(first, 0, 1, 2, tail, response);
        byte[] complete = new byte[193];
        System.arraycopy(firstChunk, 0, complete, 0, firstChunk.length);
        complete[192] = 8;
        require(send(first, 1, 0, 0, closeData(complete), response) == 35,
                "owning method must remain usable");
    }

    // S-06: CLEAR_ON_DESELECT zeroes every injected array. Emulate that here and
    // prove the runtime treats the all-zero image as the empty state: a pending
    // read is refused with 6985, nothing of the old session survives, and a
    // fresh session starts cleanly. No reset marker is involved any more.
    private static void testDeselectClearedTransientStateStartsEmpty() {
        Fixture fixture = new Fixture();
        Session session = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        send(session, 0, 0, 2, new byte[192], response);
        require(!allZero(fixture.scalars), "a live session must have non-zero scalar state");
        fixture.emulateDeselect();

        expectStatus(0x6985, () -> send(session, 2, 0, 0, EMPTY, response));
        require(allZero(fixture.workspace), "deselect image must leave the workspace empty");

        send(session, 0, 0, 1, new byte[]{1}, response);
        require(fixture.workspace[0] == 1, "fresh session must start after a deselect clear");
    }

    // S-06: an explicit abort must leave the injected arrays bit-identical to the
    // CLEAR_ON_DESELECT image (all zero, handler slot empty) — the runtime has no
    // other place to keep state, so this is the whole post-abort footprint.
    private static void testAbortLeavesTransientStateAllZero() {
        Fixture fixture = new Fixture();
        Session session = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        send(session, 0, 0, 2, new byte[192], response);
        require(fixture.handlerSlot[0] != null, "a live session must hold its handler");

        require(send(session, 5, 0, 0, EMPTY, response) == 0, "abort");
        require(allZero(fixture.scalars), "abort must zero the scalar state machine");
        require(fixture.handlerSlot[0] == null, "abort must drop the handler reference");
        require(allZero(fixture.workspace) && allZero(fixture.digest), "abort must wipe buffers");
    }

    // S-06: the state machine has no home outside the injected arrays. Two
    // runtimes sharing the same arrays must observe each other's session — a
    // hidden instance field would make the second runtime see an empty state.
    private static void testStateMachineLivesOnlyInInjectedArrays() throws Exception {
        Fixture fixture = new Fixture();
        StreamDemoBoundedStreamRuntime twin = fixture.twinRuntime();
        CountingReverseHandler handler = new CountingReverseHandler();
        Session session = fixture.session((byte) 1, true, true, handler);
        byte[] response = new byte[255];
        send(session, 0, 0, 2, new byte[192], response);

        Session viaTwin = new Session(twin, (byte) 1, true, true, (short) -1, handler);
        byte[] tail = new byte[]{8};
        send(viaTwin, 0, 1, 2, tail, response);
        byte[] complete = new byte[193];
        complete[192] = 8;
        require(send(viaTwin, 1, 0, 0, closeData(complete), response) == 35,
                "twin runtime over the same arrays must continue the session");
        require(handler.calls == 1, "handler must execute once through the twin");
    }

    private static void testResponsePreflightRunsBeforeHandler() throws Exception {
        Fixture fixture = new Fixture();
        CountingReverseHandler handler = new CountingReverseHandler();
        Session session = fixture.session((byte) 1, true, true, handler);
        byte[] input = new byte[]{1, 2, 3};
        byte[] response = new byte[34];
        send(session, 0, 0, 1, input, response);
        expectStatus(0x6700, () -> send(session, 1, 0, 0, closeData(input), response));
        require(handler.calls == 0, "handler must not run without descriptor capacity");
    }

    private static void testConflictingReplayWipesWorkspace() {
        Fixture fixture = new Fixture();
        Session session = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        send(session, 0, 0, 1, new byte[]{1, 2, 3}, response);
        expectStatus(0x6A80, () -> send(session, 0, 0, 1, new byte[]{1, 2, 4}, response));
        require(allZero(fixture.workspace), "conflicting replay must wipe workspace");
    }

    private static void testDigestMismatchWipesWorkspace() throws Exception {
        Fixture fixture = new Fixture();
        Session session = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        byte[] input = new byte[]{1, 2, 3, 4};
        send(session, 0, 0, 1, input, response);
        byte[] close = closeData(input);
        close[close.length - 1] ^= 1;
        expectStatus(0x6A80, () -> send(session, 1, 0, 0, close, response));
        require(allZero(fixture.workspace), "digest mismatch must wipe workspace");
        require(allZero(fixture.digest), "digest mismatch must wipe digest scratch");
    }

    private static void testHandlerStatusFailureWipesWorkspace() throws Exception {
        Fixture fixture = new Fixture();
        StreamDemoStreamEndpoint.StreamStatusWordException businessFailure =
                new StreamDemoStreamEndpoint.StreamStatusWordException((short) 0x6985);
        StreamDemoStreamEndpoint.Handler handler = (methodId, input, inputOffset, inputLength,
                output, outputOffset, outputCapacity) -> { throw businessFailure; };
        Session session = fixture.session((byte) 1, true, true, handler);
        byte[] response = new byte[255];
        byte[] input = new byte[]{1, 2, 3, 4};
        send(session, 0, 0, 1, input, response);
        expectStatus(0x6985, () -> send(session, 1, 0, 0, closeData(input), response));
        require(allZero(fixture.workspace), "handler failure must wipe workspace");
        require(allZero(fixture.digest), "handler failure must wipe digest scratch");
    }

    private static void testShortResponseCloseIsExactlyOnce() throws Exception {
        Fixture fixture = new Fixture();
        CountingReverseHandler handler = new CountingReverseHandler();
        Session session = fixture.session((byte) 1, true, false, handler);
        byte[] response = new byte[255];
        byte[] input = new byte[]{10, 20, 30, 40};
        send(session, 0, 0, 1, input, response);
        byte[] close = closeData(input);
        int firstLength = send(session, 1, 0, 0, close, response);
        byte[] first = Arrays.copyOf(response, firstLength);
        Arrays.fill(response, (byte) 0);
        int retryLength = send(session, 1, 0, 0, close, response);
        require(Arrays.equals(first, Arrays.copyOf(response, retryLength)), "short close retry result");
        require(handler.calls == 1, "short close retry must not execute handler again");
    }

    private static void testNegativeHandlerLengthFailsClosed() throws Exception {
        Fixture fixture = new Fixture();
        StreamDemoStreamEndpoint.Handler handler = (methodId, input, inputOffset, inputLength,
                output, outputOffset, outputCapacity) -> (short) -1;
        Session session = fixture.session((byte) 1, true, false, handler);
        byte[] response = new byte[255];
        byte[] input = new byte[]{1, 2, 3, 4};
        send(session, 0, 0, 1, input, response);
        expectStatus(0x6700, () -> send(session, 1, 0, 0, closeData(input), response));
        require(allZero(fixture.workspace), "negative handler length must wipe workspace");
        require(allZero(fixture.digest), "negative handler length must wipe digest scratch");
    }

    private static void testExactShortResponseLength() throws Exception {
        for (int produced : new int[]{72, 74}) {
            Fixture fixture = new Fixture();
            StreamDemoStreamEndpoint.Handler handler = (methodId, input, inputOffset, inputLength,
                    output, outputOffset, outputCapacity) -> {
                require((outputCapacity & 0xFFFF) == 73, "exact output capacity");
                return (short) produced;
            };
            Session session = fixture.session((byte) 1, true, false, (short) 73, handler);
            byte[] input = new byte[]{1};
            byte[] response = new byte[255];
            send(session, 0, 0, 1, input, response);
            expectStatus(0x6700,
                    () -> send(session, 1, 0, 0, closeData(input), response));
            require(allZero(fixture.workspace), "wrong exact length must wipe workspace");
        }

        Fixture fixture = new Fixture();
        StreamDemoStreamEndpoint.Handler handler = (methodId, input, inputOffset, inputLength,
                output, outputOffset, outputCapacity) -> {
            require((outputCapacity & 0xFFFF) == 73, "exact output capacity");
            Arrays.fill(output, outputOffset, outputOffset + 73, (byte) 0x5A);
            return (short) 73;
        };
        Session session = fixture.session((byte) 1, true, false, (short) 73, handler);
        byte[] input = new byte[]{1};
        byte[] response = new byte[255];
        send(session, 0, 0, 1, input, response);
        require(send(session, 1, 0, 0, closeData(input), response) == 73,
                "exact short response length");
    }

    private static void testExplicitAbortWipesWorkspace() {
        Fixture fixture = new Fixture();
        Session session = fixture.session((byte) 1, true, true, new CountingReverseHandler());
        byte[] response = new byte[255];
        byte[] firstChunk = new byte[192];
        Arrays.fill(firstChunk, (byte) 7);
        send(session, 0, 0, 2, firstChunk, response);
        require(send(session, 5, 0, 0, EMPTY, response) == 0, "explicit abort response");
        require(allZero(fixture.workspace), "explicit abort must wipe workspace");
        require(allZero(fixture.digest), "explicit abort must wipe digest scratch");
    }

    private static int send(Session session, int operation, int p1, int p2,
                            byte[] request, byte[] response) {
        return session.dispatch((byte) operation, (byte) p1, (byte) p2,
                request, (short) 0, (short) request.length,
                response, (short) 0, (short) response.length) & 0xFFFF;
    }

    private static byte[] closeData(byte[] value) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(value);
        byte[] close = new byte[34];
        close[0] = (byte) ((value.length >>> 8) & 0xFF);
        close[1] = (byte) (value.length & 0xFF);
        System.arraycopy(digest, 0, close, 2, digest.length);
        return close;
    }

    private static byte[] bytes(int count) {
        byte[] result = new byte[count];
        for (int i = 0; i < result.length; i++) result[i] = (byte) i;
        return result;
    }

    private static byte[] reverseCopy(byte[] input) {
        byte[] result = input.clone();
        for (int left = 0, right = result.length - 1; left < right; left++, right--) {
            byte value = result[left];
            result[left] = result[right];
            result[right] = value;
        }
        return result;
    }

    private static boolean allZero(byte[] value) {
        for (byte item : value) if (item != 0) return false;
        return true;
    }

    private static boolean allZero(short[] value) {
        for (short item : value) if (item != 0) return false;
        return true;
    }

    private static void expectStatus(int expected, CheckedRunnable body) {
        try {
            body.run();
            throw new AssertionError("expected status " + Integer.toHexString(expected));
        } catch (StreamDemoStreamEndpoint.StreamStatusWordException failure) {
            require((failure.getStatusWord() & 0xFFFF) == expected, "unexpected status word");
        } catch (Exception failure) {
            throw new RuntimeException(failure);
        }
    }

    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    private interface CheckedRunnable { void run() throws Exception; }

    private static final class Fixture {
        final byte[] workspace = new byte[2048];
        final byte[] digest = new byte[32];
        final short[] scalars = new short[StreamDemoBoundedStreamRuntime.SCALAR_COUNT];
        final Object[] handlerSlot = new Object[StreamDemoBoundedStreamRuntime.HANDLER_SLOT_COUNT];
        final StreamDemoBoundedStreamRuntime runtime = new StreamDemoBoundedStreamRuntime(
                workspace, digest, scalars, handlerSlot,
                (input, inputOffset, inputLength, output, outputOffset) -> {
                    try {
                        MessageDigest md = MessageDigest.getInstance("SHA-256");
                        md.update(input, inputOffset, inputLength);
                        byte[] value = md.digest();
                        System.arraycopy(value, 0, output, outputOffset, value.length);
                    } catch (Exception failure) {
                        throw new RuntimeException(failure);
                    }
                });

        Session session(byte methodId, boolean request, boolean response,
                        StreamDemoStreamEndpoint.Handler handler) {
            return session(methodId, request, response, (short) -1, handler);
        }

        /** What CLEAR_ON_DESELECT does to every injected array. */
        void emulateDeselect() {
            Arrays.fill(workspace, (byte) 0);
            Arrays.fill(digest, (byte) 0);
            Arrays.fill(scalars, (short) 0);
            Arrays.fill(handlerSlot, null);
        }

        /** A second runtime bound to the very same transient arrays. */
        StreamDemoBoundedStreamRuntime twinRuntime() {
            return new StreamDemoBoundedStreamRuntime(workspace, digest, scalars, handlerSlot,
                    (input, inputOffset, inputLength, output, outputOffset) -> {
                        try {
                            MessageDigest md = MessageDigest.getInstance("SHA-256");
                            md.update(input, inputOffset, inputLength);
                            byte[] value = md.digest();
                            System.arraycopy(value, 0, output, outputOffset, value.length);
                        } catch (Exception failure) {
                            throw new RuntimeException(failure);
                        }
                    });
        }

        Session session(byte methodId, boolean request, boolean response,
                        short exactShortResponseLength,
                        StreamDemoStreamEndpoint.Handler handler) {
            return new Session(runtime, methodId, request, response,
                    exactShortResponseLength, handler);
        }
    }

    private static final class Session {
        private final StreamDemoBoundedStreamRuntime runtime;
        private final byte methodId;
        private final boolean request;
        private final boolean response;
        private final short exactShortResponseLength;
        private final StreamDemoStreamEndpoint.Handler handler;

        Session(StreamDemoBoundedStreamRuntime runtime, byte methodId, boolean request,
                boolean response, short exactShortResponseLength,
                StreamDemoStreamEndpoint.Handler handler) {
            this.runtime = runtime;
            this.methodId = methodId;
            this.request = request;
            this.response = response;
            this.exactShortResponseLength = exactShortResponseLength;
            this.handler = handler;
        }

        short dispatch(byte operation, byte p1, byte p2, byte[] input, short inputOffset,
                       short inputLength, byte[] output, short outputOffset, short outputCapacity) {
            return runtime.dispatch(methodId, operation,
                    request, (short) 1792, (short) 192,
                    response, (short) 1792, (short) 192,
                    exactShortResponseLength,
                    handler, p1, p2, input, inputOffset, inputLength,
                    output, outputOffset, outputCapacity);
        }
    }

    private static final class CountingReverseHandler implements StreamDemoStreamEndpoint.Handler {
        int calls;

        @Override
        public short execute(byte methodId, byte[] input, short inputOffset, short inputLength,
                             byte[] output, short outputOffset, short outputCapacity) {
            calls++;
            for (short left = 0, right = (short) (inputLength - 1); left <= right; left++, right--) {
                byte value = input[(short) (inputOffset + left)];
                output[(short) (outputOffset + left)] = input[(short) (inputOffset + right)];
                output[(short) (outputOffset + right)] = value;
            }
            return inputLength;
        }
    }
}
