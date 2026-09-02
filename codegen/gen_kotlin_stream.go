package codegen

import (
	"fmt"
	"strings"
)

const kotlinStreamHelpers = `    private data class StreamDescriptor(
        val packetCount: Int,
        val totalLength: Int,
        val digest: ByteArray,
    )

    private fun streamCloseData(value: ByteArray): ByteArray {
        if (value.isEmpty() || value.size > 0x7FFF) invalidResponse()
        val digest = MessageDigest.getInstance("SHA-256").digest(value)
        return ByteArray(34).also { result ->
            result[0] = ((value.size ushr 8) and 0xFF).toByte()
            result[1] = (value.size and 0xFF).toByte()
            digest.copyInto(result, destinationOffset = 2)
        }
    }

    private fun parseStreamDescriptor(data: ByteArray, maxLength: Int, chunkSize: Int): StreamDescriptor {
        if (data.size != 35) invalidResponse()
        val packetCount = data[0].toUByte().toInt()
        val totalLength = (data[1].toUByte().toInt() shl 8) or data[2].toUByte().toInt()
        if (packetCount == 0 || totalLength == 0 || totalLength > maxLength) invalidResponse()
        val expectedPacketCount = (totalLength + chunkSize - 1) / chunkSize
        if (packetCount != expectedPacketCount) invalidResponse()
        return StreamDescriptor(packetCount, totalLength, data.copyOfRange(3, 35))
    }

    private fun verifyEmptySuccess(response: APDUResponse) {
        checkStatusWord(response.sw)
        if (response.data.isNotEmpty()) invalidResponse()
    }

    private suspend fun transmitIdempotent(
        ins: UByte,
        p1: UByte,
        p2: UByte,
        data: ByteArray?,
    ): APDUResponse {
        return try {
            transmit(CLA, ins, p1, p2, data)
        } catch (cancellation: CancellationException) {
            throw cancellation
        } catch (firstFailure: Exception) {
            try {
                transmit(CLA, ins, p1, p2, data)
            } catch (cancellation: CancellationException) {
                throw cancellation
            } catch (_: Exception) {
                throw firstFailure
            }
        }
    }

    private suspend fun bestEffortStreamAbort(ins: UByte) {
        try {
            val response = transmit(CLA, ins, 0x00u, 0x00u, null)
            verifyEmptySuccess(response)
        } catch (_: Throwable) {
            // The original operation failure remains authoritative.
        }
    }

    private fun bestEffortInvalidateStreamSession() {
        try {
            transport.invalidateSession()
        } catch (_: Throwable) {
            // The original operation failure remains authoritative.
        }
    }

`

func renderKotlinStreamImportBlock(hasStreams bool) string {
	if !hasStreams {
		return ""
	}
	return "import java.security.MessageDigest\nimport java.util.concurrent.CancellationException\nimport java.util.concurrent.atomic.AtomicBoolean\n"
}

func renderKotlinStreamHelpersBlock(hasStreams bool) string {
	if !hasStreams {
		return ""
	}
	return kotlinStreamHelpers
}

func renderKotlinStreamExceptionBlock(hasStreams bool, clientExceptionName string) string {
	if !hasStreams {
		return ""
	}
	return fmt.Sprintf(`
    public object StreamBusy : %s("Another generated stream operation is active")
`, clientExceptionName)
}

func renderKotlinStreamOwnedFieldsBlock(hasStreams bool) string {
	if !hasStreams {
		return ""
	}
	return `
    private val streamSessionInUse = AtomicBoolean(false)
`
}

func buildKotlinStreamMethod(
	appletName string,
	methodName string,
	method *Method,
) (kotlinMethodData, *kotlinResponseStructData, error) {
	requestStream := method.Request.StreamField()
	responseStream := method.Response.StreamField()

	params := make([]kotlinParam, 0)
	p1Expr := "0x00u"
	p2Expr := "0x00u"
	dataExpr := ""
	hasData := false
	dataPrepLines := []string(nil)
	if requestStream != nil {
		params = append(params, kotlinParam{Name: requestStream.Name, Type: "ByteArray"})
	} else {
		var err error
		params, p1Expr, p2Expr, dataPrepLines, dataExpr, hasData, err = buildKotlinRequestSpec(methodName, method.Request)
		if err != nil {
			return kotlinMethodData{}, nil, err
		}
	}

	returnType := "ByteArray"
	var returnLines []string
	var responseStruct *kotlinResponseStructData
	if responseStream == nil {
		var err error
		returnType, returnLines, responseStruct, err = buildKotlinResponseSpec(appletName, methodName, method.Response)
		if err != nil {
			return kotlinMethodData{}, nil, err
		}
	}

	body := make([]string, 0, 96)
	body = append(body, dataPrepLines...)
	body = append(body,
		"if (!streamSessionInUse.compareAndSet(false, true)) throw "+appletName+"ClientException.StreamBusy",
		"var streamTerminal = false",
		"try {",
	)

	if requestStream != nil {
		body = append(body, buildKotlinStreamUploadLines(method.INS, *requestStream)...)
		if responseStream != nil {
			body = append(body, buildKotlinRequestCloseForDescriptorLines(method.INS, requestStream.Name)...)
		} else {
			body = append(body, buildKotlinRequestCloseForShortResponseLines(method.INS, requestStream.Name)...)
		}
	} else {
		body = append(body, buildKotlinResponseOnlyInvokeLines(method.INS, p1Expr, p2Expr, dataExpr, hasData)...)
	}

	if responseStream != nil {
		body = append(body, buildKotlinStreamDownloadLines(method.INS, *responseStream)...)
	} else {
		body = append(body, buildKotlinValidatedShortResponseLines(returnLines)...)
	}

	body = append(body,
		"} catch (failure: Throwable) {",
		"    if (!streamTerminal) {",
		"        bestEffortStreamAbort(0x"+fmt.Sprintf("%02X", method.INS+5)+"u)",
		"        bestEffortInvalidateStreamSession()",
		"    }",
		"    throw failure",
		"} finally {",
		"    streamSessionInUse.set(false)",
		"}",
	)

	return kotlinMethodData{
		Name:               methodName,
		ParameterSignature: renderKotlinParameterSignature(params),
		ReturnType:         returnType,
		BodyLines:          body,
	}, responseStruct, nil
}

func buildKotlinValidatedShortResponseLines(returnLines []string) []string {
	if len(returnLines) == 0 {
		return []string{
			"    if (response.data.isNotEmpty()) invalidResponse()",
			"    streamTerminal = true",
			"    return",
		}
	}

	lines := []string{"    val decodedResponse = run {"}
	for _, line := range returnLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "return ") {
			line = strings.Replace(line, "return ", "return@run ", 1)
		}
		lines = append(lines, "        "+line)
	}
	return append(lines,
		"    }",
		"    streamTerminal = true",
		"    return decodedResponse",
	)
}

func buildKotlinStreamUploadLines(baseINS byte, stream Field) []string {
	name := stream.Name
	return []string{
		fmt.Sprintf("    if (%s.isEmpty() || %s.size > %d) invalidResponse()", name, name, stream.MaxLength),
		fmt.Sprintf("    val requestPacketCount = (%s.size + %d - 1) / %d", name, stream.ChunkSize, stream.ChunkSize),
		"    for (packetIndex in 0 until requestPacketCount) {",
		fmt.Sprintf("        val chunkStart = packetIndex * %d", stream.ChunkSize),
		fmt.Sprintf("        val chunkEnd = minOf(chunkStart + %d, %s.size)", stream.ChunkSize, name),
		fmt.Sprintf("        val chunk = %s.copyOfRange(chunkStart, chunkEnd)", name),
		fmt.Sprintf("        val chunkResponse = transmitIdempotent(0x%02Xu, packetIndex.toUByte(), requestPacketCount.toUByte(), chunk)", baseINS),
		"        verifyEmptySuccess(chunkResponse)",
		"    }",
	}
}

func buildKotlinRequestCloseForDescriptorLines(baseINS byte, requestName string) []string {
	closeINS := baseINS + 1
	pendingINS := baseINS + 2
	return []string{
		fmt.Sprintf("    val requestCloseData = streamCloseData(%s)", requestName),
		"    val descriptorResponse = try {",
		fmt.Sprintf("        transmit(CLA, 0x%02Xu, 0x00u, 0x00u, requestCloseData)", closeINS),
		"    } catch (cancellation: CancellationException) {",
		"        throw cancellation",
		"    } catch (closeFailure: Exception) {",
		"        try {",
		fmt.Sprintf("            transmitIdempotent(0x%02Xu, 0x00u, 0x00u, null)", pendingINS),
		"        } catch (cancellation: CancellationException) {",
		"            throw cancellation",
		"        } catch (_: Exception) {",
		"            throw closeFailure",
		"        }",
		"    }",
		"    checkStatusWord(descriptorResponse.sw)",
	}
}

func buildKotlinRequestCloseForShortResponseLines(baseINS byte, requestName string) []string {
	closeINS := baseINS + 1
	return []string{
		fmt.Sprintf("    val requestCloseData = streamCloseData(%s)", requestName),
		fmt.Sprintf("    val response = transmitIdempotent(0x%02Xu, 0x00u, 0x00u, requestCloseData)", closeINS),
		"    checkStatusWord(response.sw)",
	}
}

func buildKotlinResponseOnlyInvokeLines(baseINS byte, p1Expr, p2Expr, dataExpr string, hasData bool) []string {
	dataArg := "null"
	if hasData {
		dataArg = dataExpr
	}
	return []string{
		"    val descriptorResponse = try {",
		fmt.Sprintf("        transmit(CLA, 0x%02Xu, %s, %s, %s)", baseINS, p1Expr, p2Expr, dataArg),
		"    } catch (cancellation: CancellationException) {",
		"        throw cancellation",
		"    } catch (invokeFailure: Exception) {",
		"        try {",
		fmt.Sprintf("            transmitIdempotent(0x%02Xu, 0x00u, 0x00u, null)", baseINS+2),
		"        } catch (cancellation: CancellationException) {",
		"            throw cancellation",
		"        } catch (_: Exception) {",
		"            throw invokeFailure",
		"        }",
		"    }",
		"    checkStatusWord(descriptorResponse.sw)",
	}
}

func buildKotlinStreamDownloadLines(baseINS byte, stream Field) []string {
	descriptorSource := "descriptorResponse.data"
	lines := []string{
		fmt.Sprintf("    val streamDescriptor = parseStreamDescriptor(%s, %d, %d)", descriptorSource, stream.MaxLength, stream.ChunkSize),
		"    val streamResult = ByteArray(streamDescriptor.totalLength)",
		"    for (packetIndex in 0 until streamDescriptor.packetCount) {",
		fmt.Sprintf("        val chunkResponse = transmitIdempotent(0x%02Xu, packetIndex.toUByte(), streamDescriptor.packetCount.toUByte(), null)", baseINS+3),
		"        checkStatusWord(chunkResponse.sw)",
		fmt.Sprintf("        val chunkStart = packetIndex * %d", stream.ChunkSize),
		fmt.Sprintf("        val expectedLength = minOf(%d, streamDescriptor.totalLength - chunkStart)", stream.ChunkSize),
		"        if (chunkResponse.data.size != expectedLength) invalidResponse()",
		"        chunkResponse.data.copyInto(streamResult, destinationOffset = chunkStart)",
		"    }",
		"    if (!MessageDigest.getInstance(\"SHA-256\").digest(streamResult).contentEquals(streamDescriptor.digest)) invalidResponse()",
		"    val responseCloseData = streamCloseData(streamResult)",
		fmt.Sprintf("    val closeResponse = transmitIdempotent(0x%02Xu, 0x00u, 0x00u, responseCloseData)", baseINS+4),
		"    verifyEmptySuccess(closeResponse)",
		"    streamTerminal = true",
		"    return streamResult",
	}
	return lines
}
