import io.jcrpc.client.APDUCommand
import io.jcrpc.client.APDUResponse
import io.jcrpc.client.APDUTransport
import io.jcrpc.compat.CounterClient
import io.jcrpc.compat.CounterClientException
import io.jcrpc.compat.CounterSkeleton
import io.jcrpc.server.AppletBase
import kotlin.coroutines.*
import kotlin.test.*

class PinnedConsumerTest {
    private class Recorder(var response: APDUResponse) : APDUTransport {
        var calls = 0
        override suspend fun transmit(command: APDUCommand): APDUResponse {
            calls++
            assertEquals(3u.toUByte(), command.ins)
            return response
        }
        override fun invalidateSession() = Unit
    }
    // With an immediate runtime transport, the generated get() accepts exactly
    // two bytes and rejects neighboring wrong lengths and a failure status word.
    @Test fun generatedClientUsesPinnedRuntimeAndRejectsMalformedResponses() {
        val recorder = Recorder(APDUResponse(byteArrayOf(0, 7, 0x90.toByte(), 0)))
        val client = CounterClient(recorder)
        assertEquals(7u.toUShort(), immediate { client.get() })
        for (count in listOf(0, 1, 3)) {
            recorder.response = APDUResponse(ByteArray(count) + byteArrayOf(0x90.toByte(), 0))
            assertSame(CounterClientException.InvalidResponse, assertFails { immediate { client.get() } })
        }
        recorder.response = APDUResponse(byteArrayOf(0, 7, 0x6D, 0))
        val error = assertFailsWith<CounterClientException.StatusWord> { immediate { client.get() } }
        assertEquals(0x6D00u.toUShort(), error.sw)
        assertEquals(5, recorder.calls)
    }
    // Both released native public server classes are linked on the consumer's
    // JVM classpath; simulator behavior is covered by the separate real lane.
    @Test fun publicJavaServerClassesLinkWithoutRenaming() {
        assertEquals("io.jcrpc.server.AppletBase", AppletBase::class.java.name)
        assertEquals("io.jcrpc.compat.CounterSkeleton", CounterSkeleton::class.java.name)
    }
    private fun <T> immediate(block: suspend () -> T): T {
        var result: Result<T>? = null
        block.startCoroutine(object : Continuation<T> {
            override val context = EmptyCoroutineContext
            override fun resumeWith(value: Result<T>) { result = value }
        })
        return checkNotNull(result) { "Test transport must respond synchronously" }.getOrThrow()
    }
}

// Executes regenerated packed/borrowed callbacks against the pinned runtime
// classpath. No simulator or physical transport is claimed by this fixture.
class PinnedWriterConsumerTest {
    private class Logic : CounterSkeleton(null) {
        var packedCalls = 0
        override fun onIncrement(amount: Byte, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = amount.toShort()
        override fun onDecrement(amount: Byte, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = amount.toShort()
        override fun onGet(callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 7
        override fun onReset(callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short) = Unit
        override fun onSetLimit(limit: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short) = Unit
        override fun onStore(data: ByteArray, dataOffset: Short, dataLength: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short) = Unit
        override fun onGetInfo(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short {
            packedCalls++
            assertEquals(7, outputCapacity.toInt())
            var at = outputOffset.toInt()
            at = packU16(output, at, 7)
            at = packU16(output, at, 12)
            at = packU8(output, at, 1)
            at = packBool(output, at, false)
            packBool(output, at, true)
            return 7
        }
        override fun onEchoMessage(message: ByteArray, messageOffset: Short, messageLength: Short,
            output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short {
            if (messageLength > outputCapacity) throw statusWordFailure(0x6700)
            packBytes(output, outputOffset.toInt(), message, messageOffset.toInt(), messageLength.toInt())
            return messageLength
        }
        override fun onLoad(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 0
        override fun onGetSpki(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 91
        override fun onGetImsi(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 0
        override fun onGetAppletInfo(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 12
        override fun onSignChallenge(challenge: ByteArray, challengeOffset: Short, challengeLength: Short,
            output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 0
        override fun onGetDisplayName(output: ByteArray, outputOffset: Short, outputCapacity: Short, callerWorkspace: ByteArray, callerWorkspaceOffset: Short, callerWorkspaceCapacity: Short): Short = 0
    }
    // Insufficient capacity refuses before the packed callback, without writes;
    // the valid control gets the exact width and preserves neighboring bytes.
    @Test fun fixedWriterRefusesBeforeCallbackAndPreservesWire() {
        val logic = Logic()
        val output = ByteArray(133) { 0x55 }
        val failure = assertFailsWith<CounterSkeleton.StatusWordException> {
            logic.dispatchTo(6, 0, 0, null, 0, 0, output, 7, 6, output, 7, 6)
        }
        assertEquals(0x6700, failure.statusWord.toInt() and 0xFFFF)
        assertEquals(0, logic.packedCalls)
        assertTrue(output.all { it == 0x55.toByte() })
        assertEquals(7, logic.dispatchTo(6, 0, 0, null, 0, 0, output, 7, 100, output, 7, 100).toInt())
        assertContentEquals(byteArrayOf(0, 7, 0, 12, 1, 0, 1), output.copyOfRange(7, 14))
        assertEquals(0x55, output[6].toInt())
        assertEquals(0x55, output[14].toInt())
        assertEquals(1, logic.packedCalls)
    }
    // The generated dispatcher borrows a nonzero input span and the trusted
    // writer's memmove copy preserves the existing echo wire under overlap.
    @Test fun borrowedWriterSupportsOverlapAndRejectsInvalidWindows() {
        val logic = Logic()
        val buffer = byteArrayOf(99, 99, 1, 2, 3, 4, 99, 99)
        assertEquals(4, logic.dispatchTo(14, 0, 0, buffer, 2, 4, buffer, 0, 4, buffer, 0, 4).toInt())
        assertContentEquals(byteArrayOf(1, 2, 3, 4), buffer.copyOfRange(0, 4))
        for (offset in listOf<Short>(-1, 8)) {
            val before = buffer.copyOf()
            val failure = assertFailsWith<CounterSkeleton.StatusWordException> {
                logic.dispatchTo(14, 0, 0, buffer, offset, 1, buffer, 0, 4, buffer, 0, 4)
            }
            assertEquals(0x6700, failure.statusWord.toInt() and 0xFFFF)
            assertContentEquals(before, buffer)
        }
    }
}
