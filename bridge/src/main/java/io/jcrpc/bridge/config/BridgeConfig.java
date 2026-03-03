package io.jcrpc.bridge.config;

import java.util.List;

/**
 * Bridge server configuration.
 */
public final class BridgeConfig {
    private final String host;
    private final int port;
    private final List<AppletConfig> applets;

    public BridgeConfig(String host, int port, List<AppletConfig> applets) {
        this.host = host;
        this.port = port;
        this.applets = applets;
    }

    public String getHost() { return host; }
    public int getPort() { return port; }
    public List<AppletConfig> getApplets() { return applets; }
}
