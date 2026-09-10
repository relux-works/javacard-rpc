package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import com.licel.jcardsim.smartcardio.CardSimulator;

/** No-arg constructor exists but is private — not loadable by name. */
public class PrivateCtorProvider implements CardProvider {
    private PrivateCtorProvider() {}

    @Override
    public CardSimulator create() {
        return new CardSimulator();
    }
}
