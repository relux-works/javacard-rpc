package io.jcrpc.client

class APDUResponse(val rawBytes: ByteArray) {
    init {
        require(rawBytes.size >= 2)
    }

    val sw: UShort
        get() = (((rawBytes[rawBytes.lastIndex - 1].toUByte().toUInt() shl 8) or
            rawBytes[rawBytes.lastIndex].toUByte().toUInt()) and 0xFFFFu).toUShort()

    val data: ByteArray
        get() = rawBytes.copyOf(rawBytes.size - 2)
}
