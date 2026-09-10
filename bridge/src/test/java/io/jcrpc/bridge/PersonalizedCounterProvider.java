package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import com.licel.jcardsim.smartcardio.CardSimulator;
import com.licel.jcardsim.utils.AIDUtil;
import javacard.framework.AID;

import javax.smartcardio.CommandAPDU;

/**
 * Consumer-style test provider: installs the counter applet and
 * pre-personalizes it to PERSONALIZED_COUNT increments, then resets the card
 * so no applet is selected when the bridge hands it out.
 */
public class PersonalizedCounterProvider implements CardProvider {
    public static final int PERSONALIZED_COUNT = 5;
    static final String AID_HEX = "F000000101";

    @Override
    public CardSimulator create() {
        CardSimulator sim = new CardSimulator();
        AID aid = AIDUtil.create(AID_HEX);
        sim.installApplet(aid, TestCounterApplet.class);
        sim.selectApplet(aid);
        for (int i = 0; i < PERSONALIZED_COUNT; i++) {
            sim.transmitCommand(new CommandAPDU(TestCounterApplet.CLA, TestCounterApplet.INS_INC, 0, 0));
        }
        sim.reset();
        return sim;
    }
}
