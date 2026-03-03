import Foundation
import AppletRPCClient
import CounterClient

// MARK: - DI: Conform TCPTransport to generated CounterTransport protocol

extension TCPTransport: @retroactive CounterTransport {
    public func transmit(cla: UInt8, ins: UInt8, p1: UInt8, p2: UInt8, data: Data?) async throws -> (sw: UInt16, data: Data) {
        let command = APDUCommand(cla: cla, ins: ins, p1: p1, p2: p2, data: data)
        let response = try await transmit(command)
        return (sw: response.sw, data: response.data)
    }
}

// MARK: - E2E Test Runner

@main
struct CounterE2E {
    static func main() async {
        let transport = TCPTransport(host: "127.0.0.1", port: 9025)

        print("=== javacard-rpc Counter E2E Tests ===\n")

        do {
            try transport.connect()
            print("[OK] Connected to bridge\n")
        } catch {
            print("[FAIL] Connection failed: \(error)")
            exit(1)
        }

        let counter = CounterClient(transport: transport)
        var passed = 0
        var failed = 0

        func test(_ name: String, _ body: () async throws -> Void) async {
            do {
                try await body()
                print("[PASS] \(name)")
                passed += 1
            } catch {
                print("[FAIL] \(name): \(error)")
                failed += 1
            }
        }

        func expect<T: Equatable>(_ actual: T, _ expected: T, _ msg: String) throws {
            guard actual == expected else {
                throw TestError.assertion("\(msg): expected \(expected), got \(actual)")
            }
        }

        func expectSW(_ body: () async throws -> Void, sw: UInt16) async throws {
            do {
                try await body()
                throw TestError.assertion("Expected SW \(String(format: "%04X", sw)) but got success")
            } catch is TestError {
                throw TestError.assertion("Expected SW \(String(format: "%04X", sw)) but got success")
            } catch {
                // Generated client throws TransportError.statusWord(UInt16)
                // Check the error description contains the expected SW value
                let desc = String(describing: error)
                let swDecimal = String(sw)
                guard desc.contains(swDecimal) || desc.contains(String(format: "%04X", sw).lowercased()) else {
                    throw TestError.assertion("Expected SW \(String(format: "%04X", sw)), got error: \(error)")
                }
            }
        }

        // Test 1: SELECT applet
        await test("1. SELECT applet") {
            try await counter.select()
        }

        // Test 2: increment(5) → 5, increment(3) → 8
        await test("2. increment(5) → 5, increment(3) → 8") {
            let v1 = try await counter.increment(amount: 5)
            try expect(v1, UInt16(5), "increment(5)")
            let v2 = try await counter.increment(amount: 3)
            try expect(v2, UInt16(8), "increment(3)")
        }

        // Test 3: get() → 8
        await test("3. get() → 8") {
            let v = try await counter.get()
            try expect(v, UInt16(8), "get()")
        }

        // Test 4: decrement(2) → 6
        await test("4. decrement(2) → 6") {
            let v = try await counter.decrement(amount: 2)
            try expect(v, UInt16(6), "decrement(2)")
        }

        // Test 5: decrement(100) → SW_UNDERFLOW
        await test("5. decrement(100) → SW_UNDERFLOW") {
            try await expectSW({ try await counter.decrement(amount: 100) }, sw: 0x6985)
        }

        // Test 6: setLimit(10), increment(5) → SW_OVERFLOW (6+5=11 > 10)
        await test("6. setLimit(10), increment(5) → SW_OVERFLOW") {
            try await counter.setLimit(limit: 10)
            try await expectSW({ try await counter.increment(amount: 5) }, sw: 0x6986)
        }

        // Test 7: store("hello world"), load() → "hello world"
        await test("7. store + load roundtrip") {
            let data = "hello world".data(using: .utf8)!
            try await counter.store(data: data)
            let loaded = try await counter.load()
            guard loaded == data else {
                throw TestError.assertion("store/load mismatch: expected \(data.count) bytes, got \(loaded.count)")
            }
        }

        // Test 8: load() before store → SW_NO_DATA (need new session)
        // Skip — we already stored data. Tested in jCardSim tests.
        // Instead, test store with too-large data
        await test("8. store(200 bytes) → SW_DATA_TOO_LONG") {
            let bigData = Data(repeating: 0xAB, count: 200)
            try await expectSW({ try await counter.store(data: bigData) }, sw: 0x6A80)
        }

        // Test 9: getInfo() → correct values
        await test("9. getInfo() → {value=6, limit=10, version=1}") {
            let info = try await counter.getInfo()
            try expect(info.value, UInt16(6), "value")
            try expect(info.limit, UInt16(10), "limit")
            try expect(info.version, UInt8(1), "version")
        }

        // Test 10: reset() → get() → 0
        await test("10. reset() → get() → 0") {
            try await counter.reset()
            let v = try await counter.get()
            try expect(v, UInt16(0), "get after reset")
        }

        // Test 11: store(128 bytes) → OK, load() → same
        await test("11. store(128 bytes max) → load() roundtrip") {
            let data = Data((0..<128).map { UInt8($0) })
            try await counter.store(data: data)
            let loaded = try await counter.load()
            guard loaded == data else {
                throw TestError.assertion("128-byte roundtrip failed")
            }
        }

        print("\n=== Results: \(passed) passed, \(failed) failed ===")
        transport.disconnect()
        exit(failed > 0 ? 1 : 0)
    }
}

enum TestError: Error, CustomStringConvertible {
    case assertion(String)
    var description: String {
        switch self { case .assertion(let msg): return msg }
    }
}
