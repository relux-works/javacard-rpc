package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import io.jcrpc.bridge.card.CardProviderStartupException;
import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.config.AppletConfig;
import io.jcrpc.bridge.config.BridgeConfig;
import io.jcrpc.bridge.server.TcpBridgeServer;

import java.util.List;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

/** Runs TcpBridgeServer.start() (the production entry point) on a background thread on an ephemeral port. */
final class BridgeHarness implements AutoCloseable {
    final TcpBridgeServer server;
    final int port;
    private final Thread thread;
    private final AtomicReference<Throwable> failure = new AtomicReference<>();

    BridgeHarness(CardProvider provider, CardScope scope, List<AppletConfig> applets) throws Exception {
        this(new BridgeConfig("127.0.0.1", 0, applets, provider, scope));
    }

    BridgeHarness(BridgeConfig config) throws Exception {
        server = new TcpBridgeServer(config);
        thread = new Thread(() -> {
            try {
                server.start();
            } catch (Throwable t) {
                failure.set(t);
            }
        }, "bridge-under-test");
        thread.setDaemon(true);
        thread.start();
        // Either the socket binds or start() refuses.
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (thread.isAlive() && failure.get() == null && !server.isBound()) {
            if (System.nanoTime() > deadline) throw new AssertionError("bridge did not bind or refuse within 10s");
            Thread.sleep(5);
        }
        Throwable t = failure.get();
        if (t instanceof Exception) throw (Exception) t;
        if (t != null) throw new AssertionError(t);
        port = server.awaitPort();
    }

    BridgeTestClient connect() throws Exception {
        return new BridgeTestClient(port);
    }

    @Override
    public void close() {
        server.stop();
    }
}
