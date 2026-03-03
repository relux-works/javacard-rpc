package io.jcrpc.bridge.server;

import io.jcrpc.bridge.config.BridgeConfig;

import java.io.IOException;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * TCP bridge server — accepts connections and spawns ClientHandler threads.
 * Each connection gets its own jCardSim session (virtual card).
 */
public final class TcpBridgeServer {
    private final BridgeConfig config;
    private final AtomicInteger clientCounter = new AtomicInteger(0);
    private volatile boolean running = true;

    public TcpBridgeServer(BridgeConfig config) {
        this.config = config;
    }

    public void start() throws IOException {
        ServerSocket serverSocket = new ServerSocket(
                config.getPort(), 50, InetAddress.getByName(config.getHost()));
        System.out.println("[bridge] listening on " + config.getHost() + ":" + config.getPort());
        System.out.println("[bridge] applets: " + config.getApplets().size());

        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            running = false;
            try { serverSocket.close(); } catch (IOException ignored) {}
            System.out.println("[bridge] shutdown");
        }));

        while (running) {
            try {
                Socket client = serverSocket.accept();
                int id = clientCounter.incrementAndGet();
                Thread handler = new Thread(new ClientHandler(client, config.getApplets(), id));
                handler.setDaemon(true);
                handler.setName("client-" + id);
                handler.start();
            } catch (IOException e) {
                if (running) {
                    System.err.println("[bridge] accept error: " + e.getMessage());
                }
            }
        }
    }
}
