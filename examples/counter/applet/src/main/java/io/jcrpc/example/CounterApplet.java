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
        byte[] buf = new byte[5]; // u16 + u16 + u8
        int off = 0;
        off = packU16(buf, off, counter);
        off = packU16(buf, off, limit);
        packU8(buf, off, VERSION);
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

}
