import Foundation
import Testing
import CounterClient
import JavaCardRPCClient

struct RuntimeAdapter: CounterTransport {
    let response: APDUResponse
    func transmit(cla: UInt8, ins: UInt8, p1: UInt8, p2: UInt8,
                  data: Data?) async throws -> (sw: UInt16, data: Data) {
        let command = APDUCommand(cla: cla, ins: ins, p1: p1, p2: p2, data: data)
        #expect(command.bytes == Data([0xB0, 3, 0, 0]))
        return (response.sw, response.data)
    }
}
// The unchanged generated get() uses a host adapter with real pinned runtime
// command/response types. Exact response bytes succeed; short responses reject.
@Test func pinnedRuntimeAdapter() async throws {
    let adapter = RuntimeAdapter(response: APDUResponse(rawBytes: Data([0, 7, 0x90, 0])))
    #expect(try await CounterClient(transport: adapter).get() == 7)
}
@Test(arguments: [0, 1]) func generatedClientRejectsShortResponse(count: Int) async throws {
    let adapter = RuntimeAdapter(response: APDUResponse(rawBytes: Data(repeating: 0, count: count) + Data([0x90, 0])))
    do {
        _ = try await CounterClient(transport: adapter).get()
        Issue.record("Malformed response admitted")
    } catch {
        #expect(String(describing: error) == "invalidResponse")
    }
}
// Released Swift decoding accepts trailing response bytes, unlike Kotlin's
// exact-length check. This compatibility lane preserves that existing bound.
@Test func generatedSwiftPreservesTrailingByteAcceptance() async throws {
    let adapter = RuntimeAdapter(response: APDUResponse(rawBytes: Data([0, 7, 99, 0x90, 0])))
    #expect(try await CounterClient(transport: adapter).get() == 7)
}
