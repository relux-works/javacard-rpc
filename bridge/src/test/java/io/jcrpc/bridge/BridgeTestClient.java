package io.jcrpc.bridge;

import io.jcrpc.bridge.protocol.FrameCodec;
import io.jcrpc.bridge.protocol.MessageType;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.net.Socket;
import java.util.Arrays;

/** Raw wire-protocol client: one TCP connection to the bridge. */
final class BridgeTestClient implements AutoCloseable {
    static final byte[] AID = {(byte) 0xF0, 0x00, 0x00, 0x01, 0x01};

    private final Socket socket;
    private final DataInputStream in;
    private final DataOutputStream out;

    BridgeTestClient(int port) throws IOException {
        socket = new Socket("127.0.0.1", port);
        in = new DataInputStream(new BufferedInputStream(socket.getInputStream()));
        out = new DataOutputStream(new BufferedOutputStream(socket.getOutputStream()));
    }

    byte[] send(byte type, byte[] body) throws IOException {
        byte[] frame = new byte[1 + body.length];
        frame[0] = type;
        System.arraycopy(body, 0, frame, 1, body.length);
        FrameCodec.writeFrame(out, frame);
        byte[] resp = FrameCodec.readFrame(in);
        if (resp == null) throw new IOException("bridge closed connection");
        return resp;
    }

    /** Send APDU, return R-APDU bytes (without the APDU_RESPONSE tag). */
    byte[] apdu(byte[] capdu) throws IOException {
        byte[] resp = send(MessageType.APDU, capdu);
        if (resp[0] != MessageType.APDU_RESPONSE) {
            throw new IOException("unexpected response " + Arrays.toString(resp));
        }
        return Arrays.copyOfRange(resp, 1, resp.length);
    }

    byte[] select() throws IOException {
        byte[] c = new byte[5 + AID.length];
        c[0] = 0x00; c[1] = (byte) 0xA4; c[2] = 0x04; c[3] = 0x00; c[4] = (byte) AID.length;
        System.arraycopy(AID, 0, c, 5, AID.length);
        return apdu(c);
    }

    byte[] increment() throws IOException {
        return apdu(new byte[]{TestCounterApplet.CLA, TestCounterApplet.INS_INC, 0, 0});
    }

    /** READ; returns the full R-APDU (data + SW). */
    byte[] read() throws IOException {
        return apdu(new byte[]{TestCounterApplet.CLA, TestCounterApplet.INS_READ, 0, 0, 0x02});
    }

    byte[] reset() throws IOException {
        return send(MessageType.RESET, new byte[0]);
    }

    byte[] atr() throws IOException {
        byte[] resp = send(MessageType.GET_ATR, new byte[0]);
        return Arrays.copyOfRange(resp, 1, resp.length);
    }

    static int sw(byte[] rapdu) {
        return ((rapdu[rapdu.length - 2] & 0xFF) << 8) | (rapdu[rapdu.length - 1] & 0xFF);
    }

    static int counter(byte[] readRapdu) {
        if (readRapdu.length != 4) throw new AssertionError("expected 2 data bytes + SW, got " + Arrays.toString(readRapdu));
        return ((readRapdu[0] & 0xFF) << 8) | (readRapdu[1] & 0xFF);
    }

    @Override
    public void close() throws IOException {
        socket.close();
    }
}
