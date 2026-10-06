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
