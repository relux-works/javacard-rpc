package io.jcrpc.bridge.session;

import com.licel.jcardsim.smartcardio.CardSimulator;

import javax.smartcardio.CommandAPDU;
import javax.smartcardio.ResponseAPDU;

/**
 * A card session over a jCardSim CardSimulator supplied by a CardProvider.
 *
 * Under connection scope every TCP connection owns its own instance and card.
 * Under shared scope a single instance wraps the one shared card, and its
 * methods are serialized under this object's monitor so concurrent
 * connections never interleave inside the simulator.
 */
public final class SimulatorSession {
    private final CardSimulator sim;

    public SimulatorSession(CardSimulator sim) {
        if (sim == null) throw new IllegalArgumentException("CardSimulator must not be null");
        this.sim = sim;
    }

    /**
     * Forward raw C-APDU bytes to the simulator, return raw R-APDU bytes.
     */
    public synchronized byte[] transmitAPDU(byte[] capdu) {
        ResponseAPDU resp = sim.transmitCommand(new CommandAPDU(capdu));
        return resp.getBytes();
    }

    /**
     * Reset the card (clears selection and transient state, keeps persistent).
     * Under shared scope this is visible to every connection.
     */
    public synchronized void reset() {
        sim.reset();
    }

    /**
     * Get the ATR (Answer To Reset) bytes.
     */
    public synchronized byte[] getATR() {
        return sim.getATR();
    }
}
