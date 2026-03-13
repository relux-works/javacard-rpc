package codegen

import (
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
