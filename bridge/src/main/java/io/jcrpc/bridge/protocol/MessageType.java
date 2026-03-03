package io.jcrpc.bridge.protocol;

/**
 * Wire message types for javacard-rpc TCP bridge protocol.
 */
public final class MessageType {
    private MessageType() {}

    // Request types (client → server)
    public static final byte APDU     = (byte) 0x01;
    public static final byte RESET    = (byte) 0x02;
    public static final byte GET_ATR  = (byte) 0x03;
    public static final byte PING     = (byte) 0x04;

    // Response types (server → client)
    public static final byte APDU_RESPONSE = (byte) 0x81;
    public static final byte OK            = (byte) 0x82;
    public static final byte ATR           = (byte) 0x83;
    public static final byte ERROR         = (byte) 0xE0;

    // Error codes
    public static final byte ERR_MALFORMED   = (byte) 0x01;
    public static final byte ERR_UNKNOWN_MSG = (byte) 0x02;
    public static final byte ERR_SIM_ERROR   = (byte) 0x03;
}
