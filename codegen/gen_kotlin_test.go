package codegen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKotlinClientCounterSurface(t *testing.T) {
	s := parseCounter(t)
	if errs := Validate(s); len(errs) > 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}

	got, err := GenerateKotlinClient(s, "counter")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}

	src := string(got)

	requireContains(t, src, "package counter")
	requireContains(t, src, "public interface CounterTransport")
	requireContains(t, src, "suspend fun transmit(cla: UByte, ins: UByte, p1: UByte, p2: UByte, data: ByteArray?): CounterTransportResult")
	requireContains(t, src, "public sealed class CounterClientException")
	requireContains(t, src, "public class CounterClient(")
	requireContains(t, src, "override suspend fun increment(amount: UByte): UShort")
	requireContains(t, src, "override suspend fun getInfo(): CounterInfo")
	requireContains(t, src, "override suspend fun setCount(value: UInt)")
	requireContains(t, src, "override suspend fun setEnabled(enabled: Boolean)")
	requireContains(t, src, "override suspend fun getHash(): ByteArray")
	requireContains(t, src, "public data class CounterInfo(")
	requireContains(t, src, "public object CounterError")
	requireContains(t, src, "public val SW_UNDERFLOW: UShort = 0x6985u.toUShort()")
	requireContains(t, src, "return readBytes(response.data, 0, 32)")
	requireContains(t, src, "val data = byteArrayOf((((value.toLong() ushr 24) and 0xFF).toByte())")
	requireContains(t, src, "p1 = if (enabled) 0x01u else 0x00u")
	requireContains(t, src, "throw CounterClientException.StatusWord(sw)")
}

func TestGenerateKotlinClientSortsMethodsByINS(t *testing.T) {
	s := &Schema{
		Applet: Applet{
			Name: "Demo",
			AID:  "A000000001",
			CLA:  0x80,
		},
		Methods: map[string]*Method{
			"third":  {Name: "third", INS: 0x03},
			"first":  {Name: "first", INS: 0x01},
			"second": {Name: "second", INS: 0x02},
		},
	}

	got, err := GenerateKotlinClient(s, "demo")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}

	src := string(got)
	idxFirst := strings.Index(src, "override suspend fun first(")
	idxSecond := strings.Index(src, "override suspend fun second(")
	idxThird := strings.Index(src, "override suspend fun third(")
	if idxFirst == -1 || idxSecond == -1 || idxThird == -1 {
		t.Fatalf("generated source missing expected methods:\n%s", src)
	}
	if !(idxFirst < idxSecond && idxSecond < idxThird) {
		t.Fatalf("methods are not sorted by INS:\n%s", src)
	}
}

func TestGenerateKotlinClientSupportsASCIIAndString(t *testing.T) {
	s := &Schema{
		Applet: Applet{
			Name: "Demo",
			AID:  "A000000001",
			CLA:  0x80,
		},
		Methods: map[string]*Method{
			"setImsi": {
				Name: "setImsi",
				INS:  0x01,
				Request: &Message{Fields: []Field{
					{Name: "imsi", Type: FieldTypeASCII, Length: intPtr(15), Location: ParameterLocationData},
				}},
			},
			"echoMessage": {
				Name: "echoMessage",
				INS:  0x02,
				Request: &Message{Fields: []Field{
					{Name: "message", Type: FieldTypeString, Location: ParameterLocationData},
				}},
				Response: &Message{Fields: []Field{
					{Name: "message", Type: FieldTypeString},
				}},
			},
		},
	}

	got, err := GenerateKotlinClient(s, "demo")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}

	src := string(got)
	requireContains(t, src, "override suspend fun setImsi(imsi: String)")
	requireContains(t, src, "val data = asciiBytes(imsi)")
	requireContains(t, src, "if (data.size != 15) invalidResponse()")
	requireContains(t, src, "override suspend fun echoMessage(message: String): String")
	requireContains(t, src, "val data = utf8Bytes(message)")
	requireContains(t, src, "return readString(response.data, 0)")
}

func TestGenerateKotlinClientSupportsBidirectionalStreamLifecycle(t *testing.T) {
	s, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if errs := Validate(s); len(errs) > 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}

	got, err := GenerateKotlinClient(s, "io.jcrpc.streamdemo.client")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}
	src := string(got)

	requireContains(t, src, "import java.security.MessageDigest")
	requireContains(t, src, "override suspend fun processPacket(requestPacket: ByteArray): ByteArray")
	requireContains(t, src, "transmitIdempotent(0x20u, packetIndex.toUByte(), requestPacketCount.toUByte(), chunk)")
	requireContains(t, src, "transport.transmit(CLA, 0x21u, 0x00u, 0x00u, requestCloseData)")
	requireContains(t, src, "transmitIdempotent(0x22u, 0x00u, 0x00u, null)")
	requireContains(t, src, "transmitIdempotent(0x23u, packetIndex.toUByte(), streamDescriptor.packetCount.toUByte(), null)")
	requireContains(t, src, "transmitIdempotent(0x24u, 0x00u, 0x00u, responseCloseData)")
	requireContains(t, src, "bestEffortStreamAbort(0x25u)")
	requireContains(t, src, "fun invalidateStreamSession()")
	requireContains(t, src, "private fun bestEffortInvalidateStreamSession()")
	requireContains(t, src, "bestEffortInvalidateStreamSession()")
	requireContains(t, src, "private val streamSessionInUse = AtomicBoolean(false)")
	requireContains(t, src, "throw StreamDemoClientException.StreamBusy")
	requireContains(t, src, "streamSessionInUse.set(false)")
	requireContains(t, src, "if (data.size != 35) invalidResponse()")
	requireContains(t, src, "MessageDigest.getInstance(\"SHA-256\").digest(streamResult)")
}

func TestGenerateKotlinClientRetriesStreamRequestCloseForShortResponse(t *testing.T) {
	s := &Schema{
		Applet: Applet{Name: "Demo", AID: "A000000001", CLA: 0x80},
		Methods: map[string]*Method{
			"sign": {
				Name: "sign",
				INS:  0x30,
				Request: &Message{Fields: []Field{{
					Name: "payload", Type: FieldTypeStream, MaxLength: 1024, ChunkSize: 192,
				}}},
				Response: &Message{Fields: []Field{{Name: "receipt", Type: FieldTypeU16}}},
			},
		},
	}

	got, err := GenerateKotlinClient(s, "demo")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}
	src := string(got)
	requireContains(t, src, "override suspend fun sign(payload: ByteArray): UShort")
	requireContains(t, src, "val response = transmitIdempotent(0x31u, 0x00u, 0x00u, requestCloseData)")
	requireContains(t, src, "val decodedResponse = run {")
	requireContains(t, src, "return@run readU16(response.data, 0)")
	requireContains(t, src, "streamTerminal = true")
	requireContains(t, src, "return decodedResponse")
}

func TestGenerateKotlinClientRecoversResponseOnlyStreamDescriptor(t *testing.T) {
	s := &Schema{
		Applet: Applet{Name: "Demo", AID: "A000000001", CLA: 0x80},
		Methods: map[string]*Method{
			"export": {
				Name: "export",
				INS:  0x40,
				Request: &Message{Fields: []Field{{
					Name: "selector", Type: FieldTypeU8, Location: ParameterLocationData,
				}}},
				Response: &Message{Fields: []Field{{
					Name: "packet", Type: FieldTypeStream, MaxLength: 2048, ChunkSize: 224,
				}}},
			},
		},
	}

	got, err := GenerateKotlinClient(s, "demo")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}
	src := string(got)
	requireContains(t, src, "override suspend fun export(selector: UByte): ByteArray")
	requireContains(t, src, "transport.transmit(CLA, 0x40u, 0x00u, 0x00u, data)")
	requireContains(t, src, "transmitIdempotent(0x42u, 0x00u, 0x00u, null)")
	requireContains(t, src, "parseStreamDescriptor(descriptorResponse.data, 2048, 224)")
}
