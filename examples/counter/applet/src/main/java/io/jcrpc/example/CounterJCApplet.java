package io.jcrpc.counter.example;

import counter.CounterSkeleton;
import javacard.framework.APDU;
import javacard.framework.Applet;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;

/** Command-local DI adapter from APDU to the generated ordinary writer. */
public class CounterJCApplet extends Applet {
    private final CounterApplet logic;

    private CounterJCApplet() {
        logic = new CounterApplet();
        register();
    }

    public static void install(byte[] bArray, short bOffset, byte bLength) {
        new CounterJCApplet();
    }

    @Override
    public void process(APDU apdu) {
        if (selectingApplet()) return;
        byte[] buffer = apdu.getBuffer();
        if (buffer[ISO7816.OFFSET_CLA] != CounterSkeleton.CLA_COUNTER) {
            ISOException.throwIt(ISO7816.SW_CLA_NOT_SUPPORTED);
        }
        // Capture the header before receiving/overlapping response writes.
        byte ins = buffer[ISO7816.OFFSET_INS];
        byte p1 = buffer[ISO7816.OFFSET_P1];
        byte p2 = buffer[ISO7816.OFFSET_P2];
        short received = apdu.setIncomingAndReceive();
        short requestLength = apdu.getIncomingLength();
        short requestOffset = apdu.getOffsetCdata();
        // This caller supports short APDUs only, and stages the entire request
        // in the actual APDU buffer. Extended/oversize requests never dispatch.
        if (requestOffset != ISO7816.OFFSET_CDATA || requestLength < 0
                || requestLength > 255 || buffer.length > 32767
                || requestOffset > buffer.length
                || requestLength > buffer.length - requestOffset
                || received < 0 || received > requestLength) {
            ISOException.throwIt(ISO7816.SW_WRONG_LENGTH);
        }
        while (received < requestLength) {
            short fragment = apdu.receiveBytes((short) (requestOffset + received));
            if (fragment <= 0 || fragment > requestLength - received) {
                ISOException.throwIt(ISO7816.SW_WRONG_LENGTH);
            }
            received += fragment;
        }
        short outputOffset = 0;
        short outputCapacity = buffer.length > 255 ? (short) 255 : (short) buffer.length;
        try {
            short produced = logic.dispatchTo(ins, p1, p2,
                    buffer, requestOffset, requestLength,
                    buffer, outputOffset, outputCapacity);
            // Only a successful dispatch supplies a sendable span.
            if (produced > 0) {
                apdu.setOutgoingAndSend(outputOffset, produced);
            }
        } catch (CounterSkeleton.StatusWordException e) {
            ISOException.throwIt(e.getStatusWord());
        }
    }
}
