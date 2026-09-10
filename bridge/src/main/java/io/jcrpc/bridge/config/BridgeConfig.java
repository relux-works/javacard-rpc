package io.jcrpc.bridge.config;

import io.jcrpc.bridge.card.CardProvider;
import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.card.DefaultCardProvider;

import java.util.List;

/**
 * Bridge server configuration.
 */
public final class BridgeConfig {
    private final String host;
    private final int port;
    private final List<AppletConfig> applets;
    private final CardProvider cardProvider;
    private final CardScope cardScope;

    /** Today's behaviour: default provider over the applet list, one card per connection. */
    public BridgeConfig(String host, int port, List<AppletConfig> applets) {
        this(host, port, applets, new DefaultCardProvider(applets), CardScope.CONNECTION);
    }

    public BridgeConfig(String host, int port, List<AppletConfig> applets,
                        CardProvider cardProvider, CardScope cardScope) {
        this.host = host;
        this.port = port;
        this.applets = applets;
        this.cardProvider = cardProvider;
        this.cardScope = cardScope;
    }

    public String getHost() { return host; }
    public int getPort() { return port; }
    public List<AppletConfig> getApplets() { return applets; }
    public CardProvider getCardProvider() { return cardProvider; }
    public CardScope getCardScope() { return cardScope; }
}
