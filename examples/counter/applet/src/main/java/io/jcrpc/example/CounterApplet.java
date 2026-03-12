package io.jcrpc.counter.example;

import counter.CounterSkeleton;
import counter.CounterTransport;

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
    private static final byte[] MOCK_AID = {
        (byte) 0xF0, (byte) 0x00, (byte) 0x00, (byte) 0x01, (byte) 0x01
    };
    private static final byte[] MOCK_APPLET_VERSION = {
        (byte) '1', (byte) '.', (byte) '0', (byte) '.', (byte) '0'
    };
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
    private static final byte[] MOCK_EC_POINT = buildMockEcPoint();
    private static final byte[] MOCK_SPKI = buildMockSpki();
    private static final byte[] MOCK_APPLET_INFO = buildMockAppletInfo();

    private short counter;
    private short limit;
    private final byte[] storedData;
    private int storedDataLen;

    public CounterApplet() {
        super(new CounterTransport() {
            @Override
            public byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {
                throw new UnsupportedOperationException("no outgoing transport");
            }
        });
        counter = 0;
        limit = DEFAULT_LIMIT;
        storedData = new byte[MAX_DATA_SIZE];
        storedDataLen = -1; // -1 = no data stored
    }

    @Override
    protected short onIncrement(byte amount) {
        short inc = (short) (amount & 0xFF);
        short newVal = (short) (counter + inc);
        if (newVal > limit || newVal < counter) { // overflow or exceeds limit
            throw new StatusWordException(SW_OVERFLOW);
        }
        counter = newVal;
        return counter;
    }

    @Override
    protected short onDecrement(byte amount) {
        short dec = (short) (amount & 0xFF);
        if (dec > counter) {
            throw new StatusWordException(SW_UNDERFLOW);
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
    protected byte[] onGetInfo() {
        byte[] buf = new byte[7]; // u16 + u16 + u8 + bool + bool
        int off = 0;
        off = packU16(buf, off, counter);
        off = packU16(buf, off, limit);
        off = packU8(buf, off, VERSION);
        off = packBool(buf, off, storedDataLen >= 0);
        packBool(buf, off, counter == limit);
        return buf;
    }

    @Override
    protected void onStore(byte[] data) {
        if (data.length > MAX_DATA_SIZE) {
            throw new StatusWordException(SW_DATA_TOO_LONG);
        }
        System.arraycopy(data, 0, storedData, 0, data.length);
        storedDataLen = data.length;
    }

    @Override
    protected byte[] onLoad() {
        if (storedDataLen < 0) {
            throw new StatusWordException(SW_NO_DATA);
        }
        byte[] result = new byte[storedDataLen];
        System.arraycopy(storedData, 0, result, 0, storedDataLen);
        return result;
    }

    @Override
    protected byte[] onGetSpki() {
        return copyBytes(MOCK_SPKI);
    }

    @Override
    protected byte[] onGetImsi() {
        return copyBytes(MOCK_IMSI);
    }

    @Override
    protected byte[] onGetAppletInfo() {
        return copyBytes(MOCK_APPLET_INFO);
    }

    @Override
    protected byte[] onSignChallenge(byte[] challenge) {
        if (challenge.length == 0) {
            throw new StatusWordException(SW_EMPTY_CHALLENGE);
        }
        return buildMockSignature(challenge);
    }

    private static byte[] buildMockEcPoint() {
        byte[] point = new byte[65];
        point[0] = 0x04;
        for (int i = 0; i < 32; i++) {
            point[1 + i] = (byte) (0x11 + i);
            point[33 + i] = (byte) (0x41 + i);
        }
        return point;
    }

    private static byte[] buildMockSpki() {
        byte[] out = new byte[MOCK_SPKI_PREFIX.length + MOCK_EC_POINT.length];
        System.arraycopy(MOCK_SPKI_PREFIX, 0, out, 0, MOCK_SPKI_PREFIX.length);
        System.arraycopy(MOCK_EC_POINT, 0, out, MOCK_SPKI_PREFIX.length, MOCK_EC_POINT.length);
        return out;
    }

    private static byte[] buildMockAppletInfo() {
        byte[] out = new byte[24];
        int off = 0;
        off = putTlvScalar(out, off, (byte) 0x01, (byte) 0x01);
        off = putTlvBytes(out, off, (byte) 0x02, MOCK_AID);
        off = putTlvBytes(out, off, (byte) 0x03, MOCK_APPLET_VERSION);
        off = putTlvScalar(out, off, (byte) 0x04, (byte) 0x01);
        putTlvBytes(out, off, (byte) 0x05, new byte[] { (byte) 0x00, (byte) 0x3F });
        return out;
    }

    private byte[] buildMockSignature(byte[] challenge) {
        int partLen = Math.min(challenge.length, 8);
        byte[] out = new byte[2 + 2 + partLen + 2 + partLen];
        out[0] = 0x30;
        out[1] = (byte) (out.length - 2);
        out[2] = 0x02;
        out[3] = (byte) partLen;

        for (int i = 0; i < partLen; i++) {
            out[4 + i] = (byte) (challenge[i] ^ VERSION ^ (byte) counter);
        }
        out[4] = (byte) (out[4] & 0x7F);

        int secondOff = 4 + partLen;
        out[secondOff] = 0x02;
        out[secondOff + 1] = (byte) partLen;
        for (int i = 0; i < partLen; i++) {
            int srcIndex = challenge.length - 1 - i;
            out[secondOff + 2 + i] = (byte) (challenge[srcIndex] ^ (byte) limit ^ 0x5A);
        }
        out[secondOff + 2] = (byte) (out[secondOff + 2] & 0x7F);
        return out;
    }

    private static int putTlvScalar(byte[] out, int off, byte tag, byte value) {
        out[off++] = tag;
        out[off++] = 0x01;
        out[off++] = value;
        return off;
    }

    private static int putTlvBytes(byte[] out, int off, byte tag, byte[] value) {
        out[off++] = tag;
        out[off++] = (byte) value.length;
        System.arraycopy(value, 0, out, off, value.length);
        return off + value.length;
    }

    private static byte[] copyBytes(byte[] source) {
        byte[] out = new byte[source.length];
        System.arraycopy(source, 0, out, 0, source.length);
        return out;
    }

}
