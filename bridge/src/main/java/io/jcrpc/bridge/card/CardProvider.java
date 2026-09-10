package io.jcrpc.bridge.card;

import com.licel.jcardsim.smartcardio.CardSimulator;

/**
 * Consumer SPI: builds the jCardSim card the bridge talks to.
 *
 * The bridge only ever calls {@link #create()}. Everything else — runtime choice
 * (custom SimulatorRuntime), GlobalPlatform secure-channel setup, applet install
 * and issuer personalization — is the provider's business. The bridge never
 * learns any key or authority.
 *
 * Implementations are loaded by fully qualified class name (--card-provider)
 * through a public no-arg constructor, or discovered via ServiceLoader
 * (META-INF/services/io.jcrpc.bridge.card.CardProvider).
 *
 * Scope decides how often create() runs: once per TCP connection
 * (--card-scope connection) or once at server start (--card-scope shared).
 */
public interface CardProvider {
    CardSimulator create();
}
