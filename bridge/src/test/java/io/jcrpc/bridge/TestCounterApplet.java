package io.jcrpc.bridge;

import javacard.framework.APDU;
import javacard.framework.Applet;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;

/**
 * Minimal persistent counter applet for bridge tests (no dependency on the
 * generated counter example). CLA B0: INS 01 = increment, INS 02 = read (2 bytes).
 * Install params in JC format [aidLen][aid][privLen][priv][c9Len][c9]: a 2-byte c9
 * seeds the counter, so the bridge's install-param plumbing is observable.
 */
public class TestCounterApplet extends Applet {
    public static final byte CLA = (byte) 0xB0;
    public static final byte INS_INC = 0x01;
    public static final byte INS_READ = 0x02;

    private short counter = 0;

    private TestCounterApplet(short seed) {
        counter = seed;
    }

    public static void install(byte[] bArray, short bOffset, byte bLength) {
        short seed = 0;
        if (bArray != null && bLength > 0) {
            short off = bOffset;
            short aidLen = (short) (bArray[off++] & 0xFF);
            off += aidLen;
            short privLen = (short) (bArray[off++] & 0xFF);
            off += privLen;
            short c9Len = (short) (bArray[off++] & 0xFF);
            if (c9Len == 2) {
                seed = (short) (((bArray[off] & 0xFF) << 8) | (bArray[(short) (off + 1)] & 0xFF));
            }
        }
        new TestCounterApplet(seed).register();
    }

    @Override
    public void process(APDU apdu) {
        if (selectingApplet()) return;
        byte[] buf = apdu.getBuffer();
        if (buf[ISO7816.OFFSET_CLA] != CLA) ISOException.throwIt(ISO7816.SW_CLA_NOT_SUPPORTED);
        switch (buf[ISO7816.OFFSET_INS]) {
            case INS_INC:
                counter++;
                return;
            case INS_READ:
                buf[0] = (byte) (counter >> 8);
                buf[1] = (byte) counter;
                apdu.setOutgoingAndSend((short) 0, (short) 2);
                return;
            default:
                ISOException.throwIt(ISO7816.SW_INS_NOT_SUPPORTED);
        }
    }
}
