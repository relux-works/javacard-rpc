package io.jcrpc.bridge.server;

import io.jcrpc.bridge.card.CardProvider;
import io.jcrpc.bridge.card.CardProviderStartupException;
import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.config.BridgeConfig;
import io.jcrpc.bridge.session.SimulatorSession;
import com.licel.jcardsim.smartcardio.CardSimulator;

import java.io.IOException;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Supplier;

/**
 * TCP bridge server — accepts connections and spawns ClientHandler threads.
 *
 * connection scope: each connection gets its own card from the CardProvider.
 * shared scope:     one card is created at start and every connection talks to
 *                   it through a single locked SimulatorSession.
 *
 * In both scopes the provider is exercised once before the socket is bound, so
 * a broken provider is a typed startup refusal rather than a per-connection
 * ERR_SIM_ERROR.
 */
public final class TcpBridgeServer {
    private final BridgeConfig config;
    private final AtomicInteger clientCounter = new AtomicInteger(0);
    private volatile boolean running = true;
    private volatile ServerSocket serverSocket;
    private final CountDownLatch bound = new CountDownLatch(1);

    public TcpBridgeServer(BridgeConfig config) {
        this.config = config;
    }

    /** Blocks in the accept loop until {@link #stop()} or the JVM shutdown hook. */
    public void start() throws IOException, CardProviderStartupException {
        Supplier<SimulatorSession> sessions = prepareSessions();

        ServerSocket socket = new ServerSocket(
                config.getPort(), 50, InetAddress.getByName(config.getHost()));
        serverSocket = socket;
        bound.countDown();
        System.out.println("[bridge] listening on " + config.getHost() + ":" + socket.getLocalPort());
        System.out.println("[bridge] applets: " + config.getApplets().size());
        System.out.println("[bridge] card provider: " + config.getCardProvider().getClass().getName()
                + ", scope: " + config.getCardScope().cliName());

        Runtime.getRuntime().addShutdownHook(new Thread(this::stop));

        while (running) {
            try {
                Socket client = socket.accept();
                int id = clientCounter.incrementAndGet();
                Thread handler = new Thread(new ClientHandler(client, sessions, id));
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

    /** Local port once bound (useful with port 0). Blocks until the socket is bound. */
    public int awaitPort() throws InterruptedException {
        bound.await();
        return serverSocket.getLocalPort();
    }

    /** True once the listening socket is bound. */
    public boolean isBound() {
        return bound.getCount() == 0;
    }

    public void stop() {
        if (!running) return;
        running = false;
        ServerSocket socket = serverSocket;
        if (socket != null) {
            try { socket.close(); } catch (IOException ignored) {}
        }
        System.out.println("[bridge] shutdown");
    }

    /**
     * Validate the provider by creating the first card now. Shared scope keeps
     * that card for everyone; connection scope hands it to the first
     * connection and creates a fresh one for each later connection.
     */
    private Supplier<SimulatorSession> prepareSessions() throws CardProviderStartupException {
        CardProvider provider = config.getCardProvider();
        CardSimulator first = createCard(provider);
        if (config.getCardScope() == CardScope.SHARED) {
            SimulatorSession shared = new SimulatorSession(first);
            return () -> shared;
        }
        return new Supplier<SimulatorSession>() {
            private CardSimulator warm = first;

            @Override
            public synchronized SimulatorSession get() {
                CardSimulator sim = warm;
                if (sim != null) {
                    warm = null;
                } else {
                    sim = provider.create();
                    if (sim == null) throw new IllegalStateException("CardProvider.create() returned null");
                }
                return new SimulatorSession(sim);
            }
        };
    }

    private static CardSimulator createCard(CardProvider provider) throws CardProviderStartupException {
        CardSimulator sim;
        try {
            sim = provider.create();
        } catch (RuntimeException | Error e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.CARD_CREATE_FAILED,
                    provider.getClass().getName() + ".create() threw: " + e, e);
        }
        if (sim == null) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.CARD_CREATE_FAILED,
                    provider.getClass().getName() + ".create() returned null");
        }
        return sim;
    }
}
