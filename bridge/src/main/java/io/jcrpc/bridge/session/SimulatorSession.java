package io.jcrpc.bridge.session;

import io.jcrpc.bridge.config.AppletConfig;
import com.licel.jcardsim.smartcardio.CardSimulator;
import com.licel.jcardsim.utils.AIDUtil;
import javacard.framework.AID;
import javacard.framework.Applet;

import javax.smartcardio.CommandAPDU;
import javax.smartcardio.ResponseAPDU;
import java.util.List;

/**
 * A card session backed by jCardSim.
 * One session per TCP connection — fully isolated.
 */
public final class SimulatorSession {
    private final CardSimulator sim;

    @SuppressWarnings("unchecked")
    public SimulatorSession(List<AppletConfig> applets) {
        sim = new CardSimulator();
        for (AppletConfig cfg : applets) {
            try {
                AID aid = AIDUtil.create(cfg.getAidHex());
                Class<?> clazz = Class.forName(cfg.getClassName());
                byte[] params = cfg.buildInstallParams();
                sim.installApplet(aid, (Class<? extends Applet>) clazz,
                        params, (short) 0, (byte) params.length);
            } catch (ClassNotFoundException e) {
                System.err.println("[session] Applet class not found: " + cfg.getClassName());
                throw new RuntimeException(e);
            }
        }
    }

    /**
     * Forward raw C-APDU bytes to the simulator, return raw R-APDU bytes.
     */
    public byte[] transmitAPDU(byte[] capdu) {
        ResponseAPDU resp = sim.transmitCommand(new CommandAPDU(capdu));
        return resp.getBytes();
    }

    /**
     * Reset the card (clears selection and transient state, keeps persistent).
     */
    public void reset() {
        sim.reset();
    }

    /**
     * Get the ATR (Answer To Reset) bytes.
     */
    public byte[] getATR() {
        return sim.getATR();
    }
}
