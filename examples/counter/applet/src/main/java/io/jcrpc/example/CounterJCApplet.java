package io.jcrpc.counter.example;

import counter.CounterSkeleton;
import javacard.framework.APDU;
import javacard.framework.Applet;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;
import javacard.framework.Util;

/**
 * Java Card wrapper for CounterApplet.
 * Bridges javacard.framework.APDU → CounterSkeleton.dispatch().
 * This is the DI adapter: jCardSim sees a real JC Applet,
 * business logic lives in CounterApplet (extends CounterSkeleton).
 */
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
        byte[] buf = apdu.getBuffer();
        if (selectingApplet()) {
            return;
        }

        if (buf[ISO7816.OFFSET_CLA] != CounterSkeleton.CLA_COUNTER) {
            ISOException.throwIt(ISO7816.SW_CLA_NOT_SUPPORTED);
        }

        byte ins = buf[ISO7816.OFFSET_INS];
        byte p1 = buf[ISO7816.OFFSET_P1];
        byte p2 = buf[ISO7816.OFFSET_P2];

        // Read incoming data
        short lc = apdu.setIncomingAndReceive();
        byte[] data = null;
        if (lc > 0) {
            data = new byte[lc];
            Util.arrayCopy(buf, ISO7816.OFFSET_CDATA, data, (short) 0, lc);
        }

        try {
            byte[] response = logic.dispatch(ins, p1, p2, data);
            if (response != null && response.length > 0) {
                Util.arrayCopy(response, (short) 0, buf, (short) 0, (short) response.length);
                apdu.setOutgoingAndSend((short) 0, (short) response.length);
            }
        } catch (CounterSkeleton.StatusWordException e) {
            ISOException.throwIt(e.getStatusWord());
        }
    }
}
