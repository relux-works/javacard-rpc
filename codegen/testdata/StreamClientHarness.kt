package io.jcrpc.streamdemo.client

import io.jcrpc.streamdemo.server.StreamDemoBoundedStreamRuntime
import io.jcrpc.streamdemo.server.StreamDemoStreamEndpoint
import java.security.MessageDigest
import java.util.concurrent.CancellationException
import kotlin.coroutines.Continuation
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.coroutines.startCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.suspendCoroutine
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

private class LostResponse : RuntimeException()

private class RuntimeBackedTransport(
    private val loseWriteChunkResponse: Boolean = false,
    private val loseCloseWriteResponse: Boolean = false,
    private val loseReadChunkResponse: Boolean = false,
    private val loseCloseReadResponse: Boolean = false,
    private val corruptDescriptorDigest: Boolean = false,
    private val cancelAfterINS: Int? = null,
    private val suspendAfterINS: Int? = null,
    private val throwOnInvalidate: Boolean = false,
) : StreamDemoTransport {
    private val workspace = ByteArray(2048)
    private val digestScratch = ByteArray(32)
    private val resetMarker = ByteArray(1)
    private var writeChunkResponseLost = false
    private var closeWriteResponseLost = false
    private var readChunkResponseLost = false
    private var closeReadResponseLost = false
    private var cancellationDelivered = false
    private var suspensionDelivered = false
    private var pendingContinuation: Continuation<StreamDemoTransportResult>? = null
    private var pendingResponse: StreamDemoTransportResult? = null

    var handlerExecutions: Int = 0
        private set
    var writeChunkCalls: Int = 0
        private set
    var pendingInfoCalls: Int = 0
        private set
    var closeReadCalls: Int = 0
        private set
    var readChunkCalls: Int = 0
        private set
    var abortCalls: Int = 0
        private set
    var invalidateCalls: Int = 0
        private set

    private val sha256 = StreamDemoStreamEndpoint.Sha256 { input, inputOffset, inputLength, output, outputOffset ->
            val digest = MessageDigest.getInstance("SHA-256").digest(
                input.copyOfRange(inputOffset.toInt(), inputOffset.toInt() + inputLength.toInt()),
            )
            digest.copyInto(output, destinationOffset = outputOffset.toInt())
        }

    private val handler = StreamDemoStreamEndpoint.Handler { _, input, inputOffset, inputLength, output, outputOffset, _ ->
            handlerExecutions++
            var left = 0
            var right = inputLength.toInt() - 1
            while (left <= right) {
                val leftValue = input[inputOffset.toInt() + left]
                output[outputOffset.toInt() + left] = input[inputOffset.toInt() + right]
                output[outputOffset.toInt() + right] = leftValue
                left++
                right--
            }
            inputLength
        }

    private val runtime = StreamDemoBoundedStreamRuntime(
        workspace,
        digestScratch,
        resetMarker,
        sha256,
    )

    override suspend fun transmit(
        cla: UByte,
        ins: UByte,
        p1: UByte,
        p2: UByte,
        data: ByteArray?,
    ): StreamDemoTransportResult {
        assertEquals(0xB0u.toUByte(), cla)
        val operation = ins.toInt() - 0x20
        require(operation in 0..5) { "unexpected INS 0x%02X".format(ins.toInt()) }
        if (ins.toInt() == 0x20) writeChunkCalls++
        if (ins.toInt() == 0x22) pendingInfoCalls++
        if (ins.toInt() == 0x23) readChunkCalls++
        if (ins.toInt() == 0x24) closeReadCalls++
        if (ins.toInt() == 0x25) abortCalls++

        val request = data ?: byteArrayOf()
        val response = ByteArray(255)
        val responseLength = try {
            runtime.dispatch(
                1.toByte(),
                operation.toByte(),
                true,
                1792.toShort(),
                192.toShort(),
                true,
                1792.toShort(),
                192.toShort(),
                handler,
                p1.toByte(),
                p2.toByte(),
                request,
                0,
                request.size.toShort(),
                response,
                0,
                response.size.toShort(),
            ).toInt() and 0xFFFF
        } catch (failure: StreamDemoStreamEndpoint.StreamStatusWordException) {
            return StreamDemoTransportResult(failure.statusWord.toUShort(), byteArrayOf())
        }

        if (ins.toInt() == cancelAfterINS && !cancellationDelivered) {
            cancellationDelivered = true
            throw CancellationException("cancelled after INS")
        }

        if (ins.toInt() == 0x20 && p1.toInt() == 0 && loseWriteChunkResponse && !writeChunkResponseLost) {
            writeChunkResponseLost = true
            throw LostResponse()
        }
        if (ins.toInt() == 0x21 && loseCloseWriteResponse && !closeWriteResponseLost) {
            closeWriteResponseLost = true
            throw LostResponse()
        }
        if (ins.toInt() == 0x23 && p1.toInt() == 0 && loseReadChunkResponse && !readChunkResponseLost) {
            readChunkResponseLost = true
            throw LostResponse()
        }
        if (ins.toInt() == 0x24 && loseCloseReadResponse && !closeReadResponseLost) {
            closeReadResponseLost = true
            throw LostResponse()
        }

        val result = response.copyOf(responseLength)
        if (corruptDescriptorDigest && (ins.toInt() == 0x21 || ins.toInt() == 0x22) && result.size == 35) {
            result[3] = (result[3].toInt() xor 0x01).toByte()
        }
        val transportResult = StreamDemoTransportResult(0x9000u.toUShort(), result)
        if (ins.toInt() == suspendAfterINS && !suspensionDelivered) {
            suspensionDelivered = true
            return suspendCoroutine { continuation ->
                pendingContinuation = continuation
                pendingResponse = transportResult
            }
        }
        return transportResult
    }

    override fun invalidateStreamSession() {
        invalidateCalls++
        if (throwOnInvalidate) throw IllegalStateException("invalidation failed")
        runtime.abort(StreamDemoStreamEndpoint.ABORT_HOST_FAILURE)
    }

    fun workspaceIsCleared(): Boolean = workspace.all { it == 0.toByte() }

    fun resumeSuspendedResponse() {
        val continuation = requireNotNull(pendingContinuation)
        val response = requireNotNull(pendingResponse)
        pendingContinuation = null
        pendingResponse = null
        continuation.resume(response)
    }
}

class StreamClientHarness {
    @Test
    fun generatedClientAndRuntimeRecoverLostCloseResponsesWithoutRepeatingTheHandler() {
        val transport = RuntimeBackedTransport(
            loseWriteChunkResponse = true,
            loseCloseWriteResponse = true,
            loseReadChunkResponse = true,
            loseCloseReadResponse = true,
        )
        val client = StreamDemoClient(transport)
        val request = ByteArray(300) { index -> (index and 0xFF).toByte() }

        val response = runSuspend { client.processPacket(request) }

        assertContentEquals(request.reversedArray(), response)
        assertEquals(1, transport.handlerExecutions)
        assertEquals(3, transport.writeChunkCalls)
        assertEquals(1, transport.pendingInfoCalls)
        assertEquals(3, transport.readChunkCalls)
        assertEquals(2, transport.closeReadCalls)
        assertEquals(0, transport.abortCalls)
        assertEquals(0, transport.invalidateCalls)
        assertTrue(transport.workspaceIsCleared())
    }

    @Test
    fun generatedClientAbortsGeneratedRuntimeWhenTheResultDigestDoesNotMatch() {
        val transport = RuntimeBackedTransport(corruptDescriptorDigest = true)
        val client = StreamDemoClient(transport)
        val request = ByteArray(300) { index -> (index and 0xFF).toByte() }

        assertFailsWith<StreamDemoClientException.InvalidResponse> {
            runSuspend { client.processPacket(request) }
        }

        assertEquals(1, transport.handlerExecutions)
        assertEquals(1, transport.abortCalls)
        assertEquals(1, transport.invalidateCalls)
        assertTrue(transport.workspaceIsCleared())
    }

    @Test
    fun generatedClientInvalidatesTheTransportAtEveryCancellationBoundary() {
        for (ins in listOf(0x20, 0x21, 0x23, 0x24)) {
            val transport = RuntimeBackedTransport(cancelAfterINS = ins)
            val client = StreamDemoClient(transport)
            val request = ByteArray(300) { index -> (index and 0xFF).toByte() }

            assertFailsWith<CancellationException>("INS 0x%02X".format(ins)) {
                runSuspend { client.processPacket(request) }
            }

            assertEquals(1, transport.invalidateCalls, "INS 0x%02X".format(ins))
            assertTrue(transport.workspaceIsCleared(), "INS 0x%02X".format(ins))
        }
    }

    @Test
    fun generatedClientRejectsASecondStreamOperationWithoutTouchingTheFirstSession() {
        val transport = RuntimeBackedTransport(suspendAfterINS = 0x20)
        val client = StreamDemoClient(transport)
        val request = ByteArray(300) { index -> (index and 0xFF).toByte() }
        var firstResult: Result<ByteArray>? = null
        suspend { client.processPacket(request) }.startCoroutine(object : Continuation<ByteArray> {
            override val context = EmptyCoroutineContext
            override fun resumeWith(result: Result<ByteArray>) { firstResult = result }
        })
        assertEquals(null, firstResult)

        assertFailsWith<StreamDemoClientException.StreamBusy> {
            runSuspend { client.processPacket(byteArrayOf(9)) }
        }
        assertEquals(0, transport.abortCalls)
        assertEquals(0, transport.invalidateCalls)

        transport.resumeSuspendedResponse()
        assertContentEquals(request.reversedArray(), requireNotNull(firstResult).getOrThrow())
        assertTrue(transport.workspaceIsCleared())
    }

    @Test
    fun invalidationFailureDoesNotMaskTheAuthoritativeFailureOrCancellation() {
        val protocolTransport = RuntimeBackedTransport(
            corruptDescriptorDigest = true,
            throwOnInvalidate = true,
        )
        assertFailsWith<StreamDemoClientException.InvalidResponse> {
            runSuspend { StreamDemoClient(protocolTransport).processPacket(ByteArray(300) { it.toByte() }) }
        }
        assertEquals(1, protocolTransport.invalidateCalls)

        val cancelledTransport = RuntimeBackedTransport(
            cancelAfterINS = 0x20,
            throwOnInvalidate = true,
        )
        assertFailsWith<CancellationException> {
            runSuspend { StreamDemoClient(cancelledTransport).processPacket(ByteArray(300) { it.toByte() }) }
        }
        assertEquals(1, cancelledTransport.invalidateCalls)
    }
}

private fun <T> runSuspend(block: suspend () -> T): T {
    var result: Result<T>? = null
    block.startCoroutine(object : Continuation<T> {
        override val context = EmptyCoroutineContext

        override fun resumeWith(resumeResult: Result<T>) {
            result = resumeResult
        }
    })
    return requireNotNull(result).getOrThrow()
}
