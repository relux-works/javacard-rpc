package io.jcrpc.example

import counter.CounterAppletInfo
import counter.CounterClient
import counter.CounterClientException
import counter.CounterInfo
import counter.CounterTransport
import counter.CounterTransportResult
import io.jcrpc.client.APDUCommand
import io.jcrpc.client.APDUTransport
import io.jcrpc.client.TCPTransport
import kotlinx.coroutines.runBlocking
import kotlin.system.exitProcess

private const val DISPLAY_NAME = "\u041F\u0440\u0438\u0432\u0435\u0442, BSim"
private const val ROUNDTRIP_MESSAGE = "\u041F\u0440\u0438\u0432\u0435\u0442, BSim \uD83D\uDE80"

private class CounterBridgeTransport(
    private val transport: APDUTransport,
) : CounterTransport {
    override suspend fun transmit(
        cla: UByte,
        ins: UByte,
        p1: UByte,
        p2: UByte,
        data: ByteArray?,
    ): CounterTransportResult {
        val response = transport.transmit(APDUCommand(cla = cla, ins = ins, p1 = p1, p2 = p2, data = data))
        return CounterTransportResult(sw = response.sw, data = response.data)
    }
}

fun main(): Unit = runBlocking {
    val tcpTransport = TCPTransport(host = "127.0.0.1", port = 9025)

    println("=== javacard-rpc Counter Kotlin E2E Tests ===")
    println()

    try {
        tcpTransport.connect()
        println("[OK] Connected to bridge")
        println()
    } catch (error: Exception) {
        println("[FAIL] Connection failed: $error")
        exitProcess(1)
    }

    val counter = CounterClient(transport = CounterBridgeTransport(tcpTransport))
    var passed = 0
    var failed = 0

    suspend fun test(name: String, body: suspend () -> Unit) {
        try {
            body()
            println("[PASS] $name")
            passed += 1
        } catch (error: Exception) {
            println("[FAIL] $name: $error")
            failed += 1
        }
    }

    fun <T> expect(actual: T, expected: T, message: String) {
        if (actual != expected) {
            throw IllegalStateException("$message: expected $expected, got $actual")
        }
    }

    fun expectBytes(actual: ByteArray, expected: ByteArray, message: String) {
        if (!actual.contentEquals(expected)) {
            throw IllegalStateException("$message: expected ${expected.toHexString()}, got ${actual.toHexString()}")
        }
    }

    fun expectPrefix(actual: ByteArray, prefix: ByteArray, message: String) {
        if (actual.size < prefix.size || !actual.copyOfRange(0, prefix.size).contentEquals(prefix)) {
            throw IllegalStateException("$message: wrong prefix")
        }
    }

    fun expectedMockSignature(challenge: ByteArray, counter: UShort, limit: UShort): ByteArray {
        val partLen = minOf(challenge.size, 8)
        val output = ByteArray(2 + 2 + partLen + 2 + partLen)
        output[0] = 0x30
        output[1] = (output.size - 2).toByte()
        output[2] = 0x02
        output[3] = partLen.toByte()

        for (index in 0 until partLen) {
            output[4 + index] = (challenge[index].toInt() xor 0x01 xor (counter.toInt() and 0xFF)).toByte()
        }
        output[4] = (output[4].toInt() and 0x7F).toByte()

        val secondOffset = 4 + partLen
        output[secondOffset] = 0x02
        output[secondOffset + 1] = partLen.toByte()
        for (index in 0 until partLen) {
            val sourceIndex = challenge.lastIndex - index
            output[secondOffset + 2 + index] =
                (challenge[sourceIndex].toInt() xor (limit.toInt() and 0xFF) xor 0x5A).toByte()
        }
        output[secondOffset + 2] = (output[secondOffset + 2].toInt() and 0x7F).toByte()
        return output
    }

    suspend fun expectSW(expectedSW: UShort, body: suspend () -> Unit) {
        try {
            body()
            throw IllegalStateException("Expected SW ${expectedSW.toHexString()} but got success")
        } catch (error: CounterClientException.StatusWord) {
            if (error.sw != expectedSW) {
                throw IllegalStateException("Expected SW ${expectedSW.toHexString()}, got ${error.sw.toHexString()}")
            }
        }
    }

    test("1. SELECT applet") {
        counter.select()
    }

    test("2. increment(5) -> 5, increment(3) -> 8") {
        expect(counter.increment(5u), 5u.toUShort(), "increment(5)")
        expect(counter.increment(3u), 8u.toUShort(), "increment(3)")
    }

    test("3. get() -> 8") {
        expect(counter.get(), 8u.toUShort(), "get()")
    }

    test("4. decrement(2) -> 6") {
        expect(counter.decrement(2u), 6u.toUShort(), "decrement(2)")
    }

    test("5. decrement(100) -> SW_UNDERFLOW") {
        expectSW(0x6985u.toUShort()) { counter.decrement(100u) }
    }

    test("6. setLimit(10), increment(5) -> SW_OVERFLOW") {
        counter.setLimit(10u)
        expectSW(0x6986u.toUShort()) { counter.increment(5u) }
    }

    test("7. store + load roundtrip") {
        val data = "hello world".encodeToByteArray()
        counter.store(data)
        expectBytes(counter.load(), data, "store/load roundtrip")
    }

    test("8. store(200 bytes) -> SW_DATA_TOO_LONG") {
        val largePayload = ByteArray(200) { 0xAB.toByte() }
        expectSW(0x6A80u.toUShort()) { counter.store(largePayload) }
    }

    test("9. getInfo() -> typed state snapshot") {
        val info: CounterInfo = counter.getInfo()
        expect(info.value, 6u.toUShort(), "value")
        expect(info.limit, 10u.toUShort(), "limit")
        expect(info.version, 1u.toUByte(), "version")
        expect(info.hasStoredData, true, "hasStoredData")
        expect(info.isAtLimit, false, "isAtLimit")
    }

    test("10. getSpki() -> fixed 91-byte mock DER") {
        val spki = counter.getSpki()
        expect(spki.size, 91, "spki byte count")
        expectPrefix(spki, byteArrayOf(0x30, 0x59, 0x30, 0x13), "spki")
        expect(spki[26].toUByte(), 0x04u.toUByte(), "spki EC point prefix")
    }

    test("11. getImsi() -> ASCII digits") {
        expect(counter.getImsi(), "250011234567890", "imsi")
    }

    test("12. getAppletInfo() -> typed metadata payload") {
        val info: CounterAppletInfo = counter.getAppletInfo()
        expect(info.schemaVersion, 0x01u.toUByte(), "schemaVersion")
        expectBytes(info.appletAid, byteArrayOf(0xF0.toByte(), 0x00, 0x00, 0x01, 0x01), "appletAid")
        expect(info.versionMajor, 0x01u.toUByte(), "versionMajor")
        expect(info.versionMinor, 0x00u.toUByte(), "versionMinor")
        expect(info.versionPatch, 0x00u.toUByte(), "versionPatch")
        expect(info.keyAlgorithm, 0x01u.toUByte(), "keyAlgorithm")
        expect(info.capabilities, 0x003Fu.toUShort(), "capabilities")
    }

    test("13. signChallenge() -> deterministic mock DER") {
        val challenge = "auth-like-challenge".encodeToByteArray()
        val signature = counter.signChallenge(challenge)
        val info = counter.getInfo()
        val expectedSignature = expectedMockSignature(challenge = challenge, counter = info.value, limit = info.limit)
        expectBytes(signature, expectedSignature, "signature")
    }

    test("14. signChallenge(empty) -> SW_EMPTY_CHALLENGE") {
        expectSW(0x6700u.toUShort()) { counter.signChallenge(byteArrayOf()) }
    }

    test("15. getDisplayName() -> UTF-8 string") {
        expect(counter.getDisplayName(), DISPLAY_NAME, "displayName")
    }

    test("16. echoMessage() -> UTF-8 roundtrip") {
        expect(counter.echoMessage(ROUNDTRIP_MESSAGE), ROUNDTRIP_MESSAGE, "echoMessage")
    }

    test("17. reset() -> get() -> 0") {
        counter.reset()
        expect(counter.get(), 0u.toUShort(), "get after reset")
    }

    test("18. store(128 bytes max) -> load() roundtrip") {
        val data = ByteArray(128) { index -> index.toByte() }
        counter.store(data)
        expectBytes(counter.load(), data, "128-byte roundtrip")
    }

    println()
    println("=== Results: $passed passed, $failed failed ===")
    tcpTransport.disconnect()
    exitProcess(if (failed > 0) 1 else 0)
}

private fun UShort.toHexString(): String = "0x%04X".format(this.toInt())

private fun ByteArray.toHexString(): String = joinToString(separator = "") { "%02X".format(it.toUByte().toInt()) }
