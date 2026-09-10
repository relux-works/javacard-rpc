package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProviderLoader;
import io.jcrpc.bridge.card.CardProviderStartupException;
import io.jcrpc.bridge.card.CardProviderStartupException.Reason;
import io.jcrpc.bridge.card.CardScope;
import io.jcrpc.bridge.card.DefaultCardProvider;
import io.jcrpc.bridge.config.BridgeConfig;
import io.jcrpc.bridge.server.TcpBridgeServer;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.io.IOException;
import java.net.ConnectException;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Provider/scope problems are typed startup refusals surfaced by the production
 * entry points Main.configure(args) and TcpBridgeServer.start(); nothing binds, nothing hangs.
 */
class StartupRefusalTest {

    private static CardProviderStartupException refuse(String... args) {
        return assertThrows(CardProviderStartupException.class, () -> Main.configure(args));
    }

    // Missing class → PROVIDER_NOT_FOUND
    @Test
    void missingProviderClassIsRefused() {
        assertEquals(Reason.PROVIDER_NOT_FOUND, refuse("--card-provider", "io.jcrpc.nope.Missing").getReason());
    }

    // Class without public no-arg ctor → PROVIDER_NO_NOARG_CTOR
    @Test
    void providerWithoutNoArgCtorIsRefused() {
        assertEquals(Reason.PROVIDER_NO_NOARG_CTOR,
                refuse("--card-provider", NoNoArgCtorProvider.class.getName()).getReason());
    }

    // Narrow: a no-arg ctor that is not public is also PROVIDER_NO_NOARG_CTOR (no setAccessible).
    @Test
    void providerWithPrivateNoArgCtorIsRefused() {
        assertEquals(Reason.PROVIDER_NO_NOARG_CTOR,
                refuse("--card-provider", PrivateCtorProvider.class.getName()).getReason());
    }

    // Class that exists, has a ctor, but is not a CardProvider → PROVIDER_NOT_A_CARD_PROVIDER
    @Test
    void nonProviderClassIsRefused() {
        assertEquals(Reason.PROVIDER_NOT_A_CARD_PROVIDER,
                refuse("--card-provider", NotAProvider.class.getName()).getReason());
    }

    // Invalid scope → INVALID_SCOPE (both an unknown word and a missing value)
    @Test
    void invalidScopeIsRefused() {
        assertEquals(Reason.INVALID_SCOPE, refuse("--card-scope", "global").getReason());
        assertEquals(Reason.INVALID_SCOPE, assertThrows(CardProviderStartupException.class,
                () -> CardScope.parse(null)).getReason());
    }

    // Positive controls: valid provider / scope are accepted by the same parser.
    @Test
    void validProviderAndScopeAreAccepted() throws Exception {
        BridgeConfig c = Main.configure(new String[]{
                "--card-provider", PersonalizedCounterProvider.class.getName(), "--card-scope", "shared"});
        assertEquals(PersonalizedCounterProvider.class, c.getCardProvider().getClass());
        assertEquals(CardScope.SHARED, c.getCardScope());
        BridgeConfig d = Main.configure(new String[]{"--card-scope", "connection"});
        assertEquals(DefaultCardProvider.class, d.getCardProvider().getClass());
        assertEquals(CardScope.CONNECTION, d.getCardScope());
    }

    // Throwing create() → CARD_CREATE_FAILED from start(), and the port is never bound,
    // in both scopes.
    @Test
    void throwingProviderIsRefusedBeforeBind() throws Exception {
        for (CardScope scope : CardScope.values()) {
            int port = freePort();
            TcpBridgeServer server = new TcpBridgeServer(new BridgeConfig(
                    "127.0.0.1", port, Collections.emptyList(), new ThrowingProvider(), scope));
            CardProviderStartupException e = startExpectingRefusal(server, scope);
            assertEquals(Reason.CARD_CREATE_FAILED, e.getReason(), scope.toString());
            assertTrue(e.getMessage().contains("personalization failed"));
            assertThrows(ConnectException.class, () -> new Socket("127.0.0.1", port).close(), scope.toString());
        }
    }

    /** Runs start() on a helper thread: a refusal must surface within 10s; binding instead is a failure, not a hang. */
    private static CardProviderStartupException startExpectingRefusal(TcpBridgeServer server, CardScope scope) throws Exception {
        java.util.concurrent.atomic.AtomicReference<Throwable> thrown = new java.util.concurrent.atomic.AtomicReference<>();
        Thread t = new Thread(() -> { try { server.start(); } catch (Throwable x) { thrown.set(x); } });
        t.setDaemon(true);
        t.start();
        t.join(10_000);
        if (server.isBound()) {
            server.stop();
            throw new AssertionError("bridge bound its port under " + scope + " instead of refusing");
        }
        assertTrue(!t.isAlive(), "start() neither refused nor bound within 10s under " + scope);
        Throwable x = thrown.get();
        assertTrue(x instanceof CardProviderStartupException, "unexpected: " + x);
        return (CardProviderStartupException) x;
    }

    // create() returning null is the same refusal class, not a later NPE per connection.
    @Test
    void nullCardIsRefusedBeforeBind() {
        TcpBridgeServer server = new TcpBridgeServer(new BridgeConfig(
                "127.0.0.1", 0, Collections.emptyList(), () -> null, CardScope.SHARED));
        assertEquals(Reason.CARD_CREATE_FAILED,
                assertThrows(CardProviderStartupException.class, server::start).getReason());
    }

    // Loader-level: constructor that throws is PROVIDER_INSTANTIATION_FAILED, with the cause kept.
    @Test
    void throwingConstructorIsRefused() {
        CardProviderStartupException e = assertThrows(CardProviderStartupException.class,
                () -> CardProviderLoader.loadByName(ThrowingCtorProvider.class.getName()));
        assertEquals(Reason.PROVIDER_INSTANTIATION_FAILED, e.getReason());
        assertTrue(e.getCause() instanceof IllegalArgumentException);
    }

    public static class ThrowingCtorProvider implements io.jcrpc.bridge.card.CardProvider {
        public ThrowingCtorProvider() { throw new IllegalArgumentException("no issuer key"); }
        @Override public com.licel.jcardsim.smartcardio.CardSimulator create() { return null; }
    }

    // Real process: `java io.jcrpc.bridge.Main --card-provider <throwing>` exits 2 with the
    // typed reason on stderr and does not hang (the production main()).
    @Test
    void mainProcessExitsWithCode2OnRefusal() throws Exception {
        String[][] cases = {
                {"io.jcrpc.nope.Missing", "PROVIDER_NOT_FOUND"},
                {NoNoArgCtorProvider.class.getName(), "PROVIDER_NO_NOARG_CTOR"},
                {ThrowingProvider.class.getName(), "CARD_CREATE_FAILED"},
        };
        for (String[] c : cases) {
            Process p = launchMain("--port", String.valueOf(freePort()), "--card-provider", c[0]);
            assertTrue(p.waitFor(60, TimeUnit.SECONDS), "main did not exit for " + c[0]);
            String err = new String(p.getErrorStream().readAllBytes(), StandardCharsets.UTF_8);
            assertEquals(Main.EXIT_STARTUP_REFUSED, p.exitValue(), err);
            assertTrue(err.contains("[bridge] startup refused: " + c[1]), err);
        }
    }

    private static Process launchMain(String... args) throws IOException {
        List<String> cmd = new ArrayList<>();
        cmd.add(System.getProperty("java.home") + File.separator + "bin" + File.separator + "java");
        cmd.add("--add-modules"); cmd.add("java.smartcardio");
        cmd.add("-cp"); cmd.add(System.getProperty("java.class.path"));
        cmd.add(Main.class.getName());
        cmd.addAll(java.util.Arrays.asList(args));
        return new ProcessBuilder(cmd).start();
    }

    private static int freePort() throws IOException {
        try (ServerSocket s = new ServerSocket(0)) {
            return s.getLocalPort();
        }
    }

    // ServiceLoader fallback: with no --card-provider, exactly one registered provider is used;
    // two registered providers are an ambiguity refusal; none → DefaultCardProvider.
    @Test
    void serviceLoaderFallbackPicksSingleRegisteredProvider() throws Exception {
        withServices(Collections.singletonList(PersonalizedCounterProvider.class.getName()), () ->
                assertEquals(PersonalizedCounterProvider.class,
                        CardProviderLoader.resolve(null, Collections.emptyList()).getClass()));
        withServices(java.util.Arrays.asList(PersonalizedCounterProvider.class.getName(), ThrowingProvider.class.getName()), () ->
                assertEquals(Reason.PROVIDER_AMBIGUOUS, assertThrows(CardProviderStartupException.class,
                        () -> CardProviderLoader.resolve(null, Collections.emptyList())).getReason()));
        assertEquals(DefaultCardProvider.class, CardProviderLoader.resolve(null, Collections.emptyList()).getClass());
    }

    // Explicit --card-provider wins over a ServiceLoader registration.
    @Test
    void explicitProviderOverridesServiceLoader() throws Exception {
        withServices(Collections.singletonList(ThrowingProvider.class.getName()), () ->
                assertEquals(PersonalizedCounterProvider.class,
                        CardProviderLoader.resolve(PersonalizedCounterProvider.class.getName(), Collections.emptyList()).getClass()));
    }

    interface Body { void run() throws Exception; }

    /** Runs body with a thread-context classloader exposing a META-INF/services CardProvider registration. */
    private static void withServices(List<String> impls, Body body) throws Exception {
        java.nio.file.Path dir = java.nio.file.Files.createTempDirectory("jcrpc-services");
        java.nio.file.Path svc = dir.resolve("META-INF/services/" + io.jcrpc.bridge.card.CardProvider.class.getName());
        java.nio.file.Files.createDirectories(svc.getParent());
        java.nio.file.Files.write(svc, impls, StandardCharsets.UTF_8);
        ClassLoader prev = Thread.currentThread().getContextClassLoader();
        try (java.net.URLClassLoader cl = new java.net.URLClassLoader(
                new java.net.URL[]{dir.toUri().toURL()}, StartupRefusalTest.class.getClassLoader())) {
            Thread.currentThread().setContextClassLoader(cl);
            body.run();
        } finally {
            Thread.currentThread().setContextClassLoader(prev);
        }
    }
}
