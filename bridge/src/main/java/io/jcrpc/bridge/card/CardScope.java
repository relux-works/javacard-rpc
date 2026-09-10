package io.jcrpc.bridge.card;

import java.util.Locale;

/**
 * How cards are mapped onto TCP connections.
 */
public enum CardScope {
    /** A fresh card per TCP connection (default, fully isolated). */
    CONNECTION,
    /** One card created at server start, shared by every connection under a single lock. */
    SHARED;

    public static CardScope parse(String value) throws CardProviderStartupException {
        if (value == null) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.INVALID_SCOPE, "--card-scope requires a value");
        }
        switch (value.toLowerCase(Locale.ROOT)) {
            case "connection":
                return CONNECTION;
            case "shared":
                return SHARED;
            default:
                throw new CardProviderStartupException(
                        CardProviderStartupException.Reason.INVALID_SCOPE,
                        "unknown --card-scope '" + value + "' (expected connection|shared)");
        }
    }

    public String cliName() {
        return name().toLowerCase(Locale.ROOT);
    }
}
