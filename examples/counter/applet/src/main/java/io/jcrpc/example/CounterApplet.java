package io.jcrpc.counter.example;

import counter.CounterSkeleton;
import counter.CounterTransport;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;
import javacard.framework.Util;

/**
 * Counter applet — business logic.
 * Extends generated CounterSkeleton, implements all RPC methods.
 */
public class CounterApplet extends CounterSkeleton {

    private static final byte VERSION = (byte) 0x01;
    private static final int MAX_DATA_SIZE = 128;
    private static final short DEFAULT_LIMIT = (short) 0x7FFF; // max positive short
    private static final byte[] MOCK_IMSI = {
        (byte) '2', (byte) '5', (byte) '0', (byte) '0', (byte) '1',
        (byte) '1', (byte) '2', (byte) '3', (byte) '4', (byte) '5',
        (byte) '6', (byte) '7', (byte) '8', (byte) '9', (byte) '0'
    };
    private static final byte[] MOCK_DISPLAY_NAME = {
        (byte) 0xD0, (byte) 0x9F, (byte) 0xD1, (byte) 0x80, (byte) 0xD0, (byte) 0xB8,
        (byte) 0xD0, (byte) 0xB2, (byte) 0xD0, (byte) 0xB5, (byte) 0xD1, (byte) 0x82,
        (byte) 0x2C, (byte) 0x20, (byte) 0x42, (byte) 0x53, (byte) 0x69, (byte) 0x6D
    };
    private static final byte[] MOCK_AID = {
        (byte) 0xF0, (byte) 0x00, (byte) 0x00, (byte) 0x01, (byte) 0x01
    };
    private static final byte MOCK_SCHEMA_VERSION = (byte) 0x01;
    private static final byte MOCK_VERSION_MAJOR = (byte) 0x01;
    private static final byte MOCK_VERSION_MINOR = (byte) 0x00;
    private static final byte MOCK_VERSION_PATCH = (byte) 0x00;
    private static final byte MOCK_KEY_ALGORITHM = (byte) 0x01;
    private static final short MOCK_CAPABILITIES = (short) 0x003F;
    private static final byte[] MOCK_SPKI_PREFIX = {
        (byte) 0x30, (byte) 0x59,
        (byte) 0x30, (byte) 0x13,
        (byte) 0x06, (byte) 0x07, (byte) 0x2A, (byte) 0x86,
        (byte) 0x48, (byte) 0xCE, (byte) 0x3D, (byte) 0x02,
        (byte) 0x01,
        (byte) 0x06, (byte) 0x08, (byte) 0x2A, (byte) 0x86,
        (byte) 0x48, (byte) 0xCE, (byte) 0x3D, (byte) 0x03,
        (byte) 0x01, (byte) 0x07,
        (byte) 0x03, (byte) 0x42, (byte) 0x00
    };
    // Classic clinit cannot call builders; initialize owned mock wire at install.
    private final byte[] mockSpki;
    // Built in the constructor: the generated pack* helpers are instance
    // methods so their guards can reuse the skeleton's one preconstructed
    // exception instead of allocating per call.
    private final byte[] mockAppletInfo;
    // Owned scratch: consume borrowed challenge bytes before overlapping writes.
    private final byte[] signatureScratch = new byte[16];

    private short counter;
    private short limit;
    private final byte[] storedData;
    private int storedDataLen;

    public CounterApplet() {
        super(new CounterTransport() {
            @Override
            public byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {
                ISOException.throwIt(ISO7816.SW_INS_NOT_SUPPORTED);
                return null;
            }
        });
        counter = 0;
        limit = DEFAULT_LIMIT;
        storedData = new byte[MAX_DATA_SIZE];
        storedDataLen = -1; // -1 = no data stored
        mockSpki = buildMockSpki();
        mockAppletInfo = buildMockAppletInfo();
    }

    @Override
    protected short onIncrement(byte amount) {
        short inc = (short) (amount & 0xFF);
        short newVal = (short) (counter + inc);
        if (newVal > limit || newVal < counter) { // overflow or exceeds limit
            throw statusWordFailure(SW_OVERFLOW);
        }
        counter = newVal;
        return counter;
    }

    @Override
    protected short onDecrement(byte amount) {
        short dec = (short) (amount & 0xFF);
        if (dec > counter) {
            throw statusWordFailure(SW_UNDERFLOW);
        }
        counter -= dec;
        return counter;
    }

    @Override
    protected short onGet() {
        return counter;
    }

    @Override
    protected void onReset() {
        counter = 0;
    }

    @Override
    protected void onSetLimit(short newLimit) {
        limit = newLimit;
    }

    @Override
    protected short onGetInfo(byte[] output, short outputOffset, short outputCapacity) {
        int off = outputOffset;
        off = packU16(output, off, counter);
        off = packU16(output, off, limit);
        off = packU8(output, off, VERSION);
        off = packBool(output, off, storedDataLen >= 0);
        packBool(output, off, counter == limit);
        return 7;
    }

    @Override
    protected void onStore(byte[] data, short dataOffset, short dataLength) {
        if (dataLength > MAX_DATA_SIZE) throw statusWordFailure(SW_DATA_TOO_LONG);
        Util.arrayCopyNonAtomic(data, dataOffset, storedData, (short) 0, dataLength);
        storedDataLen = dataLength;
    }

    @Override
    protected short onLoad(byte[] output, short outputOffset, short outputCapacity) {
        if (storedDataLen < 0) throw statusWordFailure(SW_NO_DATA);
        return writeBytes(storedData, (short) 0, (short) storedDataLen,
                output, outputOffset, outputCapacity);
    }

    @Override
    protected short onGetSpki(byte[] output, short outputOffset, short outputCapacity) {
        return writeBytes(mockSpki, (short) 0, (short) mockSpki.length,
                output, outputOffset, outputCapacity);
    }

    @Override
    protected short onGetImsi(byte[] output, short outputOffset, short outputCapacity) {
        return writeBytes(MOCK_IMSI, (short) 0, (short) MOCK_IMSI.length,
                output, outputOffset, outputCapacity);
    }

    @Override
    protected short onGetAppletInfo(byte[] output, short outputOffset, short outputCapacity) {
        return writeBytes(mockAppletInfo, (short) 0, (short) mockAppletInfo.length,
                output, outputOffset, outputCapacity);
    }

    @Override
    protected short onSignChallenge(byte[] challenge, short challengeOffset, short challengeLength,
            byte[] output, short outputOffset, short outputCapacity) {
        if (challengeLength == 0) throw statusWordFailure(SW_EMPTY_CHALLENGE);
        short partLen = challengeLength > 8 ? (short) 8 : challengeLength;
        short length = (short) (6 + 2 * partLen);
        requireCapacity(outputCapacity, length);
        for (short i = 0; i < partLen; i++) {
            signatureScratch[i] = (byte) (challenge[(short) (challengeOffset + i)] ^ VERSION ^ (byte) counter);
            signatureScratch[(short) (8 + i)] = (byte) (challenge[(short) (challengeOffset + challengeLength - 1 - i)] ^ (byte) limit ^ 0x5A);
        }
        signatureScratch[0] &= 0x7F;
        signatureScratch[8] &= 0x7F;
        int off = outputOffset;
        off = packU8(output, off, (byte) 0x30);
        off = packU8(output, off, (byte) (length - 2));
        off = packU8(output, off, (byte) 0x02);
        off = packU8(output, off, (byte) partLen);
        off = packBytes(output, off, signatureScratch, 0, partLen);
        off = packU8(output, off, (byte) 0x02);
        off = packU8(output, off, (byte) partLen);
        packBytes(output, off, signatureScratch, 8, partLen);
        return length;
    }

    @Override
    protected short onGetDisplayName(byte[] output, short outputOffset, short outputCapacity) {
        return writeBytes(MOCK_DISPLAY_NAME, (short) 0, (short) MOCK_DISPLAY_NAME.length,
                output, outputOffset, outputCapacity);
    }

    @Override
    protected short onEchoMessage(byte[] message, short messageOffset, short messageLength,
            byte[] output, short outputOffset, short outputCapacity) {
        return writeBytes(message, messageOffset, messageLength, output, outputOffset, outputCapacity);
    }

    private void requireCapacity(short capacity, short length) {
        if (length > capacity) throw statusWordFailure((short) 0x6700);
    }

    private short writeBytes(byte[] source, short offset, short length,
            byte[] output, short outputOffset, short outputCapacity) {
        requireCapacity(outputCapacity, length);
        packBytes(output, outputOffset, source, offset, length);
        return length;
    }

    private static byte[] buildMockEcPoint() {
        byte[] point = new byte[65];
        point[0] = 0x04;
        for (int i = 0; i < 32; i++) {
            point[(short) (1 + i)] = (byte) (0x11 + i);
            point[(short) (33 + i)] = (byte) (0x41 + i);
        }
        return point;
    }

    private static byte[] buildMockSpki() {
        byte[] point = buildMockEcPoint();
        byte[] out = new byte[(short) (MOCK_SPKI_PREFIX.length + point.length)];
        Util.arrayCopyNonAtomic(MOCK_SPKI_PREFIX, (short) 0, out, (short) 0, (short) MOCK_SPKI_PREFIX.length);
        Util.arrayCopyNonAtomic(point, (short) 0, out, (short) MOCK_SPKI_PREFIX.length, (short) point.length);
        return out;
    }

    private byte[] buildMockAppletInfo() {
        byte[] out = new byte[12];
        int off = 0;
        off = packU8(out, off, MOCK_SCHEMA_VERSION);
        off = packBytes(out, off, MOCK_AID, 0, MOCK_AID.length);
        off = packU8(out, off, MOCK_VERSION_MAJOR);
        off = packU8(out, off, MOCK_VERSION_MINOR);
        off = packU8(out, off, MOCK_VERSION_PATCH);
        off = packU8(out, off, MOCK_KEY_ALGORITHM);
        packU16(out, off, MOCK_CAPABILITIES);
        return out;
    }

}
