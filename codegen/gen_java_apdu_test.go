package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedStreamAPDUAdapterHandlesFragmentedIncomingData(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not available")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not available")
	}
	schema, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	result, err := GenerateJavaSkeleton(schema, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	frameworkDir := filepath.Join(root, "javacard", "framework")
	securityDir := filepath.Join(root, "javacard", "security")
	serverDir := filepath.Join(root, "io", "jcrpc", "streamdemo", "server")
	if err := os.MkdirAll(frameworkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll framework: %v", err)
	}
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatalf("MkdirAll server: %v", err)
	}
	if err := os.MkdirAll(securityDir, 0o755); err != nil {
		t.Fatalf("MkdirAll security: %v", err)
	}

	files := map[string][]byte{
		filepath.Join(serverDir, result.TransportName+".java"):         result.TransportSource,
		filepath.Join(serverDir, result.SkeletonName+".java"):          result.SkeletonSource,
		filepath.Join(serverDir, result.StreamEndpointName+".java"):    result.StreamEndpointSource,
		filepath.Join(serverDir, result.StreamRuntimeName+".java"):     result.StreamRuntimeSource,
		filepath.Join(serverDir, result.StreamAPDUAdapterName+".java"): result.StreamAPDUAdapterSource,
		filepath.Join(frameworkDir, "ISO7816.java"):                    []byte(apduISO7816Stub),
		filepath.Join(frameworkDir, "ISOException.java"):               []byte(apduISOExceptionStub),
		filepath.Join(frameworkDir, "JCSystem.java"):                   []byte(apduJCSystemStub),
		filepath.Join(frameworkDir, "Applet.java"):                     []byte(apduAppletStub),
		filepath.Join(frameworkDir, "APDU.java"):                       []byte(fragmentedAPDUStub),
		filepath.Join(securityDir, "MessageDigest.java"):               []byte(apduMessageDigestStub),
		filepath.Join(serverDir, "StreamDemoApplet.java"):              []byte(streamDemoAppletFixture),
		filepath.Join(serverDir, "StreamAPDUAdapterHarness.java"):      []byte(streamAPDUAdapterHarness),
	}
	paths := make([]string, 0, len(files))
	for path, source := range files {
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		paths = append(paths, path)
	}
	args := append([]string{"-d", root}, paths...)
	if output, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, output)
	}
	command := exec.Command(java, "-cp", root, "io.jcrpc.streamdemo.server.StreamAPDUAdapterHarness")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("APDU adapter harness failed: %v\n%s", err, output)
	}
}

const apduISO7816Stub = `package javacard.framework;
public interface ISO7816 {
    short OFFSET_CLA = 0;
    short OFFSET_INS = 1;
    short OFFSET_P1 = 2;
    short OFFSET_P2 = 3;
    short OFFSET_CDATA = 5;
    short SW_CLA_NOT_SUPPORTED = (short) 0x6E00;
    short SW_WRONG_LENGTH = (short) 0x6700;
    short SW_INS_NOT_SUPPORTED = (short) 0x6D00;
}
`

const apduISOExceptionStub = `package javacard.framework;
public final class ISOException extends RuntimeException {
    public final short sw;
    private ISOException(short sw) { this.sw = sw; }
    public static void throwIt(short sw) { throw new ISOException(sw); }
}
`

const apduJCSystemStub = `package javacard.framework;
public final class JCSystem {
    public static final byte CLEAR_ON_DESELECT = 1;
    public static byte[] makeTransientByteArray(short length, byte event) {
        return new byte[length];
    }
}
`

const apduAppletStub = `package javacard.framework;
public abstract class Applet {
    protected Applet() { }
    protected final void register() { }
    public abstract void process(APDU apdu);
    public boolean selectingApplet() { return false; }
    public void deselect() { }
}
`

const apduMessageDigestStub = `package javacard.security;
public final class MessageDigest {
    public static final byte ALG_SHA_256 = 4;
    private MessageDigest() { }
    public static MessageDigest getInstance(byte algorithm, boolean externalAccess) {
        return new MessageDigest();
    }
    public short doFinal(byte[] input, short inputOffset, short inputLength,
                         byte[] output, short outputOffset) {
        try {
            java.security.MessageDigest digest = java.security.MessageDigest.getInstance("SHA-256");
            digest.update(input, inputOffset, inputLength);
            byte[] value = digest.digest();
            System.arraycopy(value, 0, output, outputOffset, value.length);
            return (short) value.length;
        } catch (Exception failure) {
            throw new RuntimeException(failure);
        }
    }
}
`

const fragmentedAPDUStub = `package javacard.framework;
import java.util.Arrays;
public final class APDU {
    private final byte[] buffer = new byte[260];
    private final byte[] payload;
    private final int[] chunks;
    private int chunkIndex;
    private int payloadOffset;
    public byte[] outgoing = new byte[0];

    public APDU(byte cla, byte ins, byte p1, byte p2, byte[] payload, int[] chunks) {
        buffer[ISO7816.OFFSET_CLA] = cla;
        buffer[ISO7816.OFFSET_INS] = ins;
        buffer[ISO7816.OFFSET_P1] = p1;
        buffer[ISO7816.OFFSET_P2] = p2;
        this.payload = payload;
        this.chunks = chunks;
    }
    public byte[] getBuffer() { return buffer; }
    public short getIncomingLength() { return (short) payload.length; }
    public short setIncomingAndReceive() { return receiveBytes(ISO7816.OFFSET_CDATA); }
    public short receiveBytes(short offset) {
        if (payloadOffset == payload.length) return 0;
        int length = Math.min(chunks[chunkIndex++], payload.length - payloadOffset);
        System.arraycopy(payload, payloadOffset, buffer, offset, length);
        payloadOffset += length;
        return (short) length;
    }
    public short setOutgoing() { return 0; }
    public void setOutgoingLength(short length) { }
    public void sendBytesLong(byte[] source, short offset, short length) {
        outgoing = Arrays.copyOfRange(source, offset, offset + length);
    }
}
`

const streamDemoAppletFixture = `package io.jcrpc.streamdemo.server;
import javacard.framework.APDU;
import javacard.framework.Applet;
import javacard.framework.ISO7816;
import javacard.framework.ISOException;

public final class StreamDemoApplet extends Applet {
    private final Logic logic;
    private final StreamDemoStreamAPDUAdapter streams;

    public static void install(byte[] installData, short offset, byte length) {
        new StreamDemoApplet().register();
    }

    StreamDemoApplet() {
        logic = new Logic();
        streams = new StreamDemoStreamAPDUAdapter(logic);
    }

    @Override
    public void process(APDU apdu) {
        if (selectingApplet()) return;
        if (streams.processIfStream(apdu)) return;
        ISOException.throwIt(ISO7816.SW_INS_NOT_SUPPORTED);
    }

    @Override
    public void deselect() {
        streams.deselect();
    }

    StreamDemoStreamAPDUAdapter streamsForTest() { return streams; }
    byte responseOnlyCallsForTest() { return logic.responseOnlyCalls; }

    private static final class Logic extends StreamDemoSkeleton {
        Logic() { super(new NoopTransport()); }

        @Override
        protected byte onGetVersion() { return (byte) 1; }

        @Override
        protected short onProcessPacketStream(byte[] input, short inputOffset, short inputLength,
                byte[] output, short outputOffset, short outputCapacity) {
            if (inputLength == 1 && input[inputOffset] == (byte) 0x7F) return (short) -1;
            for (short left = 0, right = (short) (inputLength - 1); left <= right; left++, right--) {
                byte value = input[(short) (inputOffset + left)];
                output[(short) (outputOffset + left)] = input[(short) (inputOffset + right)];
                output[(short) (outputOffset + right)] = value;
            }
            return inputLength;
        }

        @Override
        protected short onIssueReportStream(byte[] input, short inputOffset, short inputLength,
                byte[] output, short outputOffset, short outputCapacity) {
            responseOnlyCalls++;
            output[outputOffset] = (byte) 1;
            output[(short) (outputOffset + 1)] = (byte) 2;
            output[(short) (outputOffset + 2)] = (byte) 3;
            output[(short) (outputOffset + 3)] = responseOnlyCalls;
            return (short) 4;
        }

        private byte responseOnlyCalls;
    }

    private static final class NoopTransport implements StreamDemoTransport {
        @Override
        public byte[] transmit(byte ins, byte p1, byte p2, byte[] data) { return data; }
    }
}
`

const streamAPDUAdapterHarness = `package io.jcrpc.streamdemo.server;
import java.util.Arrays;
import java.lang.reflect.Field;
import java.security.MessageDigest;
import javacard.framework.APDU;
public final class StreamAPDUAdapterHarness {
    public static void main(String[] args) throws Exception {
        StreamDemoApplet applet = new StreamDemoApplet();
        StreamDemoStreamAPDUAdapter adapter = applet.streamsForTest();
        byte[] abandoned = new byte[192];
        Arrays.fill(abandoned, (byte) 7);
        APDU first = new APDU((byte) 0xB0, (byte) 0x20, (byte) 0, (byte) 2,
                abandoned, new int[]{17, 61, 114});
        applet.process(first);
        if (first.outgoing.length != 0) throw new AssertionError("write response must be empty");
        assertScratchWiped(adapter);

        applet.deselect();

        byte[] invalid = new byte[]{(byte) 0x7F};
        APDU invalidWrite = new APDU((byte) 0xB0, (byte) 0x20, (byte) 0, (byte) 1,
                invalid, new int[]{1});
        applet.process(invalidWrite);
        try {
            applet.process(new APDU((byte) 0xB0, (byte) 0x21, (byte) 0, (byte) 0,
                    closeData(invalid), new int[]{5, 29}));
            throw new AssertionError("negative handler length must fail closed");
        } catch (javacard.framework.ISOException expected) {
            if (expected.sw != (short) 0x6700) throw new AssertionError("wrong status word");
        }
        assertScratchWiped(adapter);

        byte[] payload = new byte[]{1, 2, 3, 4, 5};
        APDU restarted = new APDU((byte) 0xB0, (byte) 0x20, (byte) 0, (byte) 1,
                payload, new int[]{2, 3});
        applet.process(restarted);
        assertScratchWiped(adapter);

        APDU closeWrite = new APDU((byte) 0xB0, (byte) 0x21, (byte) 0, (byte) 0,
                closeData(payload), new int[]{1, 11, 22});
        applet.process(closeWrite);
        if (closeWrite.outgoing.length != 35) throw new AssertionError("descriptor length");
        assertScratchWiped(adapter);

        APDU read = new APDU((byte) 0xB0, (byte) 0x23, (byte) 0, (byte) 1,
                new byte[0], new int[0]);
        applet.process(read);
        byte[] expected = new byte[]{5, 4, 3, 2, 1};
        if (!Arrays.equals(expected, read.outgoing)) throw new AssertionError("generated handler result");

        APDU closeRead = new APDU((byte) 0xB0, (byte) 0x24, (byte) 0, (byte) 0,
                closeData(expected), new int[]{7, 27});
        applet.process(closeRead);
        if (closeRead.outgoing.length != 0) throw new AssertionError("close-read response");
        assertScratchWiped(adapter);

        APDU invoke = new APDU((byte) 0xB0, (byte) 0x30, (byte) 0, (byte) 0,
                new byte[0], new int[0]);
        applet.process(invoke);
        byte[] responseOnlyDescriptor = invoke.outgoing;
        if (responseOnlyDescriptor.length != 35) throw new AssertionError("response-only descriptor");
        if (applet.responseOnlyCallsForTest() != 1) throw new AssertionError("response-only handler count");

        try {
            applet.process(new APDU((byte) 0xB0, (byte) 0x30, (byte) 0, (byte) 0,
                    new byte[0], new int[0]));
            throw new AssertionError("response-only replay must fail");
        } catch (javacard.framework.ISOException expectedFailure) {
            if (expectedFailure.sw != (short) 0x6985) throw new AssertionError("wrong replay status");
        }
        if (applet.responseOnlyCallsForTest() != 1) throw new AssertionError("response-only replay executed handler");

        APDU pending = new APDU((byte) 0xB0, (byte) 0x32, (byte) 0, (byte) 0,
                new byte[0], new int[0]);
        applet.process(pending);
        if (!Arrays.equals(responseOnlyDescriptor, pending.outgoing)) {
            throw new AssertionError("response-only replay replaced descriptor");
        }
        APDU responseOnlyRead = new APDU((byte) 0xB0, (byte) 0x33, (byte) 0, (byte) 1,
                new byte[0], new int[0]);
        applet.process(responseOnlyRead);
        byte[] report = new byte[]{1, 2, 3, 1};
        if (!Arrays.equals(report, responseOnlyRead.outgoing)) throw new AssertionError("response-only result");
        applet.process(new APDU((byte) 0xB0, (byte) 0x34, (byte) 0, (byte) 0,
                closeData(report), new int[]{9, 25}));
        assertScratchWiped(adapter);
    }

    private static byte[] closeData(byte[] value) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(value);
        byte[] close = new byte[34];
        close[0] = (byte) ((value.length >>> 8) & 0xFF);
        close[1] = (byte) (value.length & 0xFF);
        System.arraycopy(digest, 0, close, 2, digest.length);
        return close;
    }

    private static void assertScratchWiped(StreamDemoStreamAPDUAdapter adapter) throws Exception {
        Field scratchField = StreamDemoStreamAPDUAdapter.class.getDeclaredField("ioScratch");
        scratchField.setAccessible(true);
        byte[] scratch = (byte[]) scratchField.get(adapter);
        for (byte value : scratch) if (value != 0) throw new AssertionError("scratch not wiped");
    }
}
`
