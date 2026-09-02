package io.jcrpc.client

data class APDUCommand(
    val cla: UByte,
    val ins: UByte,
    val p1: UByte = 0u,
    val p2: UByte = 0u,
    val data: ByteArray? = null,
    val le: UByte? = null,
)
