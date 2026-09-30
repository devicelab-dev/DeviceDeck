import XCTest
import XPC
import HIDProtocol
@testable import DTUHID

final class DTUHIDWireTests: XCTestCase {

    func testShipsFromCoreSimulator1155_4() {
        XCTAssertFalse(DTUHIDWire.ships(coreSimulatorVersion: nil))
        XCTAssertFalse(DTUHIDWire.ships(coreSimulatorVersion: ""))
        XCTAssertFalse(DTUHIDWire.ships(coreSimulatorVersion: "1051.9"))
        XCTAssertFalse(DTUHIDWire.ships(coreSimulatorVersion: "1155.3"))
        XCTAssertTrue(DTUHIDWire.ships(coreSimulatorVersion: "1155.4"))
        XCTAssertTrue(DTUHIDWire.ships(coreSimulatorVersion: "1155.10")) // numeric, not lexical
        XCTAssertTrue(DTUHIDWire.ships(coreSimulatorVersion: "1200"))
    }

    func testEdgeUsesTheDaemonsNumbering() {
        XCTAssertEqual(DTUHIDWire.edge(.none), 0)
        XCTAssertEqual(DTUHIDWire.edge(.top), 1)
        XCTAssertEqual(DTUHIDWire.edge(.left), 2)
        XCTAssertEqual(DTUHIDWire.edge(.bottom), 3)
        XCTAssertEqual(DTUHIDWire.edge(.right), 4)
    }

    func testPhasesMapToStartPositionEnd() {
        XCTAssertEqual(DTUHIDWire.eventType(.down), 0)
        XCTAssertEqual(DTUHIDWire.eventType(.move), 1)
        XCTAssertEqual(DTUHIDWire.eventType(.up), 2)
    }

    func testTouchMessage() {
        let msg = DTUHIDWire.touch(x: 0.25, y: 0.75, phase: .move, edge: .bottom)
        assertEnvelope(msg, type: "IndigoDigitizerEvent", barrier: false)
        let payload = xpc_dictionary_get_value(msg, "payload")!
        let p1 = xpc_dictionary_get_value(payload, "pointOne")!
        XCTAssertEqual(xpc_dictionary_get_double(p1, "x"), 0.25)
        XCTAssertEqual(xpc_dictionary_get_double(p1, "y"), 0.75)
        XCTAssertNil(xpc_dictionary_get_value(payload, "pointTwo"))
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "eventType"), 1)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "edge"), 3)
        XCTAssertTrue(xpc_get_type(xpc_dictionary_get_value(payload, "target")!) == XPC_TYPE_UINT64)
    }

    func testTwoFingerMessage() {
        let msg = DTUHIDWire.twoFinger(x1: 0.1, y1: 0.2, x2: 0.3, y2: 0.4, phase: .up)
        assertEnvelope(msg, type: "IndigoDigitizerEvent", barrier: false)
        let payload = xpc_dictionary_get_value(msg, "payload")!
        let p2 = xpc_dictionary_get_value(payload, "pointTwo")!
        XCTAssertEqual(xpc_dictionary_get_double(p2, "x"), 0.3)
        XCTAssertEqual(xpc_dictionary_get_double(p2, "y"), 0.4)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "eventType"), 2)
    }

    func testKeyMessage() {
        let down = DTUHIDWire.key(usage: 0x28, down: true)
        assertEnvelope(down, type: "IndigoKeyboardButtonEvent", barrier: false)
        let payload = xpc_dictionary_get_value(down, "payload")!
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "usageCode"), 0x28)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "state"), 1)
        let up = xpc_dictionary_get_value(DTUHIDWire.key(usage: 0x28, down: false), "payload")!
        XCTAssertEqual(xpc_dictionary_get_uint64(up, "state"), 2)
    }

    func testButtonMessage() {
        let msg = DTUHIDWire.button(page: 0x0C, usage: 0xE9, down: true)
        assertEnvelope(msg, type: "IndigoButtonEvent", barrier: false)
        let payload = xpc_dictionary_get_value(msg, "payload")!
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "usagePage"), 0x0C)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "usageCode"), 0xE9)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "state"), 1)
    }

    func testBarrierIsAHarmlessKeyUp() {
        let msg = DTUHIDWire.barrier()
        assertEnvelope(msg, type: "IndigoKeyboardButtonEvent", barrier: true)
        let payload = xpc_dictionary_get_value(msg, "payload")!
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "usageCode"), 0)
        XCTAssertEqual(xpc_dictionary_get_uint64(payload, "state"), 2)
    }

    private func assertEnvelope(_ msg: xpc_object_t, type: String, barrier: Bool,
                                file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertEqual(String(cString: xpc_dictionary_get_string(msg, "messageType")!), type, file: file, line: line)
        XCTAssertEqual(String(cString: xpc_dictionary_get_string(msg, "featureIdentifier")!),
                       DTUHIDWire.digitizerService, file: file, line: line)
        XCTAssertEqual(xpc_dictionary_get_bool(msg, "isBarrier"), barrier, file: file, line: line)
    }
}
