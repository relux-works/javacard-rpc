package io.jcrpc.bridge.config;

/**
 * Configuration for a single applet to install in each session.
 */
public final class AppletConfig {
    private final String aidHex;
    private final String className;
    private final String c9Hex;

    public AppletConfig(String aidHex, String className, String c9Hex) {
        this.aidHex = aidHex;
        this.className = className;
        this.c9Hex = c9Hex != null ? c9Hex : "";
    }

    public AppletConfig(String aidHex, String className) {
        this(aidHex, className, "");
    }

    public String getAidHex() { return aidHex; }
    public String getClassName() { return className; }
    public String getC9Hex() { return c9Hex; }

    public byte[] getAidBytes() { return hexToBytes(aidHex); }
    public byte[] getC9Bytes() { return c9Hex.isEmpty() ? new byte[0] : hexToBytes(c9Hex); }

    /**
     * Build install params in JC standard format:
     * [instanceAidLen][instanceAidBytes][privilegesLen][c9Len][c9Data]
     */
    public byte[] buildInstallParams() {
        byte[] aidBytes = getAidBytes();
        byte[] c9 = getC9Bytes();
        byte[] buf = new byte[1 + aidBytes.length + 1 + 1 + c9.length];
        int off = 0;
        buf[off++] = (byte) aidBytes.length;
        System.arraycopy(aidBytes, 0, buf, off, aidBytes.length);
        off += aidBytes.length;
        buf[off++] = 0; // no privileges
        buf[off++] = (byte) c9.length;
        if (c9.length > 0) {
            System.arraycopy(c9, 0, buf, off, c9.length);
        }
        return buf;
    }

    private static byte[] hexToBytes(String hex) {
        int len = hex.length();
        byte[] data = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            data[i / 2] = (byte) ((Character.digit(hex.charAt(i), 16) << 4)
                    + Character.digit(hex.charAt(i + 1), 16));
        }
        return data;
    }
}
