package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.card.DefaultCardProvider;
import io.jcrpc.bridge.config.AppletConfig;
import io.jcrpc.bridge.config.BridgeConfig;
import com.licel.jcardsim.smartcardio.CardSimulator;
import com.licel.jcardsim.utils.AIDUtil;
import javacard.framework.AID;
import org.junit.jupiter.api.Test;

import javax.smartcardio.CommandAPDU;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * DefaultCardProvider must reproduce the pre-SPI bridge byte for byte:
 * new CardSimulator() + installApplet(aid, class, installParams) per --config entry.
 */
class DefaultCardProviderTest {
    private static final List<AppletConfig> APPLETS = Collections.singletonList(
            new AppletConfig("F000000101", TestCounterApplet.class.getName(), "0007"));

    /** The exact construction the bridge performed before the SPI (SimulatorSession v0.2.1). */
    @SuppressWarnings("unchecked")
    private static CardSimulator legacyBuild(List<AppletConfig> applets) throws Exception {
        CardSimulator sim = new CardSimulator();
        for (AppletConfig cfg : applets) {
            AID aid = AIDUtil.create(cfg.getAidHex());
            Class<?> clazz = Class.forName(cfg.getClassName());
            byte[] params = cfg.buildInstallParams();
            sim.installApplet(aid, (Class<? extends javacard.framework.Applet>) clazz,
                    params, (short) 0, (byte) params.length);
        }
        return sim;
    }

    private static byte[][] replay(CardSimulator sim) {
        byte[] select = new byte[5 + BridgeTestClient.AID.length];
        select[1] = (byte) 0xA4; select[2] = 0x04; select[4] = (byte) BridgeTestClient.AID.length;
        System.arraycopy(BridgeTestClient.AID, 0, select, 5, BridgeTestClient.AID.length);
        return new byte[][]{
                sim.getATR(),
                sim.transmitCommand(new CommandAPDU(select)).getBytes(),
                sim.transmitCommand(new CommandAPDU(new byte[]{TestCounterApplet.CLA, TestCounterApplet.INS_INC, 0, 0})).getBytes(),
                sim.transmitCommand(new CommandAPDU(new byte[]{TestCounterApplet.CLA, TestCounterApplet.INS_READ, 0, 0, 2})).getBytes(),
                sim.transmitCommand(new CommandAPDU(new byte[]{0x00, 0x00, 0, 0})).getBytes(), // CLA not supported
        };
    }

    // Proves: ATR, SELECT, INC, READ and an error R-APDU are byte-identical between the
    // legacy construction and DefaultCardProvider.create() (fresh card per call).
    @Test
    void createIsByteIdenticalToLegacyConstruction() throws Exception {
        byte[][] legacy = replay(legacyBuild(APPLETS));
        byte[][] viaProvider = replay(new DefaultCardProvider(APPLETS).create());
        for (int i = 0; i < legacy.length; i++) {
            assertArrayEquals(legacy[i], viaProvider[i], "step " + i + " differs: "
                    + Arrays.toString(legacy[i]) + " vs " + Arrays.toString(viaProvider[i]));
        }
        assertEquals(0x9000, BridgeTestClient.sw(viaProvider[1]));
        // c9=0007 seeded the counter through the install params, then one INC
        assertEquals(8, BridgeTestClient.counter(viaProvider[3]));
        assertNotEquals(0x9000, BridgeTestClient.sw(viaProvider[4]));
    }

    // Proves: every create() call yields a new card (connection scope isolation relies on this).
    @Test
    void createReturnsFreshCardEachCall() {
        DefaultCardProvider p = new DefaultCardProvider(APPLETS);
        CardSimulator a = p.create();
        CardSimulator b = p.create();
        assertTrue(a != b);
    }

    // Proves: the legacy 3-arg BridgeConfig keeps today's wiring — default provider, connection scope.
    @Test
    void legacyBridgeConfigDefaultsToDefaultProviderAndConnectionScope() {
        BridgeConfig c = new BridgeConfig("127.0.0.1", 0, APPLETS);
        assertEquals(DefaultCardProvider.class, c.getCardProvider().getClass());
        assertEquals(CardScope.CONNECTION, c.getCardScope());
    }

    // Proves: the whole legacy path — 3-arg BridgeConfig → TcpBridgeServer → ClientHandler —
    // still serves the --config applet over TCP with the same R-APDUs as a direct simulator.
    @Test
    void legacyConfigServesAppletOverTcp() throws Exception {
        byte[][] legacy = replay(legacyBuild(APPLETS));
        try (BridgeHarness h = new BridgeHarness(new BridgeConfig("127.0.0.1", 0, APPLETS));
             BridgeTestClient c = h.connect()) {
            assertArrayEquals(legacy[0], c.atr());
            assertArrayEquals(legacy[1], c.select());
            assertArrayEquals(legacy[2], c.increment());
            assertArrayEquals(legacy[3], c.read());
        }
    }

    // Negative: a --config applet class that does not exist still fails the same way as before
    // (RuntimeException from create), which TcpBridgeServer turns into a startup refusal.
    @Test
    void missingAppletClassFailsCreate() {
        DefaultCardProvider p = new DefaultCardProvider(Collections.singletonList(
                new AppletConfig("F000000101", "io.jcrpc.nope.Missing", "")));
        RuntimeException e = org.junit.jupiter.api.Assertions.assertThrows(RuntimeException.class, p::create);
        assertTrue(e.getCause() instanceof ClassNotFoundException);
    }
}
