package io.jcrpc.bridge;

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
 *   java -jar bridge.jar --config applets.properties [--host 127.0.0.1] [--port 9025]
 *
 * Config file format (one applet per line):
 *   applet.1.aid=F000000101
 *   applet.1.class=io.jcrpc.counter.example.CounterApplet
 *   applet.1.c9=
 *
 *   applet.2.aid=...
 *   applet.2.class=...
 */
public final class Main {

    public static void main(String[] args) throws IOException {
        String host = "127.0.0.1";
        int port = 9025;
        String configFile = null;

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
            }
        }

        List<AppletConfig> applets;
        if (configFile != null) {
            applets = loadConfig(configFile);
        } else {
            System.out.println("[bridge] no --config specified, starting with no applets");
            applets = new ArrayList<>();
        }

        BridgeConfig config = new BridgeConfig(host, port, applets);
        new TcpBridgeServer(config).start();
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
