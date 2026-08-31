package codegen

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const javaStreamEndpointTemplate = `package {{.PackageName}};

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
         * Return 0..outputCapacity. Use the generated skeleton's
         * failStream(statusWord) helper for a business status word.
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

    /** One mutable exception object is allocated once at applet installation. */
    final class StreamStatusWordException extends RuntimeException {
        private short statusWord;

        public StreamStatusWordException(short statusWord) {
            super();
            this.statusWord = statusWord;
        }

        public void setStatusWord(short statusWord) {
            this.statusWord = statusWord;
        }

        public short getStatusWord() {
            return statusWord;
        }
    }
}
`

const javaStreamRuntimeTemplate = `package {{.PackageName}};

/**
 * Generated single-owner, bounded, half-duplex stream state machine.
 *
 * The skeleton constructs exactly one instance and injects CLEAR_ON_DESELECT
 * arrays. Reset/deselect clears the transient marker; the next dispatch first
 * wipes all persistent scalar metadata. Command processing reuses one exception
 * object and never allocates an object or array.
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
    private static final byte RESET_MARKER_LIVE  = (byte) 0x5A;

    private static final short DIGEST_LENGTH = (short) 32;
    private static final short CLOSE_LENGTH = (short) 34;
    private static final short DESCRIPTOR_LENGTH = (short) 35;
    private static final short SHORT_RESPONSE_CAPACITY = (short) 255;

    private final byte[] workspace;
    private final byte[] digestScratch;
    private final byte[] resetMarker;
    private final Sha256 sha256;
    private final StreamStatusWordException failure;

    private byte state;
    private byte activeMethod;
    private boolean hasRequestStream;
    private boolean hasResponseStream;
    private short requestMaxLength;
    private short requestChunkSize;
    private short responseMaxLength;
    private short responseChunkSize;
    private Handler activeHandler;

    private short inputLength;
    private short lastChunkOffset;
    private short lastChunkLength;
    private short shortResultLength;
    private short resultLength;
    private byte packetCount;
    private byte nextPacketIndex;
    private byte resultPacketCount;

    public {{.StreamRuntimeName}}(
            byte[] workspace,
            byte[] digestScratch,
            byte[] resetMarker,
            Sha256 sha256) {
        this.workspace = workspace;
        this.digestScratch = digestScratch;
        this.resetMarker = resetMarker;
        this.sha256 = sha256;
        this.failure = new StreamStatusWordException(SW_WRONG_STATE);

        if (workspace == null || workspace.length == 0 ||
                digestScratch == null || digestScratch.length < DIGEST_LENGTH ||
                resetMarker == null || resetMarker.length < 1 || sha256 == null) {
            failure.setStatusWord(SW_NO_MEMORY);
            throw failure;
        }
        clearAll();
        resetMarker[0] = RESET_MARKER_LIVE;
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
            Handler handler,
            byte p1,
            byte p2,
            byte[] requestBuffer,
            short requestOffset,
            short requestLength,
            byte[] responseBuffer,
            short responseOffset,
            short responseCapacity) {
        ensureResetBoundary();
        try {
            validateRange(requestBuffer, requestOffset, requestLength);
            validateRange(responseBuffer, responseOffset, responseCapacity);

            if (operation == OP_ABORT) {
                requireEmptyCommand(p1, p2, requestLength);
                abort(ABORT_EXPLICIT);
                return (short) 0;
            }

            if (operation == OP_WRITE_OR_INVOKE) {
                if (state == STATE_READ_CLOSED || state == STATE_SHORT_CLOSED) {
                    clearAll();
                }
                if (state == STATE_EMPTY) {
                    begin(methodId, requestEnabled, requestLimit, requestChunk,
                            responseEnabled, responseLimit, responseChunk, handler);
                } else if (activeMethod != methodId) {
                    rejectWithoutClearing(SW_WRONG_STATE);
                }
            } else if (state == STATE_EMPTY || activeMethod != methodId) {
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

    private void ensureResetBoundary() {
        if (resetMarker[0] != RESET_MARKER_LIVE) {
            clearAll();
            resetMarker[0] = RESET_MARKER_LIVE;
        }
    }

    private void begin(
            byte methodId,
            boolean requestEnabled,
            short requestLimit,
            short requestChunk,
            boolean responseEnabled,
            short responseLimit,
            short responseChunk,
            Handler handler) {
        short required = requestEnabled ? requestLimit : (short) 0;
        if (responseEnabled && responseLimit > required) {
            required = responseLimit;
        }
        if (!responseEnabled && required < SHORT_RESPONSE_CAPACITY) {
            required = SHORT_RESPONSE_CAPACITY;
        }
        if ((!requestEnabled && !responseEnabled) || handler == null ||
                (requestEnabled && (requestLimit <= 0 || requestChunk <= 0)) ||
                (responseEnabled && (responseLimit <= 0 || responseChunk <= 0)) ||
                required > workspace.length) {
            fail(SW_NO_MEMORY);
        }

        activeMethod = methodId;
        hasRequestStream = requestEnabled;
        hasResponseStream = responseEnabled;
        requestMaxLength = requestLimit;
        requestChunkSize = requestChunk;
        responseMaxLength = responseLimit;
        responseChunkSize = responseChunk;
        activeHandler = handler;
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
        if (!hasRequestStream) {
            if (state != STATE_EMPTY) {
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
        int chunkSize = requestChunkSize & 0xFFFF;
        int maxPackets = ((requestMaxLength & 0xFFFF) + chunkSize - 1) / chunkSize;
        if (count == 0 || count > maxPackets || index >= count || length == 0 ||
                length > chunkSize || (index < count - 1 && length != chunkSize)) {
            fail(SW_WRONG_LENGTH);
        }

        if (state == STATE_WRITING && index == ((nextPacketIndex & 0xFF) - 1)) {
            if (count != (packetCount & 0xFF) || length != (lastChunkLength & 0xFFFF) ||
                    !equalsRange(workspace, lastChunkOffset, request, requestOffset, requestLength)) {
                fail(SW_INVALID_DATA);
            }
            return (short) 0;
        }

        if (index == 0 && state == STATE_EMPTY) {
            state = STATE_WRITING;
            packetCount = p2;
            nextPacketIndex = (byte) 0;
        }
        if (state != STATE_WRITING || count != (packetCount & 0xFF) ||
                index != (nextPacketIndex & 0xFF)) {
            fail(SW_INVALID_DATA);
        }
        if ((inputLength & 0xFFFF) + length > (requestMaxLength & 0xFFFF)) {
            fail(SW_WRONG_LENGTH);
        }

        lastChunkOffset = inputLength;
        lastChunkLength = requestLength;
        copy(request, requestOffset, workspace, inputLength, requestLength);
        inputLength = (short) ((inputLength & 0xFFFF) + length);
        nextPacketIndex = (byte) (index + 1);
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
        if (state == STATE_SHORT_CLOSED) {
            if (declaredLength != (inputLength & 0xFFFF) ||
                    !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
                fail(SW_INVALID_DATA);
            }
            ensureResponseCapacity(responseCapacity, shortResultLength);
            copy(workspace, (short) 0, response, responseOffset, shortResultLength);
            return shortResultLength;
        }
        if (!hasRequestStream || state != STATE_WRITING ||
                (nextPacketIndex & 0xFF) != (packetCount & 0xFF)) {
            fail(SW_WRONG_STATE);
        }
        if (declaredLength != (inputLength & 0xFFFF)) {
            fail(SW_WRONG_LENGTH);
        }

        preflightHandlerResponse(responseCapacity);
        sha256.digest(workspace, (short) 0, inputLength, digestScratch, (short) 0);
        if (!equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
            fail(SW_INVALID_DATA);
        }
        return executeOnce(workspace, (short) 0, inputLength,
                response, responseOffset, responseCapacity, true);
    }

    private void preflightHandlerResponse(short responseCapacity) {
        ensureResponseCapacity(responseCapacity,
                hasResponseStream ? DESCRIPTOR_LENGTH : SHORT_RESPONSE_CAPACITY);
    }

    private short executeOnce(
            byte[] input,
            short inputOffset,
            short inputSize,
            byte[] response,
            short responseOffset,
            short responseCapacity,
            boolean preserveInputCloseReceipt) {
        short outputCapacity = hasResponseStream ? responseMaxLength : SHORT_RESPONSE_CAPACITY;
        short produced = activeHandler.execute(activeMethod, input, inputOffset, inputSize,
                workspace, (short) 0, outputCapacity);
        if (produced < 0 || produced > outputCapacity || (hasResponseStream && produced == 0)) {
            fail(SW_WRONG_LENGTH);
        }

        resultLength = produced;
        if (hasResponseStream) {
            sha256.digest(workspace, (short) 0, resultLength, digestScratch, (short) 0);
            int chunks = ((resultLength & 0xFFFF) + (responseChunkSize & 0xFFFF) - 1) /
                    (responseChunkSize & 0xFFFF);
            resultPacketCount = (byte) chunks;
            state = STATE_READ_PENDING;
            return writeDescriptor(response, responseOffset, responseCapacity);
        }

        copy(workspace, (short) 0, response, responseOffset, produced);
        shortResultLength = produced;
        if (preserveInputCloseReceipt) {
            state = STATE_SHORT_CLOSED;
        } else {
            clearAll();
        }
        return produced;
    }

    private short pendingInfo(byte[] response, short responseOffset, short responseCapacity) {
        if (!hasResponseStream || state != STATE_READ_PENDING) {
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
        if (!hasResponseStream || state != STATE_READ_PENDING) {
            fail(SW_WRONG_STATE);
        }
        int index = p1 & 0xFF;
        int count = p2 & 0xFF;
        if (count != (resultPacketCount & 0xFF) || index >= count) {
            fail(SW_INVALID_DATA);
        }
        int offset = index * (responseChunkSize & 0xFFFF);
        int remaining = (resultLength & 0xFFFF) - offset;
        int length = remaining < (responseChunkSize & 0xFFFF) ?
                remaining : (responseChunkSize & 0xFFFF);
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

        if (state != STATE_READ_PENDING && state != STATE_READ_CLOSED) {
            fail(SW_WRONG_STATE);
        }
        if (declaredLength != (resultLength & 0xFFFF) ||
                !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)) {
            fail(SW_INVALID_DATA);
        }
        if (state == STATE_READ_PENDING) {
            wipe(workspace);
            state = STATE_READ_CLOSED;
        }
        return (short) 0;
    }

    private short writeDescriptor(byte[] response, short offset, short capacity) {
        ensureResponseCapacity(capacity, DESCRIPTOR_LENGTH);
        response[offset] = resultPacketCount;
        response[(short) (offset + 1)] = (byte) ((resultLength >>> 8) & 0xFF);
        response[(short) (offset + 2)] = (byte) (resultLength & 0xFF);
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

    private void clearAll() {
        wipe(workspace);
        wipe(digestScratch);
        state = STATE_EMPTY;
        activeMethod = 0;
        hasRequestStream = false;
        hasResponseStream = false;
        requestMaxLength = 0;
        requestChunkSize = 0;
        responseMaxLength = 0;
        responseChunkSize = 0;
        activeHandler = null;
        inputLength = 0;
        lastChunkOffset = 0;
        lastChunkLength = 0;
        shortResultLength = 0;
        resultLength = 0;
        packetCount = 0;
        nextPacketIndex = 0;
        resultPacketCount = 0;
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

import javacard.framework.JCSystem;
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
		"    protected %s(%s transport) {\n        this.transport = transport;\n        this.empty = new byte[0];\n    }",
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
    public static final short STREAM_RESET_MARKER_LENGTH = (short) 1;

    private final %s streamSession;
    private final MessageDigest streamSha256;
    private final %s.StreamStatusWordException streamHandlerFailure;`, workspaceLength, data.StreamRuntimeName, data.StreamEndpointName)
}

func buildJavaStreamConstructor(data *javaTemplateData) string {
	return fmt.Sprintf(`    protected %s(%s transport) {
        this.transport = transport;
        this.empty = new byte[0];
        this.streamSha256 = MessageDigest.getInstance(MessageDigest.ALG_SHA_256, false);
        this.streamHandlerFailure = new %s.StreamStatusWordException((short) 0x6985);
        this.streamSession = new %s(
                JCSystem.makeTransientByteArray(
                        STREAM_WORKSPACE_LENGTH, JCSystem.CLEAR_ON_DESELECT),
                JCSystem.makeTransientByteArray(
                        STREAM_DIGEST_LENGTH, JCSystem.CLEAR_ON_DESELECT),
                JCSystem.makeTransientByteArray(
                        STREAM_RESET_MARKER_LENGTH, JCSystem.CLEAR_ON_DESELECT),
                this);
    }`, data.ClassName, data.TransportInterfaceName, data.StreamEndpointName, data.StreamRuntimeName)
}

func buildJavaStreamDispatchSupport(data *javaTemplateData, methods []javaMethodRender) string {
	var b strings.Builder
	b.WriteString(`    /**
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
        try {
            switch (ins) {
`)
	operations := []string{
		"OP_WRITE_OR_INVOKE",
		"OP_CLOSE_WRITE",
		"OP_GET_PENDING_READ_INFO",
		"OP_READ_CHUNK",
		"OP_CLOSE_READ",
		"OP_ABORT",
	}
	suffixes := []string{
		"WRITE_OR_INVOKE",
		"CLOSE_WRITE",
		"GET_PENDING_READ_INFO",
		"READ_CHUNK",
		"CLOSE_READ",
		"ABORT",
	}
	methodID := byte(1)
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		requestEnabled, requestMax, requestChunk := javaStreamFieldConfig(method.RequestStream)
		responseEnabled, responseMax, responseChunk := javaStreamFieldConfig(method.ResponseStream)
		for i := range operations {
			fmt.Fprintf(&b, "                case %s_%s:\n", method.INSConstName, suffixes[i])
			fmt.Fprintf(&b, "                    return streamSession.dispatch((byte) %d, %s.%s,\n", methodID, data.StreamEndpointName, operations[i])
			fmt.Fprintf(&b, "                            %t, (short) %d, (short) %d,\n", requestEnabled, requestMax, requestChunk)
			fmt.Fprintf(&b, "                            %t, (short) %d, (short) %d, this,\n", responseEnabled, responseMax, responseChunk)
			b.WriteString("                            p1, p2, requestBuffer, requestOffset, requestLength,\n")
			b.WriteString("                            responseBuffer, responseOffset, responseCapacity);\n")
		}
		methodID++
	}
	b.WriteString(`                default:
                    ISOException.throwIt(SW_INS_NOT_SUPPORTED);
                    return (short) 0;
            }
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
        switch (ins) {
`)
	for _, method := range methods {
		if !method.IsStream {
			continue
		}
		for _, suffix := range suffixes {
			fmt.Fprintf(&b, "            case %s_%s:\n", method.INSConstName, suffix)
		}
	}
	b.WriteString(`                return true;
            default:
                return false;
        }
    }
`)
	return b.String()
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
