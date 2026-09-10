package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import io.jcrpc.bridge.card.CardProviderLoader;
import io.jcrpc.bridge.card.CardProviderStartupException;
import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.config.AppletConfig;
import io.jcrpc.bridge.config.BridgeConfig;
import io.jcrpc.bridge.server.TcpBridgeServer;

import java.io.BufferedReader;
import java.io.FileReader;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

/**
 * javacard-rpc TCP Bridge — entry point.
 *
 * Usage:
 *   java -jar bridge.jar [--config applets.properties] [--host 127.0.0.1] [--port 9025]
 *                        [--card-provider fqcn] [--card-scope connection|shared]
 *
 * Config file format (one applet per line):
 *   applet.1.aid=F000000101
 *   applet.1.class=io.jcrpc.counter.example.CounterApplet
 *   applet.1.c9=
 *
 *   applet.2.aid=...
 *   applet.2.class=...
 *
 * --card-provider names a class implementing io.jcrpc.bridge.card.CardProvider
 * with a public no-arg constructor. Without it, a single ServiceLoader-registered
 * provider is used, else the default provider installs the --config applets.
 * --card-scope connection (default) = fresh card per TCP connection;
 * shared = one card created at start, every connection talks to it under a lock.
 *
 * A provider that is missing, not a CardProvider, has no no-arg constructor,
 * throws, or an invalid scope is a typed startup refusal: exit code 2, nothing bound.
 */
public final class Main {
    public static final int EXIT_STARTUP_REFUSED = 2;

    public static void main(String[] args) throws IOException {
        BridgeConfig config;
        try {
            config = configure(args);
        } catch (CardProviderStartupException e) {
            System.err.println("[bridge] startup refused: " + e.getMessage());
            System.exit(EXIT_STARTUP_REFUSED);
            return;
        }
        try {
            new TcpBridgeServer(config).start();
        } catch (CardProviderStartupException e) {
            System.err.println("[bridge] startup refused: " + e.getMessage());
            System.exit(EXIT_STARTUP_REFUSED);
        }
    }

    /**
     * Parse CLI args into a BridgeConfig. Provider/scope problems surface here as
     * typed refusals; the provider's create() itself is validated by
     * TcpBridgeServer.start() before the port is bound.
     */
    public static BridgeConfig configure(String[] args) throws IOException, CardProviderStartupException {
        String host = "127.0.0.1";
        int port = 9025;
        String configFile = null;
        String providerClass = null;
        String scopeArg = null;

        for (int i = 0; i < args.length; i++) {
            switch (args[i]) {
                case "--host":
                    host = args[++i];
                    break;
                case "--port":
                    port = Integer.parseInt(args[++i]);
                    break;
                case "--config":
                    configFile = args[++i];
                    break;
                case "--card-provider":
                    providerClass = args[++i];
                    break;
                case "--card-scope":
                    scopeArg = args[++i];
                    break;
            }
        }

        List<AppletConfig> applets;
        if (configFile != null) {
            applets = loadConfig(configFile);
        } else {
            System.out.println("[bridge] no --config specified, starting with no applets");
            applets = new ArrayList<>();
        }

        CardScope scope = scopeArg == null ? CardScope.CONNECTION : CardScope.parse(scopeArg);
        CardProvider provider = CardProviderLoader.resolve(providerClass, applets);
        return new BridgeConfig(host, port, applets, provider, scope);
    }

    private static List<AppletConfig> loadConfig(String path) throws IOException {
        java.util.Properties props = new java.util.Properties();
        try (BufferedReader reader = new BufferedReader(new FileReader(path))) {
            props.load(reader);
        }

        List<AppletConfig> applets = new ArrayList<>();
        for (int i = 1; ; i++) {
            String aid = props.getProperty("applet." + i + ".aid");
            String cls = props.getProperty("applet." + i + ".class");
            if (aid == null || cls == null) break;
            String c9 = props.getProperty("applet." + i + ".c9", "");
            applets.add(new AppletConfig(aid, cls, c9));
        }
        return applets;
    }
}
