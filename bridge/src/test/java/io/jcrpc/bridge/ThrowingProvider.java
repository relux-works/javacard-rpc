package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import com.licel.jcardsim.smartcardio.CardSimulator;

/** Provider whose create() throws — must be a startup refusal, not a hang. */
public class ThrowingProvider implements CardProvider {
    @Override
    public CardSimulator create() {
        throw new IllegalStateException("personalization failed");
    }
}
