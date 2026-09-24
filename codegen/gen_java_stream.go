package codegen

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const javaStreamEndpointTemplate = `package {{.PackageName}};

import javacard.framework.JCSystem;

/**
 * Generated boundary for the single applet-level bounded stream session.
 * One implementation owns every streamed method in one selected applet.
 */
public interface {{.StreamEndpointName}} {
    byte OP_WRITE_OR_INVOKE       = (byte) 0x00;
    byte OP_CLOSE_WRITE           = (byte) 0x01;
    byte OP_GET_PENDING_READ_INFO = (byte) 0x02;
    byte OP_READ_CHUNK            = (byte) 0x03;
    byte OP_CLOSE_READ            = (byte) 0x04;
    byte OP_ABORT                 = (byte) 0x05;

    byte ABORT_EXPLICIT     = (byte) 0x01;
    byte ABORT_DESELECT     = (byte) 0x02;
    byte ABORT_RESET        = (byte) 0x03;
    byte ABORT_HOST_FAILURE = (byte) 0x04;

    /**
     * Return the response length. Protocol failures use the runtime's one
     * reusable StreamStatusWordException; generated skeleton handlers use
     * failStream(statusWord). No command-path allocation is permitted.
     */
    short dispatch(
            byte methodId,
            byte operation,
            boolean hasRequestStream,
            short requestMaxLength,
            short requestChunkSize,
            boolean hasResponseStream,
            short responseMaxLength,
            short responseChunkSize,
            short exactShortResponseLength,
            Handler handler,
            byte p1,
            byte p2,
            byte[] requestBuffer,
            short requestOffset,
            short requestLength,
            byte[] responseBuffer,
            short responseOffset,
            short responseCapacity);

    void abort(byte reason);

    interface Handler {
        /**
         * Execute one generated method. Input and output may share storage.
         * Return 0..outputCapacity. For a fixed short response, outputCapacity
         * is the exact declared width and the returned length must equal it.
         * Use the generated skeleton's failStream(statusWord) helper for a
         * business status word.
         */
        short execute(
                byte methodId,
                byte[] input,
                short inputOffset,
                short inputLength,
                byte[] output,
                short outputOffset,
                short outputCapacity);
    }

    interface Sha256 {
        void digest(
                byte[] input,
                short inputOffset,
                short inputLength,
                byte[] output,
                short outputOffset);
    }

    /** One exception object is allocated once; its status is transient RAM. */
    final class StreamStatusWordException extends RuntimeException {
        private final short[] status;

        public StreamStatusWordException(short statusWord) {
            super();
            this.status = JCSystem.makeTransientShortArray(
                    (short) 1, JCSystem.CLEAR_ON_RESET);
            this.status[0] = statusWord;
        }

        public void setStatusWord(short statusWord) {
            // This array is transient RAM; changing status never writes EEPROM.
            this.status[0] = statusWord;
        }

        public short getStatusWord() {
            return status[0];
        }
    }
}
`

const javaStreamRuntimeTemplate = `package {{.PackageName}};

/**
 * Generated single-owner, bounded, half-duplex stream state machine.
 *
 * The skeleton constructs exactly one instance and injects CLEAR_ON_DESELECT
 * arrays for everything mutable: the byte workspace, the digest scratch, the
 * short[] scalar state machine and the one-slot handler reference. This class
 * declares no mutable field of its own, so a WRITE chunk, a result or an abort
 * never touches persistent memory, and deselect or reset returns the session to
 * the all-zero empty state without a reset marker (security audit S-06).
 * Command processing reuses one exception object and never allocates an object
 * or array.
 */
public final class {{.StreamRuntimeName}} implements {{.StreamEndpointName}} {
    private static final short SW_WRONG_LENGTH = (short) 0x6700;
    private static final short SW_INVALID_DATA = (short) 0x6A80;
    private static final short SW_WRONG_STATE  = (short) 0x6985;
    private static final short SW_NO_MEMORY    = (short) 0x6A84;

    private static final byte STATE_EMPTY        = (byte) 0x00;
    private static final byte STATE_WRITING      = (byte) 0x01;
    private static final byte STATE_READ_PENDING = (byte) 0x02;
    private static final byte STATE_READ_CLOSED  = (byte) 0x03;
    private static final byte STATE_SHORT_CLOSED = (byte) 0x04;

    private static final short DIGEST_LENGTH = (short) 32;
    private static final short CLOSE_LENGTH = (short) 34;
    private static final short DESCRIPTOR_LENGTH = (short) 35;
    private static final short SHORT_RESPONSE_CAPACITY = (short) 255;

    // Layout of the CLEAR_ON_DESELECT short[] scalar array. The all-zero
    // array is the empty state: STATE_EMPTY is 0, booleans are 0/1 and every
    // length or index is only read after begin() has written it.
    private static final short IDX_STATE                      = (short) 0;
    private static final short IDX_ACTIVE_METHOD              = (short) 1;
    private static final short IDX_HAS_REQUEST_STREAM         = (short) 2;
    private static final short IDX_HAS_RESPONSE_STREAM        = (short) 3;
    private static final short IDX_REQUEST_MAX_LENGTH         = (short) 4;
    private static final short IDX_REQUEST_CHUNK_SIZE         = (short) 5;
    private static final short IDX_RESPONSE_MAX_LENGTH        = (short) 6;
    private static final short IDX_RESPONSE_CHUNK_SIZE        = (short) 7;
    private static final short IDX_EXACT_SHORT_RESPONSE_LENGTH = (short) 8;
    private static final short IDX_INPUT_LENGTH               = (short) 9;
    private static final short IDX_LAST_CHUNK_OFFSET          = (short) 10;
    private static final short IDX_LAST_CHUNK_LENGTH          = (short) 11;
    private static final short IDX_SHORT_RESULT_LENGTH        = (short) 12;
    private static final short IDX_RESULT_LENGTH              = (short) 13;
    private static final short IDX_PACKET_COUNT               = (short) 14;
    private static final short IDX_NEXT_PACKET_INDEX          = (short) 15;
    private static final short IDX_RESULT_PACKET_COUNT        = (short) 16;
    /** Required length of the injected scalar array. */
    public static final short SCALAR_COUNT = (short) 17;
    /** Required length of the injected handler reference array. */
    public static final short HANDLER_SLOT_COUNT = (short) 1;
    private static final short IDX_HANDLER = (short) 0;

    private final byte[] workspace;
    private final byte[] digestScratch;
    private final short[] scalars;
    private final Object[] handlerSlot;
    private final Sha256 sha256;
    private final StreamStatusWordException failure;

    public {{.StreamRuntimeName}}(
            byte[] workspace,
            byte[] digestScratch,
            short[] scalars,
            Object[] handlerSlot,
            Sha256 sha256) {
        this.workspace = workspace;
        this.digestScratch = digestScratch;
        this.scalars = scalars;
        this.handlerSlot = handlerSlot;
        this.sha256 = sha256;
        this.failure = new StreamStatusWordException(SW_WRONG_STATE);

        if (workspace == null || workspace.length == 0 ||
                digestScratch == null || digestScratch.length < DIGEST_LENGTH ||
                scalars == null || scalars.length < SCALAR_COUNT ||
                handlerSlot == null || handlerSlot.length < HANDLER_SLOT_COUNT ||
                sha256 == null) {
            failure.setStatusWord(SW_NO_MEMORY);
            throw failure;
        }
        clearAll();
    }

    @Override
    public short dispatch(
            byte methodId,
            byte operation,
            boolean requestEnabled,
            short requestLimit,
            short requestChunk,
            boolean responseEnabled,
            short responseLimit,
            short responseChunk,
            short shortResponseLength,
            Handler handler,
            byte p1,
            byte p2,
            byte[] requestBuffer,
            short requestOffset,
            short requestLength,
            byte[] responseBuffer,
            short responseOffset,
            short responseCapacity) {
        try {
            validateRange(requestBuffer, requestOffset, requestLength);
            validateRange(responseBuffer, responseOffset, responseCapacity);

            if (operation == OP_ABORT) {
                requireEmptyCommand(p1, p2, requestLength);
                abort(ABORT_EXPLICIT);
                return (short) 0;
            }

            if (operation == OP_WRITE_OR_INVOKE) {
                if (scalars[IDX_STATE] == STATE_READ_CLOSED ||
                        scalars[IDX_STATE] == STATE_SHORT_CLOSED) {
                    clearAll();
                }
                if (scalars[IDX_STATE] == STATE_EMPTY) {
                    begin(methodId, requestEnabled, requestLimit, requestChunk,
                            responseEnabled, responseLimit, responseChunk,
                            shortResponseLength, handler);
                } else if (scalars[IDX_ACTIVE_METHOD] != methodId) {
                    rejectWithoutClearing(SW_WRONG_STATE);
                }
            } else if (scalars[IDX_STATE] == STATE_EMPTY ||
                    scalars[IDX_ACTIVE_METHOD] != methodId) {
                rejectWithoutClearing(SW_WRONG_STATE);
            }

            switch (operation) {
                case OP_WRITE_OR_INVOKE:
                    return writeOrInvoke(p1, p2, requestBuffer, requestOffset, requestLength,
                            responseBuffer, responseOffset, responseCapacity);
                case OP_CLOSE_WRITE:
                    return closeWrite(p1, p2, requestBuffer, requestOffset, requestLength,
                            responseBuffer, responseOffset, responseCapacity);
                case OP_GET_PENDING_READ_INFO:
                    requireEmptyCommand(p1, p2, requestLength);
                    return pendingInfo(responseBuffer, responseOffset, responseCapacity);
                case OP_READ_CHUNK:
                    requireEmptyData(requestLength);
                    return readChunk(p1, p2, responseBuffer, responseOffset, responseCapacity);
                case OP_CLOSE_READ:
                    return closeRead(p1, p2, requestBuffer, requestOffset, requestLength);
                default:
                    fail(SW_WRONG_STATE);
                    return (short) 0;
            }
        } catch (StreamStatusWordException e) {
            if (e != failure) {
                clearAll();
            }
            throw e;
        } catch (RuntimeException e) {
            clearAll();
            throw e;
        }
    }

    @Override
    public void abort(byte reason) {
        clearAll();
    }

    private boolean hasRequestStream() {
        return scalars[IDX_HAS_REQUEST_STREAM] != 0;
    }

    private boolean hasResponseStream() {
        return scalars[IDX_HAS_RESPONSE_STREAM] != 0;
    }

    private void begin(
            byte methodId,
            boolean requestEnabled,
            short requestLimit,
            short requestChunk,
            boolean responseEnabled,
            short responseLimit,
            short responseChunk,
            short shortResponseLength,
            Handler handler) {
        short required = requestEnabled ? requestLimit : (short) 0;
        if (responseEnabled && responseLimit > required) {
            required = responseLimit;
        }
        short shortCapacity = shortResponseLength >= 0 ?
                shortResponseLength : SHORT_RESPONSE_CAPACITY;
        if (!responseEnabled && required < shortCapacity) {
            required = shortCapacity;
        }
        if ((!requestEnabled && !responseEnabled) || handler == null ||
                (requestEnabled && (requestLimit <= 0 || requestChunk <= 0)) ||
                (responseEnabled && (responseLimit <= 0 || responseChunk <= 0)) ||
                (responseEnabled && shortResponseLength != (short) -1) ||
                (!responseEnabled &&
                        (shortResponseLength < (short) -1 ||
                                shortResponseLength > SHORT_RESPONSE_CAPACITY)) ||
                required > workspace.length) {
            fail(SW_NO_MEMORY);
        }

        scalars[IDX_ACTIVE_METHOD] = methodId;
        scalars[IDX_HAS_REQUEST_STREAM] = requestEnabled ? (short) 1 : (short) 0;
        scalars[IDX_HAS_RESPONSE_STREAM] = responseEnabled ? (short) 1 : (short) 0;
        scalars[IDX_REQUEST_MAX_LENGTH] = requestLimit;
        scalars[IDX_REQUEST_CHUNK_SIZE] = requestChunk;
        scalars[IDX_RESPONSE_MAX_LENGTH] = responseLimit;
        scalars[IDX_RESPONSE_CHUNK_SIZE] = responseChunk;
        scalars[IDX_EXACT_SHORT_RESPONSE_LENGTH] = shortResponseLength;
        handlerSlot[IDX_HANDLER] = handler;
    }

    private short writeOrInvoke(
            byte p1,
            byte p2,
            byte[] request,
            short requestOffset,
            short requestLength,
            byte[] response,
            short responseOffset,
            short responseCapacity) {
        if (!hasRequestStream()) {
            if (scalars[IDX_STATE] != STATE_EMPTY) {
                rejectWithoutClearing(SW_WRONG_STATE);
            }
            requireZeroParameters(p1, p2);
            preflightHandlerResponse(responseCapacity);
            return executeOnce(request, requestOffset, requestLength,
                    response, responseOffset, responseCapacity, false);
        }

        int index = p1 & 0xFF;
        int count = p2 & 0xFF;
        int length = requestLength & 0xFFFF;
        int chunkSize = scalars[IDX_REQUEST_CHUNK_SIZE] & 0xFFFF;
        int maxPackets = ((scalars[IDX_REQUEST_MAX_LENGTH] & 0xFFFF) + chunkSize - 1) / chunkSize;
        if (count == 0 || count > maxPackets || index >= count || length == 0 ||
                length > chunkSize || (index < count - 1 && length != chunkSize)) {
            fail(SW_WRONG_LENGTH);
        }

        if (scalars[IDX_STATE] == STATE_WRITING &&
                index == ((scalars[IDX_NEXT_PACKET_INDEX] & 0xFF) - 1)) {
            if (count != (scalars[IDX_PACKET_COUNT] & 0xFF) ||
                    length != (scalars[IDX_LAST_CHUNK_LENGTH] & 0xFFFF) ||
                    !equalsRange(workspace, scalars[IDX_LAST_CHUNK_OFFSET],
                            request, requestOffset, requestLength)) {
                fail(SW_INVALID_DATA);
            }
            return (short) 0;
        }

        if (index == 0 && scalars[IDX_STATE] == STATE_EMPTY) {
            scalars[IDX_STATE] = STATE_WRITING;
            scalars[IDX_PACKET_COUNT] = p2;
            scalars[IDX_NEXT_PACKET_INDEX] = (short) 0;
        }
        if (scalars[IDX_STATE] != STATE_WRITING ||
                count != (scalars[IDX_PACKET_COUNT] & 0xFF) ||
                index != (scalars[IDX_NEXT_PACKET_INDEX] & 0xFF)) {
            fail(SW_INVALID_DATA);
        }
        if ((scalars[IDX_INPUT_LENGTH] & 0xFFFF) + length >
                (scalars[IDX_REQUEST_MAX_LENGTH] & 0xFFFF)) {
            fail(SW_WRONG_LENGTH);
        }

        scalars[IDX_LAST_CHUNK_OFFSET] = scalars[IDX_INPUT_LENGTH];
        scalars[IDX_LAST_CHUNK_LENGTH] = requestLength;
        copy(request, requestOffset, workspace, scalars[IDX_INPUT_LENGTH], requestLength);
        scalars[IDX_INPUT_LENGTH] = (short) ((scalars[IDX_INPUT_LENGTH] & 0xFFFF) + length);
        scalars[IDX_NEXT_PACKET_INDEX] = (short) (index + 1);
        return (short) 0;
    }

    private short closeWrite(
            byte p1,
            byte p2,
            byte[] request,
            short requestOffset,
            short requestLength,
            byte[] response,
            short responseOffset,
            short responseCapacity) {
        requireZeroParameters(p1, p2);
        requireCloseData(requestLength);

        int declaredLength = readU16(request, requestOffset);
        short digestOffset = (short) (requestOffset + 2);
        if (scalars[IDX_STATE] == STATE_SHORT_CLOSED) {
            if (declaredLength != (scalars[IDX_INPUT_LENGTH] & 0xFFFF) ||
                    !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
                fail(SW_INVALID_DATA);
            }
            ensureResponseCapacity(responseCapacity, scalars[IDX_SHORT_RESULT_LENGTH]);
            copy(workspace, (short) 0, response, responseOffset, scalars[IDX_SHORT_RESULT_LENGTH]);
            return scalars[IDX_SHORT_RESULT_LENGTH];
        }
        if (!hasRequestStream() || scalars[IDX_STATE] != STATE_WRITING ||
                (scalars[IDX_NEXT_PACKET_INDEX] & 0xFF) != (scalars[IDX_PACKET_COUNT] & 0xFF)) {
            fail(SW_WRONG_STATE);
        }
        if (declaredLength != (scalars[IDX_INPUT_LENGTH] & 0xFFFF)) {
            fail(SW_WRONG_LENGTH);
        }

        preflightHandlerResponse(responseCapacity);
        sha256.digest(workspace, (short) 0, scalars[IDX_INPUT_LENGTH], digestScratch, (short) 0);
        if (!equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
            fail(SW_INVALID_DATA);
        }
        return executeOnce(workspace, (short) 0, scalars[IDX_INPUT_LENGTH],
                response, responseOffset, responseCapacity, true);
    }

    private short shortOutputCapacity() {
        return scalars[IDX_EXACT_SHORT_RESPONSE_LENGTH] >= 0 ?
                scalars[IDX_EXACT_SHORT_RESPONSE_LENGTH] : SHORT_RESPONSE_CAPACITY;
    }

    private void preflightHandlerResponse(short responseCapacity) {
        ensureResponseCapacity(responseCapacity,
                hasResponseStream() ? DESCRIPTOR_LENGTH : shortOutputCapacity());
    }

    private short executeOnce(
            byte[] input,
            short inputOffset,
            short inputSize,
            byte[] response,
            short responseOffset,
            short responseCapacity,
            boolean preserveInputCloseReceipt) {
        short outputCapacity = hasResponseStream() ?
                scalars[IDX_RESPONSE_MAX_LENGTH] : shortOutputCapacity();
        short produced = ((Handler) handlerSlot[IDX_HANDLER]).execute(
                (byte) scalars[IDX_ACTIVE_METHOD], input, inputOffset, inputSize,
                workspace, (short) 0, outputCapacity);
        if (produced < 0 || produced > outputCapacity ||
                (hasResponseStream() && produced == 0) ||
                (!hasResponseStream() && scalars[IDX_EXACT_SHORT_RESPONSE_LENGTH] >= 0 &&
                        produced != scalars[IDX_EXACT_SHORT_RESPONSE_LENGTH])) {
            fail(SW_WRONG_LENGTH);
        }

        scalars[IDX_RESULT_LENGTH] = produced;
        if (hasResponseStream()) {
            sha256.digest(workspace, (short) 0, produced, digestScratch, (short) 0);
            int chunkSize = scalars[IDX_RESPONSE_CHUNK_SIZE] & 0xFFFF;
            int chunks = ((produced & 0xFFFF) + chunkSize - 1) / chunkSize;
            scalars[IDX_RESULT_PACKET_COUNT] = (short) chunks;
            scalars[IDX_STATE] = STATE_READ_PENDING;
            return writeDescriptor(response, responseOffset, responseCapacity);
        }

        copy(workspace, (short) 0, response, responseOffset, produced);
        scalars[IDX_SHORT_RESULT_LENGTH] = produced;
        if (preserveInputCloseReceipt) {
            scalars[IDX_STATE] = STATE_SHORT_CLOSED;
        } else {
            clearAll();
        }
        return produced;
    }

    private short pendingInfo(byte[] response, short responseOffset, short responseCapacity) {
        if (!hasResponseStream() || scalars[IDX_STATE] != STATE_READ_PENDING) {
            fail(SW_WRONG_STATE);
        }
        return writeDescriptor(response, responseOffset, responseCapacity);
    }

    private short readChunk(
            byte p1,
            byte p2,
            byte[] response,
            short responseOffset,
            short responseCapacity) {
        if (!hasResponseStream() || scalars[IDX_STATE] != STATE_READ_PENDING) {
            fail(SW_WRONG_STATE);
        }
        int index = p1 & 0xFF;
        int count = p2 & 0xFF;
        if (count != (scalars[IDX_RESULT_PACKET_COUNT] & 0xFF) || index >= count) {
            fail(SW_INVALID_DATA);
        }
        int chunkSize = scalars[IDX_RESPONSE_CHUNK_SIZE] & 0xFFFF;
        int offset = index * chunkSize;
        int remaining = (scalars[IDX_RESULT_LENGTH] & 0xFFFF) - offset;
        int length = remaining < chunkSize ? remaining : chunkSize;
        ensureResponseCapacity(responseCapacity, (short) length);
        copy(workspace, (short) offset, response, responseOffset, (short) length);
        return (short) length;
    }

    private short closeRead(
            byte p1,
            byte p2,
            byte[] request,
            short requestOffset,
            short requestLength) {
        requireZeroParameters(p1, p2);
        requireCloseData(requestLength);
        int declaredLength = readU16(request, requestOffset);
        short digestOffset = (short) (requestOffset + 2);

        if (scalars[IDX_STATE] != STATE_READ_PENDING && scalars[IDX_STATE] != STATE_READ_CLOSED) {
            fail(SW_WRONG_STATE);
        }
        if (declaredLength != (scalars[IDX_RESULT_LENGTH] & 0xFFFF) ||
                !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
            fail(SW_INVALID_DATA);
        }
        if (scalars[IDX_STATE] == STATE_READ_PENDING) {
            wipe(workspace);
            scalars[IDX_STATE] = STATE_READ_CLOSED;
        }
        return (short) 0;
    }

    private short writeDescriptor(byte[] response, short offset, short capacity) {
        ensureResponseCapacity(capacity, DESCRIPTOR_LENGTH);
        response[offset] = (byte) scalars[IDX_RESULT_PACKET_COUNT];
        response[(short) (offset + 1)] = (byte) ((scalars[IDX_RESULT_LENGTH] >>> 8) & 0xFF);
        response[(short) (offset + 2)] = (byte) (scalars[IDX_RESULT_LENGTH] & 0xFF);
        copy(digestScratch, (short) 0, response, (short) (offset + 3), DIGEST_LENGTH);
        return DESCRIPTOR_LENGTH;
    }

    private void requireCloseData(short length) {
        if (length != CLOSE_LENGTH) {
            fail(SW_WRONG_LENGTH);
        }
    }

    private void requireEmptyCommand(byte p1, byte p2, short length) {
        requireZeroParameters(p1, p2);
        requireEmptyData(length);
    }

    private void requireZeroParameters(byte p1, byte p2) {
        if (p1 != 0 || p2 != 0) {
            fail(SW_INVALID_DATA);
        }
    }

    private void requireEmptyData(short length) {
        if (length != 0) {
            fail(SW_WRONG_LENGTH);
        }
    }

    private void ensureResponseCapacity(short capacity, short required) {
        if (required < 0 || capacity < required) {
            fail(SW_WRONG_LENGTH);
        }
    }

    private int readU16(byte[] input, short offset) {
        return ((input[offset] & 0xFF) << 8) |
                (input[(short) (offset + 1)] & 0xFF);
    }

    private void validateRange(byte[] buffer, short offset, short length) {
        if (buffer == null || offset < 0 || length < 0 ||
                (offset & 0xFFFF) + (length & 0xFFFF) > buffer.length) {
            fail(SW_WRONG_LENGTH);
        }
    }

    private boolean equalsRange(
            byte[] left,
            short leftOffset,
            byte[] right,
            short rightOffset,
            short length) {
        for (short i = 0; i < length; i++) {
            if (left[(short) (leftOffset + i)] != right[(short) (rightOffset + i)]) {
                return false;
            }
        }
        return true;
    }

    private void copy(
            byte[] source,
            short sourceOffset,
            byte[] target,
            short targetOffset,
            short length) {
        for (short i = 0; i < length; i++) {
            target[(short) (targetOffset + i)] = source[(short) (sourceOffset + i)];
        }
    }

    private void wipe(byte[] value) {
        if (value == null) {
            return;
        }
        for (short i = 0; i < value.length; i++) {
            value[i] = (byte) 0;
        }
    }

    /**
     * Return to the empty state. Writes only the injected transient arrays,
     * so the result is bit-identical to what CLEAR_ON_DESELECT produces.
     */
    private void clearAll() {
        wipe(workspace);
        wipe(digestScratch);
        for (short i = 0; i < SCALAR_COUNT; i++) {
            scalars[i] = (short) 0;
        }
        handlerSlot[IDX_HANDLER] = null;
    }

    private void fail(short statusWord) {
        clearAll();
        failure.setStatusWord(statusWord);
        throw failure;
    }

    private void rejectWithoutClearing(short statusWord) {
        failure.setStatusWord(statusWord);
        throw failure;
    }
}
`

const javaStreamAPDUAdapterTemplate = `package {{.PackageName}};

import javacard.framework.APDU;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;
import javacard.framework.JCSystem;

/**
 * Generated no-allocation short-APDU adapter for streamed instructions.
 * The owning Applet calls processIfStream() from process() and deselect() from
 * its deselect callback. Ordinary non-stream instructions remain on the legacy
 * dispatch path.
 */
public final class {{.StreamAPDUAdapterName}} {
    private static final short IO_CAPACITY = (short) 255;

    private final {{.ClassName}} logic;
    private final byte[] ioScratch;

    public {{.StreamAPDUAdapterName}}({{.ClassName}} logic) {
        this.logic = logic;
        this.ioScratch = JCSystem.makeTransientByteArray(
                IO_CAPACITY, JCSystem.CLEAR_ON_DESELECT);
    }

    public boolean processIfStream(APDU apdu) {
        byte[] apduBuffer = apdu.getBuffer();
        byte ins = apduBuffer[ISO7816.OFFSET_INS];
        if (!logic.isStreamInstruction(ins)) {
            return false;
        }
        if (apduBuffer[ISO7816.OFFSET_CLA] != {{.ClassName}}.{{.CLAConstName}}) {
            ISOException.throwIt(ISO7816.SW_CLA_NOT_SUPPORTED);
        }

        try {
            short firstReceived = apdu.setIncomingAndReceive();
            short incomingLength = apdu.getIncomingLength();
            if (incomingLength < 0 || incomingLength > IO_CAPACITY) {
                ISOException.throwIt(ISO7816.SW_WRONG_LENGTH);
            }

            short copied = 0;
            short received = firstReceived;
            while (copied < incomingLength) {
                if (received <= 0 || (short) (copied + received) > incomingLength) {
                    ISOException.throwIt(ISO7816.SW_WRONG_LENGTH);
                }
                copy(apduBuffer, ISO7816.OFFSET_CDATA, ioScratch, copied, received);
                copied = (short) (copied + received);
                if (copied < incomingLength) {
                    received = apdu.receiveBytes(ISO7816.OFFSET_CDATA);
                }
            }

            short outcome = logic.dispatchStreamTo(
                    ins,
                    apduBuffer[ISO7816.OFFSET_P1],
                    apduBuffer[ISO7816.OFFSET_P2],
                    ioScratch,
                    (short) 0,
                    incomingLength,
                    ioScratch,
                    (short) 0,
                    IO_CAPACITY);
            if (outcome > 0) {
                apdu.setOutgoing();
                apdu.setOutgoingLength(outcome);
                apdu.sendBytesLong(ioScratch, (short) 0, outcome);
            }
            return true;
        } finally {
            wipe(ioScratch);
        }
    }

    public void deselect() {
        logic.abortStreams({{.StreamEndpointName}}.ABORT_DESELECT);
        wipe(ioScratch);
    }

    private static void copy(
            byte[] source,
            short sourceOffset,
            byte[] target,
            short targetOffset,
            short length) {
        for (short i = 0; i < length; i++) {
            target[(short) (targetOffset + i)] = source[(short) (sourceOffset + i)];
        }
    }

    private static void wipe(byte[] value) {
        for (short i = 0; i < value.length; i++) {
            value[i] = (byte) 0;
        }
    }
}
`

func generateJavaStreamSupport(
	data *javaTemplateData,
	methods []javaMethodRender,
) ([]byte, []byte, []byte, error) {
	if !hasJavaStreamMethods(methods) {
		return nil, nil, nil, nil
	}

	endpointSource, err := executeJavaStreamTemplate("java_stream_endpoint", javaStreamEndpointTemplate, data)
	if err != nil {
		return nil, nil, nil, err
	}
	runtimeSource, err := executeJavaStreamTemplate("java_stream_runtime", javaStreamRuntimeTemplate, data)
	if err != nil {
		return nil, nil, nil, err
	}
	adapterSource, err := executeJavaStreamTemplate("java_stream_apdu_adapter", javaStreamAPDUAdapterTemplate, data)
	if err != nil {
		return nil, nil, nil, err
	}
	return endpointSource, runtimeSource, adapterSource, nil
}

func executeJavaStreamTemplate(name, source string, data *javaTemplateData) ([]byte, error) {
	tmpl, err := template.New(name).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse %s template: %w", name, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render %s template: %w", name, err)
	}
	return out.Bytes(), nil
}

func hasJavaStreamMethods(methods []javaMethodRender) bool {
	for _, method := range methods {
		if method.IsStream {
			return true
		}
	}
	return false
}

func augmentJavaSkeletonForStreams(source string, data *javaTemplateData, methods []javaMethodRender) (string, error) {
	packageNeedle := "package " + data.PackageName + ";\n"
	imports := packageNeedle + `

import javacard.framework.ISOException;
import javacard.security.MessageDigest;
`
	if !strings.Contains(source, packageNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: package insertion point not found")
	}
	source = strings.Replace(source, packageNeedle, imports, 1)

	classNeedle := "public abstract class " + data.ClassName + " {"
	classReplacement := "public abstract class " + data.ClassName + " implements " +
		data.StreamEndpointName + ".Handler, " + data.StreamEndpointName + ".Sha256 {"
	if !strings.Contains(source, classNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: class insertion point not found")
	}
	source = strings.Replace(source, classNeedle, classReplacement, 1)

	transportNeedle := "    protected final " + data.TransportInterfaceName + " transport;\n"
	fields := buildJavaStreamOwnedFields(data, methods) + "\n" + transportNeedle
	if !strings.Contains(source, transportNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: field insertion point not found")
	}
	source = strings.Replace(source, transportNeedle, fields, 1)

	constructorNeedle := fmt.Sprintf(
		"    protected %s(%s transport) {\n        this.transport = transport;\n        this.empty = new byte[0];\n        this.sharedFailure = new StatusWordException(SW_INS_NOT_SUPPORTED);\n    }",
		data.ClassName,
		data.TransportInterfaceName,
	)
	constructorReplacement := buildJavaStreamConstructor(data)
	if !strings.Contains(source, constructorNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: constructor insertion point not found")
	}
	source = strings.Replace(source, constructorNeedle, constructorReplacement, 1)

	transmitNeedle := "    protected final byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {"
	dispatchSupport := buildJavaStreamDispatchSupport(data, methods) + "\n" + transmitNeedle
	if !strings.Contains(source, transmitNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: transmit insertion point not found")
	}
	source = strings.Replace(source, transmitNeedle, dispatchSupport, 1)

	abstractNeedle := "    // No-message, numeric-only exception:"
	abstractSupport := buildJavaStreamAbstractSupport(data, methods) + "\n" + abstractNeedle
	if !strings.Contains(source, abstractNeedle) {
		return "", fmt.Errorf("augment Java stream skeleton: abstract insertion point not found")
	}
	source = strings.Replace(source, abstractNeedle, abstractSupport, 1)
	return source, nil
}

func buildJavaStreamOwnedFields(data *javaTemplateData, methods []javaMethodRender) string {
	workspaceLength := javaStreamWorkspaceLength(methods)
	return fmt.Sprintf(`    public static final short STREAM_WORKSPACE_LENGTH = (short) %d;
    public static final short STREAM_DIGEST_LENGTH = (short) 32;
    public static final short STREAM_SCALAR_COUNT = %s.SCALAR_COUNT;
    public static final short STREAM_HANDLER_SLOT_COUNT = %s.HANDLER_SLOT_COUNT;

    private final %s streamSession;
    private final MessageDigest streamSha256;
    private final %s.StreamStatusWordException streamHandlerFailure;`, workspaceLength, data.StreamRuntimeName, data.StreamRuntimeName, data.StreamRuntimeName, data.StreamEndpointName)
}

func buildJavaStreamConstructor(data *javaTemplateData) string {
	return fmt.Sprintf(`    protected %s(%s transport) {
        this.transport = transport;
        this.empty = new byte[0];
        this.sharedFailure = new StatusWordException(SW_INS_NOT_SUPPORTED);
        this.streamSha256 = MessageDigest.getInstance(MessageDigest.ALG_SHA_256, false);
        this.streamHandlerFailure = new %s.StreamStatusWordException((short) 0x6985);
        // Every mutable word of the stream session lives in CLEAR_ON_DESELECT
        // transient memory: workspace, digest scratch, the scalar state machine
        // and the handler reference. Nothing persistent is written per command.
        this.streamSession = new %s(
                JCSystem.makeTransientByteArray(
                        STREAM_WORKSPACE_LENGTH, JCSystem.CLEAR_ON_DESELECT),
                JCSystem.makeTransientByteArray(
                        STREAM_DIGEST_LENGTH, JCSystem.CLEAR_ON_DESELECT),
                JCSystem.makeTransientShortArray(
                        STREAM_SCALAR_COUNT, JCSystem.CLEAR_ON_DESELECT),
                JCSystem.makeTransientObjectArray(
                        STREAM_HANDLER_SLOT_COUNT, JCSystem.CLEAR_ON_DESELECT),
                this);
    }`, data.ClassName, data.TransportInterfaceName, data.StreamEndpointName, data.StreamRuntimeName)
}

func buildJavaStreamDispatchSupport(data *javaTemplateData, methods []javaMethodRender) string {
	type streamRow struct {
		ins                byte
		directions         byte
		requestMax         int
		requestChunk       int
		responseMax        int
		responseChunk      int
		exactShortResponse int
	}

	rows := make([]streamRow, 0, len(methods))
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		requestEnabled, requestMax, requestChunk := javaStreamFieldConfig(method.RequestStream)
		responseEnabled, responseMax, responseChunk := javaStreamFieldConfig(method.ResponseStream)
		directions := byte(0)
		if requestEnabled {
			directions |= 0x02
		}
		if responseEnabled {
			directions |= 0x01
		}
		rows = append(rows, streamRow{
			ins:                method.INS,
			directions:         directions,
			requestMax:         requestMax,
			requestChunk:       requestChunk,
			responseMax:        responseMax,
			responseChunk:      responseChunk,
			exactShortResponse: method.ExactShortResponseLength,
		})
	}

	var b strings.Builder
	b.WriteString(`    // One row per streamed method, in method-id order. A streamed method owns six
    // consecutive instructions, base + 0..5, in the operation order the endpoint
    // declares (OP_WRITE_OR_INVOKE first, OP_ABORT last), so the instruction alone
    // decodes to a method id and an operation. Holes between families stay holes.
    private static final byte[] STREAM_INS_BASE = {
`)
	writeJavaByteRows(&b, len(rows), func(i int) string {
		return fmt.Sprintf("(byte) 0x%02X", rows[i].ins)
	})
	b.WriteString(`
    // Bit 1: the request carries a stream. Bit 0: the response carries a stream.
    private static final byte[] STREAM_DIRECTIONS = {
`)
	writeJavaByteRows(&b, len(rows), func(i int) string {
		return fmt.Sprintf("(byte) %d", rows[i].directions)
	})
	b.WriteString(`
    // Five columns per row: request maximum, request chunk, response maximum,
    // response chunk, exact short-response length.
    private static final short[] STREAM_LIMITS = {
`)
	for i, row := range rows {
		terminator := ","
		if i == len(rows)-1 {
			terminator = " };"
		}
		fmt.Fprintf(&b, "        (short) %d, (short) %d, (short) %d, (short) %d, (short) %d%s\n",
			row.requestMax, row.requestChunk, row.responseMax, row.responseChunk,
			row.exactShortResponse, terminator)
	}
	b.WriteString(`
    private static final short STREAM_LIMIT_COLUMNS = (short) 5;
    private static final short STREAM_OPERATION_COUNT = (short) 6;

    /**
     * No-allocation dispatch used by the generated Java Card APDU adapter.
     * Returns a response length. Protocol failures are translated to
     * ISOException without allocating on the command path.
     */
    public final short dispatchStreamTo(
            byte ins,
            byte p1,
            byte p2,
            byte[] requestBuffer,
            short requestOffset,
            short requestLength,
            byte[] responseBuffer,
            short responseOffset,
            short responseCapacity) {
        short row = streamRow(ins);
        if (row < (short) 0) {
            ISOException.throwIt(SW_INS_NOT_SUPPORTED);
            return (short) 0;
        }
        short limits = (short) (row * STREAM_LIMIT_COLUMNS);
        byte directions = STREAM_DIRECTIONS[row];
        byte operation = (byte) ((short) (ins & 0xFF) - (short) (STREAM_INS_BASE[row] & 0xFF));
        try {
            return streamSession.dispatch((byte) (row + (short) 1), operation,
                    (directions & (byte) 2) != (byte) 0,
                    STREAM_LIMITS[limits], STREAM_LIMITS[(short) (limits + 1)],
                    (directions & (byte) 1) != (byte) 0,
                    STREAM_LIMITS[(short) (limits + 2)], STREAM_LIMITS[(short) (limits + 3)],
                    STREAM_LIMITS[(short) (limits + 4)], this,
                    p1, p2, requestBuffer, requestOffset, requestLength,
                    responseBuffer, responseOffset, responseCapacity);
        } catch (`)
	b.WriteString(data.StreamEndpointName)
	b.WriteString(`.StreamStatusWordException failure) {
            ISOException.throwIt(failure.getStatusWord());
            return (short) 0;
        }
    }

    /** Abort the one applet-level stream session from deselect cleanup. */
    public final void abortStreams(byte reason) {
        streamSession.abort(reason);
    }

    public final boolean isStreamInstruction(byte ins) {
        return streamRow(ins) >= (short) 0;
    }

    /** The streamed method an instruction belongs to, or -1 when it belongs to none. */
    private static short streamRow(byte ins) {
        short value = (short) (ins & 0xFF);
        for (short row = (short) 0; row < (short) STREAM_INS_BASE.length; row++) {
            short base = (short) (STREAM_INS_BASE[row] & 0xFF);
            if (value >= base && value < (short) (base + STREAM_OPERATION_COUNT)) {
                return row;
            }
        }
        return (short) -1;
    }
`)
	return b.String()
}

// writeJavaByteRows prints count entries, four per line, indented as a Java array
// initialiser body and closed with the brace and semicolon.
func writeJavaByteRows(b *strings.Builder, count int, entry func(int) string) {
	const perLine = 4
	for i := 0; i < count; i++ {
		if i%perLine == 0 {
			b.WriteString("        ")
		}
		b.WriteString(entry(i))
		if i == count-1 {
			b.WriteString(" };\n")
		} else if (i+1)%perLine == 0 {
			b.WriteString(",\n")
		} else {
			b.WriteString(", ")
		}
	}
}

func buildJavaStreamAbstractSupport(data *javaTemplateData, methods []javaMethodRender) string {
	var b strings.Builder
	b.WriteString(`    @Override
    public final void digest(
            byte[] input,
            short inputOffset,
            short inputLength,
            byte[] output,
            short outputOffset) {
        streamSha256.doFinal(input, inputOffset, inputLength, output, outputOffset);
    }

    @Override
    public final short execute(
            byte methodId,
            byte[] input,
            short inputOffset,
            short inputLength,
            byte[] output,
            short outputOffset,
            short outputCapacity) {
        switch (methodId) {
`)
	methodID := byte(1)
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		fmt.Fprintf(&b, "            case %d:\n", methodID)
		fmt.Fprintf(&b, "                return on%sStream(input, inputOffset, inputLength,\n", toPascal(method.Name))
		b.WriteString("                        output, outputOffset, outputCapacity);\n")
		methodID++
	}
	b.WriteString(`            default:
                failStream(SW_INS_NOT_SUPPORTED);
                return (short) 0;
        }
    }

    /** Generated no-allocation failure path for typed stream handlers. */
    protected final void failStream(short statusWord) {
        streamHandlerFailure.setStatusWord(statusWord);
        throw streamHandlerFailure;
    }

`)
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		fmt.Fprintf(&b, "    /** %s; return response length and use failStream for a status word. */\n", method.Signature)
		fmt.Fprintf(&b, "    protected abstract short on%sStream(\n", toPascal(method.Name))
		b.WriteString("            byte[] input, short inputOffset, short inputLength,\n")
		b.WriteString("            byte[] output, short outputOffset, short outputCapacity);\n\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func javaStreamWorkspaceLength(methods []javaMethodRender) int {
	result := 0
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		if method.RequestStream != nil && method.RequestStream.MaxLength > result {
			result = method.RequestStream.MaxLength
		}
		if method.ResponseStream != nil && method.ResponseStream.MaxLength > result {
			result = method.ResponseStream.MaxLength
		}
		if method.ResponseStream == nil && result < 255 {
			result = 255
		}
	}
	if result == 0 {
		return 255
	}
	return result
}

func javaStreamFieldConfig(field *Field) (bool, int, int) {
	if field == nil {
		return false, 0, 0
	}
	return true, field.MaxLength, field.ChunkSize
}
