package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import com.licel.jcardsim.smartcardio.CardSimulator;

/** Provider with only a parameterised constructor — cannot be loaded by name. */
public class NoNoArgCtorProvider implements CardProvider {
    public NoNoArgCtorProvider(String issuerKeyRef) {}

    @Override
    public CardSimulator create() {
        return new CardSimulator();
    }
}
