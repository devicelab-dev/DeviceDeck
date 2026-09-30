import XCTest
@testable import VideoCore

/// The coalescer is what keeps frame callbacks from queueing one XPC
/// round-trip per composited frame; these pin its one-pending contract.
final class PendingCaptureTests: XCTestCase {
    func testFirstRequestMustBeScheduled() {
        var pending = PendingCapture()
        XCTAssertTrue(pending.request())
    }

    func testRepeatRequestsCoalesceOntoThePendingOne() {
        var pending = PendingCapture()
        _ = pending.request()
        XCTAssertFalse(pending.request())
        XCTAssertFalse(pending.request())
    }

    func testRequestAfterCaptureBeganSchedulesAFreshOne() {
        var pending = PendingCapture()
        _ = pending.request()
        pending.begin()
        XCTAssertTrue(pending.request())
    }

    func testBurstDuringOneSlowCaptureSchedulesOnce() {
        var pending = PendingCapture()
        var scheduled = 0
        for _ in 0..<10_000 where pending.request() { scheduled += 1 }
        XCTAssertEqual(scheduled, 1)
    }

    func testEachCaptureCycleSchedulesExactlyOnce() {
        var pending = PendingCapture()
        var scheduled = 0
        for cycle in 0..<5 {
            for _ in 0..<100 where pending.request() { scheduled += 1 }
            pending.begin()
            XCTAssertEqual(scheduled, cycle + 1)
        }
    }
}

/// The target fps is a ceiling on capture starts, never a drop.
final class CaptureCadenceTests: XCTestCase {
    func testIntervalFollowsFps() {
        XCTAssertEqual(CaptureCadence(fps: 30).interval, 1.0 / 30, accuracy: 1e-12)
        XCTAssertEqual(CaptureCadence(fps: 60).interval, 1.0 / 60, accuracy: 1e-12)
    }

    func testFpsBelowOneClampsToOne() {
        XCTAssertEqual(CaptureCadence(fps: 0).interval, 1)
        XCTAssertEqual(CaptureCadence(fps: -5).interval, 1)
    }

    func testDelayTable() {
        let cases: [(name: String, lastStart: TimeInterval?, now: TimeInterval, want: TimeInterval)] = [
            ("first capture runs immediately", nil, 100, 0),
            ("request right after a start waits a full interval", 10, 10, 0.1),
            ("request mid-interval waits the remainder", 10, 10.04, 0.06),
            ("request exactly one interval later runs now", 10, 10.1, 0),
            ("request long after runs now, never negative", 10, 50, 0),
        ]
        for c in cases {
            var cadence = CaptureCadence(fps: 10)
            if let start = c.lastStart { cadence.began(at: start) }
            XCTAssertEqual(cadence.delay(at: c.now), c.want, accuracy: 1e-9, c.name)
        }
    }

    func testCallbackFloodIsHeldToTheCeiling() {
        // 1000 notifications over one second at 30fps start at most 31 captures.
        var cadence = CaptureCadence(fps: 30)
        var pending = PendingCapture()
        var nextStart: TimeInterval?
        var starts = 0
        for tick in 0..<1000 {
            let now = Double(tick) / 1000
            if let at = nextStart, now >= at {
                pending.begin()
                cadence.began(at: now)
                starts += 1
                nextStart = nil
            }
            if pending.request() { nextStart = now + cadence.delay(at: now) }
        }
        XCTAssertLessThanOrEqual(starts, 31)
        XCTAssertGreaterThanOrEqual(starts, 29)
    }
}

/// Seed / surface-ID semantics carried over from the polling loop.
final class FrameChangeTests: XCTestCase {
    func testNeedsEncodeTable() {
        let cases: [(name: String, seed: UInt32, id: UInt32, force: Bool, want: Bool)] = [
            ("unchanged surface is skipped", 5, 7, false, false),
            ("seed moved encodes", 6, 7, false, true),
            ("surface swapped encodes", 5, 8, false, true),
            ("keyframe request encodes a still screen", 5, 7, true, true),
        ]
        for c in cases {
            var change = FrameChange()
            change.commit(seed: 5, surfaceID: 7)
            XCTAssertEqual(change.needsEncode(seed: c.seed, surfaceID: c.id, force: c.force), c.want, c.name)
        }
    }

    func testFirstRealFrameEncodes() {
        XCTAssertTrue(FrameChange().needsEncode(seed: 1, surfaceID: 1, force: false))
    }

    func testUncommittedFrameIsRetried() {
        // A frame the encoder dropped is not committed, so the next capture
        // of the same surface still encodes it.
        var change = FrameChange()
        change.commit(seed: 1, surfaceID: 1)
        XCTAssertTrue(change.needsEncode(seed: 2, surfaceID: 1, force: false))
        XCTAssertTrue(change.needsEncode(seed: 2, surfaceID: 1, force: false))
        change.commit(seed: 2, surfaceID: 1)
        XCTAssertFalse(change.needsEncode(seed: 2, surfaceID: 1, force: false))
    }
}

/// The signature gate in front of the private callback selectors.
final class MethodShapeTests: XCTestCase {
    func testShapeTable() {
        let cases: [(encoding: String, want: String)] = [
            ("v56@0:8@16@24@?32@?40@?48", "v@:@@@?@?@?"),
            ("v24@0:8@16", "v@:@"),
            ("@16@0:8", "@@:"),
            ("", ""),
        ]
        for c in cases {
            XCTAssertEqual(MethodShape.of(c.encoding), c.want, c.encoding)
        }
    }

    func testMatches() {
        XCTAssertTrue(MethodShape.matches("v56@0:8@16@24@?32@?40@?48", expected: "v@:@@@?@?@?"))
        XCTAssertFalse(MethodShape.matches("v48@0:8@16@24@?32@?40", expected: "v@:@@@?@?@?"))
        XCTAssertFalse(MethodShape.matches("v56@0:8@16@24@32@?40@?48", expected: "v@:@@@?@?@?"))
    }
}
