package io.jcrpc.client

interface APDUTransport {
    suspend fun transmit(command: APDUCommand): APDUResponse
    fun invalidateSession()
}
