import Foundation
import CoreGraphics
import HIDProtocol

/// The iOS 26-safe touch path: build a real IOHIDEvent digitizer parent +
/// finger child, wrap it via `IndigoHIDMessageForTrackpadEventFromHIDEventRef`,
/// then patch the byte slots the wrapper leaves uninitialised. Recipe
/// cracked by baguette while debugging the Xcode 26 mouse-event regression
/// (taps dropped or misread as Home gestures) — see ATTRIBUTION.md.
///
/// Patched slots, per message record (the wrapper emits two):
///   0x6c / 0x10c        → routing target 0x32; iOS drops untagged messages
///   0x3a–0x3b / 0xda–0xdb → edge present flag + edge bitmask, which is how
///                           the home-indicator recognizer sees edge touches
struct Digitizer {
    private let kit: SimKit
    private let client: HIDClient

    init(kit: SimKit, client: HIDClient) {
        self.kit = kit
        self.client = client
    }

    /// Edge bitmask values written at 0x3b/0xdb — empirically derived by
    /// baguette by sweeping the 7-arg mouse builder's edge argument.
    private static func edgeBit(_ edge: HIDProtocol.Edge) -> UInt8 {
        switch edge {
        case .none: return 0x00
        case .left: return 0x02
        case .top: return 0x08
        case .right: return 0x04
        case .bottom: return 0x01
        }
    }

    /// IOHIDDigitizerEventMask: down/move carry Range|Touch|Position (0x07)
    /// so iOS threads them as one sustained touch; up carries Touch|Position
    /// (0x06) so the lift registers as a state change.
    private static func mask(_ phase: TouchPhase) -> (mask: UInt32, contact: Bool) {
        phase == .up ? (0x06, false) : (0x07, true)
    }

    /// Build, patch, and send one digitizer event. `identifier` threads a
    /// touch sequence — same value across down/move/up, fresh per touch.
    @discardableResult
    func send(x: Double, y: Double, phase: TouchPhase, edge: HIDProtocol.Edge,
              identifier: UInt32) -> Bool {
        guard let make = kit.createDigitizer, let makeFinger = kit.createFinger,
              let append = kit.appendEvent, let wrap = kit.trackpadWrap else {
            log("digitizer path unavailable — touch dropped")
            return false
        }
        let (mask, contact) = Digitizer.mask(phase)
        let now = mach_absolute_time()
        // transducer 2 = kIOHIDDigitizerTransducerTypeFinger. Real iOS
        // touches always arrive as parent+child pairs; a bare finger event
        // wraps into a stub iOS ignores.
        guard let parentUM = make(nil, now, 2, 0, identifier, mask, 0,
                                  x, y, 0, 0, 0, contact, contact, 0) else { return false }
        let parent = parentUM.takeRetainedValue()
        if let fingerUM = makeFinger(nil, now, 0, identifier, mask,
                                     x, y, 0, 0, 0, contact, contact, 0) {
            append(parent, fingerUM.takeRetainedValue(), 0)
        }
        guard let msg = withExtendedLifetime(parent, {
            wrap(Unmanaged.passUnretained(parent as AnyObject).toOpaque())
        }) else { return false }
        patch(message: msg, edge: edge)
        client.send(msg)
        return true
    }

    /// Interpolated swipe with optional endpoint dwell. iOS discriminates
    /// Home vs App Switcher on a bottom-edge swipe purely from velocity and
    /// dwell, so the recipes below differ only in timing.
    @discardableResult
    func swipe(from start: CGPoint, to end: CGPoint, steps: Int, stepMs: UInt32,
               dwellMs: UInt32, edge: HIDProtocol.Edge, identifier: UInt32) -> Bool {
        guard send(x: start.x, y: start.y, phase: .down, edge: edge, identifier: identifier) else {
            return false
        }
        var ok = 0
        for i in 1...steps {
            usleep(stepMs * 1000)
            let t = Double(i) / Double(steps)
            if send(x: start.x + (end.x - start.x) * t,
                    y: start.y + (end.y - start.y) * t,
                    phase: .move, edge: edge, identifier: identifier) { ok += 1 }
        }
        // Re-sending the endpoint keeps the touch alive through the
        // recognizer's decision window even if a single move drops.
        for _ in 0..<(dwellMs / 50) {
            _ = send(x: end.x, y: end.y, phase: .move, edge: edge, identifier: identifier)
            usleep(50_000)
        }
        usleep(stepMs * 1000)
        let up = send(x: end.x, y: end.y, phase: .up, edge: edge, identifier: identifier)
        return up && ok >= steps / 2
    }

    private func patch(message msg: UnsafeMutableRawPointer, edge: HIDProtocol.Edge) {
        let size = malloc_size(msg)
        msg.storeBytes(of: Indigo.targetDigitizer, toByteOffset: 0x6c, as: UInt32.self)
        if size >= 0x110 {
            msg.storeBytes(of: Indigo.targetDigitizer, toByteOffset: 0x10c, as: UInt32.self)
        }
        let bit = Digitizer.edgeBit(edge)
        let present: UInt8 = bit == 0 ? 0 : 0x04
        msg.storeBytes(of: present, toByteOffset: 0x3a, as: UInt8.self)
        msg.storeBytes(of: bit, toByteOffset: 0x3b, as: UInt8.self)
        if size >= 0xdc {
            msg.storeBytes(of: present, toByteOffset: 0xda, as: UInt8.self)
            msg.storeBytes(of: bit, toByteOffset: 0xdb, as: UInt8.self)
        }
    }
}
