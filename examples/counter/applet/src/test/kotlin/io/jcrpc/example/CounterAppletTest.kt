package io.jcrpc.counter.example

import com.licel.jcardsim.smartcardio.CardSimulator
import com.licel.jcardsim.utils.AIDUtil
import javacard.framework.AID
import javax.smartcardio.CommandAPDU
import javax.smartcardio.ResponseAPDU
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

class CounterAppletTest {

    companion object {
        private const val AID_HEX = "F000000101"
        private val AID_BYTES = AID_HEX.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
        private const val CLA = 0xB0
    }

    private lateinit var sim: CardSimulator
    private val appletAid: AID = AIDUtil.create(AID_HEX)

    @BeforeEach
    fun setUp() {
        sim = CardSimulator()
        sim.installApplet(appletAid, CounterJCApplet::class.java)
        sim.selectApplet(appletAid)
    }

    private fun send(ins: Int, p1: Int = 0, p2: Int = 0, data: ByteArray? = null, le: Int? = null): ResponseAPDU {
        val cmd = if (data != null && le != null) {
            CommandAPDU(CLA, ins, p1, p2, data, le)
        } else if (data != null) {
            CommandAPDU(CLA, ins, p1, p2, data)
        } else if (le != null) {
            CommandAPDU(CLA, ins, p1, p2, le)
        } else {
            CommandAPDU(CLA, ins, p1, p2)
        }
        return sim.transmitCommand(cmd)
    }

    private fun readU16(data: ByteArray, offset: Int = 0): Int {
        return ((data[offset].toInt() and 0xFF) shl 8) or (data[offset + 1].toInt() and 0xFF)
    }

    private fun readBool(data: ByteArray, offset: Int): Boolean {
        return when (data[offset].toInt() and 0xFF) {
            0x00 -> false
            0x01 -> true
            else -> error("invalid bool encoding at offset $offset")
        }
    }

    private fun parseTlv(data: ByteArray): Map<Int, ByteArray> {
        val out = linkedMapOf<Int, ByteArray>()
        var off = 0
        while (off < data.size) {
            val tag = data[off++].toInt() and 0xFF
            val len = data[off++].toInt() and 0xFF
            out[tag] = data.copyOfRange(off, off + len)
            off += len
        }
        return out
    }

    private fun expectedMockSignature(challenge: ByteArray, counter: Int, limit: Int): ByteArray {
        val partLen = minOf(challenge.size, 8)
        val out = ByteArray(2 + 2 + partLen + 2 + partLen)
        out[0] = 0x30
        out[1] = (out.size - 2).toByte()
        out[2] = 0x02
        out[3] = partLen.toByte()
        for (i in 0 until partLen) {
            out[4 + i] = (challenge[i].toInt() xor 0x01 xor counter).toByte()
        }
        out[4] = (out[4].toInt() and 0x7F).toByte()

        val secondOff = 4 + partLen
        out[secondOff] = 0x02
        out[secondOff + 1] = partLen.toByte()
        for (i in 0 until partLen) {
            val srcIndex = challenge.size - 1 - i
            out[secondOff + 2 + i] = (challenge[srcIndex].toInt() xor limit xor 0x5A).toByte()
        }
        out[secondOff + 2] = (out[secondOff + 2].toInt() and 0x7F).toByte()
        return out
    }

    // --- SELECT ---

    @Test
    fun `SELECT returns 9000`() {
        val select = CommandAPDU(0x00, 0xA4, 0x04, 0x00, AID_BYTES)
        val resp = sim.transmitCommand(select)
        assertEquals(0x9000, resp.sw)
    }

    // --- INCREMENT ---

    @Nested
    inner class Increment {

        @Test
        fun `increment by 5 returns 5`() {
            val resp = send(0x01, p1 = 5)
            assertEquals(0x9000, resp.sw)
            assertEquals(2, resp.data.size)
            assertEquals(5, readU16(resp.data))
        }

        @Test
        fun `increment twice accumulates`() {
            send(0x01, p1 = 5)
            val resp = send(0x01, p1 = 3)
            assertEquals(0x9000, resp.sw)
            assertEquals(8, readU16(resp.data))
        }

        @Test
        fun `increment by 0 returns current`() {
            send(0x01, p1 = 10)
            val resp = send(0x01, p1 = 0)
            assertEquals(0x9000, resp.sw)
            assertEquals(10, readU16(resp.data))
        }

        @Test
        fun `increment past limit returns SW_OVERFLOW`() {
            // set limit to 10
            send(0x05, data = byteArrayOf(0x00, 0x0A))
            // increment to 10
            send(0x01, p1 = 10)
            // increment by 1 more — should overflow
            val resp = send(0x01, p1 = 1)
            assertEquals(0x6986, resp.sw)
        }
    }

    // --- DECREMENT ---

    @Nested
    inner class Decrement {

        @Test
        fun `decrement after increment`() {
            send(0x01, p1 = 10)
            val resp = send(0x02, p1 = 3)
            assertEquals(0x9000, resp.sw)
            assertEquals(7, readU16(resp.data))
        }

        @Test
        fun `decrement to zero`() {
            send(0x01, p1 = 5)
            val resp = send(0x02, p1 = 5)
            assertEquals(0x9000, resp.sw)
            assertEquals(0, readU16(resp.data))
        }

        @Test
        fun `decrement below zero returns SW_UNDERFLOW`() {
            val resp = send(0x02, p1 = 1)
            assertEquals(0x6985, resp.sw)
        }

        @Test
        fun `decrement large amount returns SW_UNDERFLOW`() {
            send(0x01, p1 = 5)
            val resp = send(0x02, p1 = 100)
            assertEquals(0x6985, resp.sw)
        }
    }

    // --- GET ---

    @Nested
    inner class Get {

        @Test
        fun `get initial value is 0`() {
            val resp = send(0x03, le = 2)
            assertEquals(0x9000, resp.sw)
            assertEquals(0, readU16(resp.data))
        }

        @Test
        fun `get after increment`() {
            send(0x01, p1 = 42)
            val resp = send(0x03, le = 2)
            assertEquals(0x9000, resp.sw)
            assertEquals(42, readU16(resp.data))
        }
    }

    // --- RESET ---

    @Nested
    inner class Reset {

        @Test
        fun `reset returns counter to zero`() {
            send(0x01, p1 = 50)
            val resp = send(0x04)
            assertEquals(0x9000, resp.sw)

            val get = send(0x03, le = 2)
            assertEquals(0, readU16(get.data))
        }

        @Test
        fun `reset when already zero`() {
            val resp = send(0x04)
            assertEquals(0x9000, resp.sw)
        }
    }

    // --- SET LIMIT ---

    @Nested
    inner class SetLimit {

        @Test
        fun `set limit accepts u16`() {
            // limit = 256 (0x0100)
            val resp = send(0x05, data = byteArrayOf(0x01, 0x00))
            assertEquals(0x9000, resp.sw)
        }

        @Test
        fun `set limit enforced on increment`() {
            // limit = 5
            send(0x05, data = byteArrayOf(0x00, 0x05))
            send(0x01, p1 = 5) // ok, at limit
            val resp = send(0x01, p1 = 1) // exceeds
            assertEquals(0x6986, resp.sw)
        }

        @Test
        fun `set limit to zero blocks all increments`() {
            send(0x05, data = byteArrayOf(0x00, 0x00))
            val resp = send(0x01, p1 = 1)
            assertEquals(0x6986, resp.sw)
        }
    }

    // --- GET INFO ---

    @Nested
    inner class GetInfo {

        @Test
        fun `getInfo returns value, limit, version`() {
            send(0x01, p1 = 7)
            send(0x05, data = byteArrayOf(0x00, 0x64)) // limit = 100
            send(0x07, data = "blob".toByteArray())
            val resp = send(0x06, le = 5)

            assertEquals(0x9000, resp.sw)
            assertEquals(7, resp.data.size)
            assertEquals(7, readU16(resp.data, 0))     // value
            assertEquals(100, readU16(resp.data, 2))    // limit
            assertEquals(0x01, resp.data[4].toInt() and 0xFF) // version
            assertEquals(true, readBool(resp.data, 5))
            assertEquals(false, readBool(resp.data, 6))
        }

        @Test
        fun `getInfo default state`() {
            val resp = send(0x06, le = 5)
            assertEquals(0x9000, resp.sw)
            assertEquals(7, resp.data.size)
            assertEquals(0, readU16(resp.data, 0))        // value = 0
            assertEquals(0x7FFF, readU16(resp.data, 2))   // default limit
            assertEquals(0x01, resp.data[4].toInt() and 0xFF) // version
            assertEquals(false, readBool(resp.data, 5))
            assertEquals(false, readBool(resp.data, 6))
        }

        @Test
        fun `getInfo reports at limit`() {
            send(0x05, data = byteArrayOf(0x00, 0x05))
            send(0x01, p1 = 5)
            val resp = send(0x06, le = 7)
            assertEquals(true, readBool(resp.data, 6))
        }
    }

    // --- AUTH-LIKE METADATA ---

    @Nested
    inner class AuthLikeMethods {

        @Test
        fun `getSpki returns fixed 91-byte DER payload`() {
            val resp = send(0x09, le = 128)
            assertEquals(0x9000, resp.sw)
            assertEquals(91, resp.data.size)
            assertContentEquals(byteArrayOf(0x30, 0x59, 0x30, 0x13), resp.data.copyOfRange(0, 4))
            assertEquals(0x04, resp.data[26].toInt() and 0xFF)
        }

        @Test
        fun `getImsi returns ascii digits`() {
            val resp = send(0x0A, le = 32)
            assertEquals(0x9000, resp.sw)
            assertContentEquals("250011234567890".toByteArray(), resp.data)
        }

        @Test
        fun `getAppletInfo returns expected tlv`() {
            val resp = send(0x0B, le = 64)
            assertEquals(0x9000, resp.sw)
            val tlv = parseTlv(resp.data)
            assertContentEquals(byteArrayOf(0x01), tlv[0x01])
            assertContentEquals(byteArrayOf(0xF0.toByte(), 0x00, 0x00, 0x01, 0x01), tlv[0x02])
            assertEquals("1.0.0", tlv[0x03]!!.toString(Charsets.US_ASCII))
            assertContentEquals(byteArrayOf(0x01), tlv[0x04])
            assertContentEquals(byteArrayOf(0x00, 0x3F), tlv[0x05])
        }

        @Test
        fun `signChallenge returns deterministic mock der signature`() {
            send(0x01, p1 = 6)
            send(0x05, data = byteArrayOf(0x00, 0x0A))
            val challenge = "auth-like-challenge".toByteArray()
            val resp = send(0x0C, data = challenge)
            assertEquals(0x9000, resp.sw)
            assertContentEquals(expectedMockSignature(challenge, counter = 6, limit = 10), resp.data)
        }

        @Test
        fun `signChallenge with empty payload returns SW_EMPTY_CHALLENGE`() {
            val resp = send(0x0C, data = byteArrayOf())
            assertEquals(0x6700, resp.sw)
        }
    }

    // --- STORE ---

    @Nested
    inner class Store {

        @Test
        fun `store small data`() {
            val data = "hello".toByteArray()
            val resp = send(0x07, data = data)
            assertEquals(0x9000, resp.sw)
        }

        @Test
        fun `store 128 bytes (max)`() {
            val data = ByteArray(128) { it.toByte() }
            val resp = send(0x07, data = data)
            assertEquals(0x9000, resp.sw)
        }

        @Test
        fun `store 129 bytes returns SW_DATA_TOO_LONG`() {
            val data = ByteArray(129) { it.toByte() }
            val resp = send(0x07, data = data)
            assertEquals(0x6A80, resp.sw)
        }
    }

    // --- LOAD ---

    @Nested
    inner class Load {

        @Test
        fun `load before store returns SW_NO_DATA`() {
            val resp = send(0x08, le = 256)
            assertEquals(0x6A88, resp.sw)
        }

        @Test
        fun `store then load returns same data`() {
            val data = "hello world".toByteArray()
            send(0x07, data = data)
            val resp = send(0x08, le = 256)
            assertEquals(0x9000, resp.sw)
            assertContentEquals(data, resp.data)
        }

        @Test
        fun `store overwrites previous`() {
            send(0x07, data = "first".toByteArray())
            send(0x07, data = "second".toByteArray())
            val resp = send(0x08, le = 256)
            assertEquals(0x9000, resp.sw)
            assertContentEquals("second".toByteArray(), resp.data)
        }

        @Test
        fun `store binary data roundtrip`() {
            val data = ByteArray(64) { (it * 3).toByte() }
            send(0x07, data = data)
            val resp = send(0x08, le = 256)
            assertEquals(0x9000, resp.sw)
            assertContentEquals(data, resp.data)
        }
    }

    // --- Error handling ---

    @Nested
    inner class ErrorHandling {

        @Test
        fun `wrong CLA returns 6E00`() {
            val cmd = CommandAPDU(0xFF, 0x01, 0x00, 0x00)
            val resp = sim.transmitCommand(cmd)
            assertEquals(0x6E00, resp.sw)
        }

        @Test
        fun `unknown INS returns 6D00`() {
            val resp = send(0xFF)
            assertEquals(0x6D00, resp.sw)
        }

        @Test
        fun `setLimit with short data returns 6700`() {
            // Only 1 byte, needs 2
            val resp = send(0x05, data = byteArrayOf(0x01))
            assertEquals(0x6700, resp.sw)
        }
    }

    // --- Integration scenarios ---

    @Nested
    inner class Integration {

        @Test
        fun `full workflow - increment, get, decrement, reset`() {
            // increment 5
            var resp = send(0x01, p1 = 5)
            assertEquals(5, readU16(resp.data))

            // increment 3
            resp = send(0x01, p1 = 3)
            assertEquals(8, readU16(resp.data))

            // get
            resp = send(0x03, le = 2)
            assertEquals(8, readU16(resp.data))

            // decrement 2
            resp = send(0x02, p1 = 2)
            assertEquals(6, readU16(resp.data))

            // reset
            send(0x04)

            // get after reset
            resp = send(0x03, le = 2)
            assertEquals(0, readU16(resp.data))
        }

        @Test
        fun `store and load with counter operations`() {
            send(0x01, p1 = 42)
            send(0x07, data = "test data".toByteArray())

            val getResp = send(0x03, le = 2)
            assertEquals(42, readU16(getResp.data))

            val loadResp = send(0x08, le = 256)
            assertContentEquals("test data".toByteArray(), loadResp.data)
        }

        @Test
        fun `getInfo reflects all state changes`() {
            send(0x01, p1 = 25)
            send(0x05, data = byteArrayOf(0x01, 0x00)) // limit = 256
            send(0x07, data = "test".toByteArray())

            val resp = send(0x06, le = 7)
            assertEquals(25, readU16(resp.data, 0))
            assertEquals(256, readU16(resp.data, 2))
            assertEquals(0x01, resp.data[4].toInt() and 0xFF)
            assertEquals(true, readBool(resp.data, 5))
            assertEquals(false, readBool(resp.data, 6))
        }

        @Test
        fun `auth-like methods coexist with counter storage flow`() {
            send(0x07, data = "session blob".toByteArray())
            val imsi = send(0x0A, le = 32)
            val appletInfo = send(0x0B, le = 64)
            val signature = send(0x0C, data = "hello".toByteArray())

            assertEquals("250011234567890", imsi.data.toString(Charsets.US_ASCII))
            assertEquals(0x9000, appletInfo.sw)
            assertEquals(0x9000, signature.sw)
            assertEquals(0x30, signature.data[0].toInt() and 0xFF)
        }
    }
}
