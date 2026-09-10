package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardScope;
import org.junit.jupiter.api.Test;

import java.util.Collections;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * --card-scope semantics driven through TcpBridgeServer.start() → ClientHandler over real TCP.
 * Production call sites: TcpBridgeServer.prepareSessions(), ClientHandler.run().
 */
class CardScopeTest {
    private static final int P = PersonalizedCounterProvider.PERSONALIZED_COUNT;

    // Proves: under shared scope, applet state written by connection A is visible to a
    // later connection B, and the provider's pre-personalization is visible to both.
    @Test
    void sharedScope_persistsStateAcrossTwoTcpConnections() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.SHARED, Collections.emptyList())) {
            try (BridgeTestClient a = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(a.select()));
                assertEquals(P, BridgeTestClient.counter(a.read()));
                a.increment();
                a.increment();
                assertEquals(P + 2, BridgeTestClient.counter(a.read()));
            }
            try (BridgeTestClient b = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(b.select()));
                assertEquals(P + 2, BridgeTestClient.counter(b.read()));
            }
        }
    }

    // Proves: under connection scope, each connection gets its own card — B does not
    // see A's increments, only the provider's pre-personalization baseline.
    @Test
    void connectionScope_isolatesStateBetweenConnections() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.CONNECTION, Collections.emptyList())) {
            try (BridgeTestClient a = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(a.select()));
                a.increment();
                a.increment();
                assertEquals(P + 2, BridgeTestClient.counter(a.read()));
            }
            try (BridgeTestClient b = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(b.select()));
                assertEquals(P, BridgeTestClient.counter(b.read()));
            }
            // and the third connection (past the startup-warmed card) is also fresh
            try (BridgeTestClient c = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(c.select()));
                assertEquals(P, BridgeTestClient.counter(c.read()));
            }
        }
    }

    // Negative: connection scope must NOT share state even when B connects while A is still open.
    @Test
    void connectionScope_concurrentConnectionsDoNotShareState() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.CONNECTION, Collections.emptyList());
             BridgeTestClient a = h.connect();
             BridgeTestClient b = h.connect()) {
            a.select();
            b.select();
            a.increment();
            assertEquals(P + 1, BridgeTestClient.counter(a.read()));
            assertEquals(P, BridgeTestClient.counter(b.read()));
        }
    }

    // Negative: under shared scope a disconnect is NOT an implicit RESET — B still finds the
    // applet selected by A (selection is card state on the single shared card).
    @Test
    void sharedScope_disconnectDoesNotResetSharedCard() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.SHARED, Collections.emptyList())) {
            try (BridgeTestClient a = h.connect()) {
                assertEquals(0x9000, BridgeTestClient.sw(a.select()));
                a.increment();
            }
            try (BridgeTestClient b = h.connect()) {
                // no SELECT on B: only works if A's selection survived A's disconnect
                byte[] r = b.read();
                assertEquals(0x9000, BridgeTestClient.sw(r));
                assertEquals(P + 1, BridgeTestClient.counter(r));
            }
        }
    }

    // Documented RESET semantics under shared scope: an explicit RESET frame from any
    // connection resets the one shared card — selection is cleared for everyone,
    // persistent applet state survives.
    @Test
    void sharedScope_explicitResetFrameIsVisibleToOtherConnection() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.SHARED, Collections.emptyList());
             BridgeTestClient a = h.connect();
             BridgeTestClient b = h.connect()) {
            assertEquals(0x9000, BridgeTestClient.sw(a.select()));
            a.increment();
            assertEquals(P + 1, BridgeTestClient.counter(b.read()));

            assertEquals((byte) 0x82, b.reset()[0]);

            // A's selection is gone: READ without SELECT is no longer answered by the applet
            assertNotEquals(0x9000, BridgeTestClient.sw(a.read()));
            // persistent state survived the reset
            assertEquals(0x9000, BridgeTestClient.sw(a.select()));
            assertEquals(P + 1, BridgeTestClient.counter(a.read()));
        }
    }

    // Negative: under connection scope a RESET on one connection must not touch another card.
    @Test
    void connectionScope_resetDoesNotLeakIntoOtherConnection() throws Exception {
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.CONNECTION, Collections.emptyList());
             BridgeTestClient a = h.connect();
             BridgeTestClient b = h.connect()) {
            a.select();
            b.select();
            b.reset();
            assertEquals(0x9000, BridgeTestClient.sw(a.read()));
            assertNotEquals(0x9000, BridgeTestClient.sw(b.read()));
        }
    }

    // Proves: concurrent connections under shared scope are serialized by the single lock —
    // N clients × M increments land exactly N*M times on the shared card.
    @Test
    void sharedScope_concurrentIncrementsAreSerialized() throws Exception {
        final int clients = 8, perClient = 50;
        try (BridgeHarness h = new BridgeHarness(new PersonalizedCounterProvider(), CardScope.SHARED, Collections.emptyList())) {
            try (BridgeTestClient s = h.connect()) { s.select(); }
            ExecutorService pool = Executors.newFixedThreadPool(clients);
            CountDownLatch go = new CountDownLatch(1);
            Future<?>[] futures = new Future<?>[clients];
            for (int i = 0; i < clients; i++) {
                futures[i] = pool.submit(() -> {
                    try (BridgeTestClient c = h.connect()) {
                        go.await();
                        for (int k = 0; k < perClient; k++) {
                            assertEquals(0x9000, BridgeTestClient.sw(c.increment()));
                        }
                    }
                    return null;
                });
            }
            go.countDown();
            for (Future<?> f : futures) f.get(60, TimeUnit.SECONDS);
            pool.shutdown();
            try (BridgeTestClient r = h.connect()) {
                assertEquals(P + clients * perClient, BridgeTestClient.counter(r.read()));
            }
        }
    }

    // Proves the single lock deterministically: while connection A's APDU is parked inside the
    // shared card, connection B's APDU must not enter the card (max concurrent entries == 1).
    // Fails if SimulatorSession.transmitAPDU loses its synchronization.
    @Test
    void sharedScope_apduFromSecondConnectionWaitsForLock() throws Exception {
        BlockingCardProvider provider = new BlockingCardProvider();
        try (BridgeHarness h = new BridgeHarness(provider, CardScope.SHARED, Collections.emptyList());
             BridgeTestClient a = h.connect();
             BridgeTestClient b = h.connect()) {
            ExecutorService pool = Executors.newFixedThreadPool(2);
            Future<byte[]> fa = pool.submit(() -> a.increment());
            assertTrue(provider.firstEntered.await(10, TimeUnit.SECONDS), "A never entered the card");
            Future<byte[]> fb = pool.submit(() -> b.increment());
            // give B every chance to (wrongly) enter the card while A is parked
            Thread.sleep(300);
            assertEquals(1, provider.inside.get(), "B entered the shared card while A was inside");
            assertEquals(1, provider.maxInside.get());
            provider.release.countDown();
            assertEquals(0x9000, BridgeTestClient.sw(fa.get(10, TimeUnit.SECONDS)));
            assertEquals(0x9000, BridgeTestClient.sw(fb.get(10, TimeUnit.SECONDS)));
            assertEquals(2, provider.calls.get());
            assertEquals(1, provider.maxInside.get());
            pool.shutdown();
        }
    }
}
