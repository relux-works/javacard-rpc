package io.jcrpc.bridge.card;

import io.jcrpc.bridge.config.AppletConfig;
import com.licel.jcardsim.smartcardio.CardSimulator;
import com.licel.jcardsim.utils.AIDUtil;
import javacard.framework.AID;
import javacard.framework.Applet;

import java.util.List;

/**
 * Default provider: a plain jCardSim CardSimulator with the --config applets
 * installed in order using their install params. This is exactly what the
 * bridge did before the SPI existed.
 */
public final class DefaultCardProvider implements CardProvider {
    private final List<AppletConfig> applets;

    public DefaultCardProvider(List<AppletConfig> applets) {
        this.applets = applets;
    }

    @Override
    @SuppressWarnings("unchecked")
    public CardSimulator create() {
        CardSimulator sim = new CardSimulator();
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
        return sim;
    }
}
