package io.jcrpc.bridge.server;

import io.jcrpc.bridge.config.AppletConfig;
import io.jcrpc.bridge.protocol.FrameCodec;
import io.jcrpc.bridge.protocol.MessageType;
import io.jcrpc.bridge.session.SimulatorSession;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.List;

/**
 * Handles a single TCP client connection.
 * Runs in a dedicated thread. Creates a fresh SimulatorSession on connect.
 */
public final class ClientHandler implements Runnable {
    private final Socket socket;
    private final List<AppletConfig> applets;
    private final int clientId;

    public ClientHandler(Socket socket, List<AppletConfig> applets, int clientId) {
        this.socket = socket;
        this.applets = applets;
        this.clientId = clientId;
    }

    @Override
    public void run() {
        String clientAddr = socket.getRemoteSocketAddress().toString();
        System.out.println("[client-" + clientId + "] connected: " + clientAddr);

        try (DataInputStream in = new DataInputStream(new BufferedInputStream(socket.getInputStream()));
             DataOutputStream out = new DataOutputStream(new BufferedOutputStream(socket.getOutputStream()))) {

            SimulatorSession session = new SimulatorSession(applets);

            while (!socket.isClosed()) {
                byte[] frame = FrameCodec.readFrame(in);
                if (frame == null) break; // client disconnected

                if (frame.length < 1) {
                    sendError(out, MessageType.ERR_MALFORMED, "Empty payload");
                    continue;
                }

                byte msgType = frame[0];
                byte[] body = frame.length > 1 ? Arrays.copyOfRange(frame, 1, frame.length) : new byte[0];

                try {
                    switch (msgType) {
                        case MessageType.APDU:
                            handleAPDU(out, session, body);
                            break;
                        case MessageType.RESET:
                            session.reset();
                            sendOK(out);
                            break;
                        case MessageType.GET_ATR:
                            handleGetATR(out, session);
                            break;
                        case MessageType.PING:
                            sendOK(out);
                            break;
                        default:
                            sendError(out, MessageType.ERR_UNKNOWN_MSG,
                                    "Unknown message type: " + String.format("0x%02X", msgType));
                    }
                } catch (Exception e) {
                    sendError(out, MessageType.ERR_SIM_ERROR, e.getMessage());
                }
            }
        } catch (IOException e) {
            if (!socket.isClosed()) {
                System.err.println("[client-" + clientId + "] IO error: " + e.getMessage());
            }
        } finally {
            try { socket.close(); } catch (IOException ignored) {}
            System.out.println("[client-" + clientId + "] disconnected");
        }
    }

    private void handleAPDU(DataOutputStream out, SimulatorSession session, byte[] capdu) throws IOException {
        byte[] rapdu = session.transmitAPDU(capdu);
        byte[] resp = new byte[1 + rapdu.length];
        resp[0] = MessageType.APDU_RESPONSE;
        System.arraycopy(rapdu, 0, resp, 1, rapdu.length);
        FrameCodec.writeFrame(out, resp);
    }

    private void handleGetATR(DataOutputStream out, SimulatorSession session) throws IOException {
        byte[] atr = session.getATR();
        byte[] resp = new byte[1 + atr.length];
        resp[0] = MessageType.ATR;
        System.arraycopy(atr, 0, resp, 1, atr.length);
        FrameCodec.writeFrame(out, resp);
    }

    private void sendOK(DataOutputStream out) throws IOException {
        FrameCodec.writeFrame(out, new byte[]{MessageType.OK});
    }

    private void sendError(DataOutputStream out, byte errorCode, String message) throws IOException {
        byte[] msgBytes = message != null ? message.getBytes(StandardCharsets.UTF_8) : new byte[0];
        byte[] resp = new byte[1 + 1 + 2 + msgBytes.length];
        resp[0] = MessageType.ERROR;
        resp[1] = errorCode;
        resp[2] = (byte) (msgBytes.length >> 8);
        resp[3] = (byte) (msgBytes.length & 0xFF);
        System.arraycopy(msgBytes, 0, resp, 4, msgBytes.length);
        FrameCodec.writeFrame(out, resp);
    }
}
