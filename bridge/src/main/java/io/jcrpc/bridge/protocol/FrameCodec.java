package io.jcrpc.bridge.protocol;

import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;

/**
 * Length-prefixed binary frame codec.
 * Frame format: [2B big-endian length][payload bytes]
 */
public final class FrameCodec {
    private FrameCodec() {}

    public static final int MAX_FRAME = 65535;

    /**
     * Read a frame from the input stream.
     * @return payload bytes, or null if stream closed
     */
    public static byte[] readFrame(DataInputStream in) throws IOException {
        int len;
        try {
            len = in.readUnsignedShort();
        } catch (java.io.EOFException e) {
            return null; // connection closed
        }
        if (len == 0 || len > MAX_FRAME) {
            throw new IOException("Invalid frame length: " + len);
        }
        byte[] payload = new byte[len];
        in.readFully(payload);
        return payload;
    }

    /**
     * Write a frame to the output stream.
     */
    public static void writeFrame(DataOutputStream out, byte[] payload) throws IOException {
        if (payload.length > MAX_FRAME) {
            throw new IOException("Frame too large: " + payload.length);
        }
        out.writeShort(payload.length);
        out.write(payload);
        out.flush();
    }
}
