import XCTest
@testable import HIDProtocol

final class FrameTests: XCTestCase {
    private func f32(_ v: Float) -> [UInt8] {
        withUnsafeBytes(of: v.bitPattern.bigEndian) { Array($0) }
    }

    private func u32(_ v: UInt32) -> [UInt8] {
        withUnsafeBytes(of: v.bigEndian) { Array($0) }
    }

    func testPayloadLengths() {
        XCTAssertEqual(Frame.payloadLength(forType: 0x01), 9)
        XCTAssertEqual(Frame.payloadLength(forType: 0x02), 9)
        XCTAssertEqual(Frame.payloadLength(forType: 0x03), 9)
        XCTAssertEqual(Frame.payloadLength(forType: 0x04), 16)
        XCTAssertEqual(Frame.payloadLength(forType: 0x05), 16)
        XCTAssertEqual(Frame.payloadLength(forType: 0x06), 16)
        XCTAssertEqual(Frame.payloadLength(forType: 0x07), 8)
        XCTAssertEqual(Frame.payloadLength(forType: 0x08), 8)
        XCTAssertEqual(Frame.payloadLength(forType: 0x09), 8)
        XCTAssertEqual(Frame.payloadLength(forType: 0x0A), 4)
        XCTAssertEqual(Frame.payloadLength(forType: 0x0B), 5)
        XCTAssertEqual(Frame.payloadLength(forType: 0x0C), 1)
        XCTAssertNil(Frame.payloadLength(forType: 0x00))
        XCTAssertNil(Frame.payloadLength(forType: 0x0D))
        XCTAssertNil(Frame.payloadLength(forType: 0xFF))
    }

    func testTouchFrames() {
        let payload = Data(f32(0.5) + f32(0.25) + [Edge.bottom.rawValue])
        XCTAssertEqual(Frame.parse(type: 0x01, payload: payload),
                       .touch(.down, x: 0.5, y: 0.25, edge: .bottom))
        XCTAssertEqual(Frame.parse(type: 0x02, payload: payload),
                       .touch(.move, x: 0.5, y: 0.25, edge: .bottom))
        XCTAssertEqual(Frame.parse(type: 0x03, payload: payload),
                       .touch(.up, x: 0.5, y: 0.25, edge: .bottom))
    }

    func testTouchEdgeValues() {
        for (raw, edge) in [(UInt8(0), Edge.none), (1, .left), (2, .top), (3, .right), (4, .bottom)] {
            let payload = Data(f32(0) + f32(0) + [raw])
            XCTAssertEqual(Frame.parse(type: 0x01, payload: payload),
                           .touch(.down, x: 0, y: 0, edge: edge))
        }
        XCTAssertNil(Frame.parse(type: 0x01, payload: Data(f32(0) + f32(0) + [5])),
                     "out-of-range edge must be rejected")
    }

    func testTwoFingerFrames() {
        // Values chosen to be exactly representable in float32 so the
        // Double round-trip compares equal.
        let payload = Data(f32(0.125) + f32(0.25) + f32(0.375) + f32(0.5))
        XCTAssertEqual(Frame.parse(type: 0x04, payload: payload),
                       .twoFinger(.down, x1: 0.125, y1: 0.25, x2: 0.375, y2: 0.5))
        XCTAssertEqual(Frame.parse(type: 0x05, payload: payload),
                       .twoFinger(.move, x1: 0.125, y1: 0.25, x2: 0.375, y2: 0.5))
        XCTAssertEqual(Frame.parse(type: 0x06, payload: payload),
                       .twoFinger(.up, x1: 0.125, y1: 0.25, x2: 0.375, y2: 0.5))
    }

    func testButtonFrames() {
        let payload = Data(u32(0x0C) + u32(0xE9))
        XCTAssertEqual(Frame.parse(type: 0x07, payload: payload),
                       .buttonPress(page: 0x0C, usage: 0xE9))
        XCTAssertEqual(Frame.parse(type: 0x08, payload: payload),
                       .buttonDown(page: 0x0C, usage: 0xE9))
        XCTAssertEqual(Frame.parse(type: 0x09, payload: payload),
                       .buttonUp(page: 0x0C, usage: 0xE9))
    }

    func testLegacyButtonFrame() {
        XCTAssertEqual(Frame.parse(type: 0x0A, payload: Data(u32(0))), .legacyButton(code: 0))
        XCTAssertEqual(Frame.parse(type: 0x0A, payload: Data(u32(1))), .legacyButton(code: 1))
    }

    func testKeyFrame() {
        let payload = Data([0x02] + u32(0x04))
        XCTAssertEqual(Frame.parse(type: 0x0B, payload: payload),
                       .key(modifiers: 0x02, usage: 0x04))
    }

    func testGestureFrame() {
        XCTAssertEqual(Frame.parse(type: 0x0C, payload: Data([1])), .gesture(.swipeToHome))
        XCTAssertEqual(Frame.parse(type: 0x0C, payload: Data([2])), .gesture(.appSwitcher))
        XCTAssertEqual(Frame.parse(type: 0x0C, payload: Data([3])), .gesture(.notificationCenter))
        XCTAssertEqual(Frame.parse(type: 0x0C, payload: Data([4])), .gesture(.lockScreen))
        XCTAssertNil(Frame.parse(type: 0x0C, payload: Data([0])), "unknown gesture kind rejected")
        XCTAssertNil(Frame.parse(type: 0x0C, payload: Data([5])), "unknown gesture kind rejected")
    }

    func testWrongPayloadLengthRejected() {
        XCTAssertNil(Frame.parse(type: 0x01, payload: Data([0, 0])))
        XCTAssertNil(Frame.parse(type: 0x0C, payload: Data()))
        XCTAssertNil(Frame.parse(type: 0xFF, payload: Data()))
    }

    func testReadersHonorNonZeroStartIndex() {
        // Slices of Data keep the parent's indices; readers must not assume 0-based.
        let full = Data([0xAA] + f32(0.75) + f32(0.5) + [0])
        let slice = full.dropFirst()
        XCTAssertEqual(Frame.parse(type: 0x01, payload: slice),
                       .touch(.down, x: 0.75, y: 0.5, edge: .none))
    }
}
