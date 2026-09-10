package io.jcrpc.bridge.card;

/**
 * Typed startup refusal: the bridge cannot build a card and will not listen.
 * Carries a machine-readable {@link Reason} so tests and operators can tell
 * the refusals apart.
 */
public final class CardProviderStartupException extends Exception {
    private static final long serialVersionUID = 1L;

    public enum Reason {
        /** --card-provider class is not on the classpath. */
        PROVIDER_NOT_FOUND,
        /** Class exists but does not implement CardProvider. */
        PROVIDER_NOT_A_CARD_PROVIDER,
        /** Class has no public no-arg constructor. */
        PROVIDER_NO_NOARG_CTOR,
        /** Constructor threw or the class could not be instantiated. */
        PROVIDER_INSTANTIATION_FAILED,
        /** ServiceLoader found more than one provider and none was named explicitly. */
        PROVIDER_AMBIGUOUS,
        /** CardProvider.create() threw or returned null during startup validation. */
        CARD_CREATE_FAILED,
        /** --card-scope value not in connection|shared. */
        INVALID_SCOPE
    }

    private final Reason reason;

    public CardProviderStartupException(Reason reason, String message) {
        this(reason, message, null);
    }

    public CardProviderStartupException(Reason reason, String message, Throwable cause) {
        super(reason + ": " + message, cause);
        this.reason = reason;
    }

    public Reason getReason() { return reason; }
}
