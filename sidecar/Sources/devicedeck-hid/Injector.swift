import Foundation
import CoreGraphics
import HIDProtocol

/// Maps decoded protocol frames onto the right dispatch path: digitizer
/// for single-finger touch, the 9-arg mouse builder for two-finger, and
/// the Indigo button/keyboard builders for everything else.
final class Injector {
    private let kit: SimKit
    private let client: HIDClient
    private let digitizer: Digitizer

    /// Identifier threading one touch sequence through the HID stack.
    /// Reset on `down`, reused until `up` — reusing across distinct
    /// touches confuses iOS's gesture recognizers.
    private var touchIdentifier: UInt32 = 0

    // Two-finger direction values for the 9-arg mouse builder (distinct
    // from NSEventType): 1=down, 0=move, 2=up.
    private static let twoFinger: [TouchPhase: (event: UInt32, dir: UInt32)] = [
        .down: (1, 1), .move: (6, 0), .up: (2, 2),
    ]

    init(kit: SimKit, client: HIDClient) {
        self.kit = kit
        self.client = client
        self.digitizer = Digitizer(kit: kit, client: client)
    }

    /// Execute one frame. Failures log and drop — the stream must survive
    /// an individual bad injection.
    func handle(_ frame: Frame) {
        switch frame {
        case let .touch(phase, x, y, edge):
            if phase == .down { touchIdentifier &+= 1; if touchIdentifier == 0 { touchIdentifier = 1 } }
            digitizer.send(x: x, y: y, phase: phase, edge: edge, identifier: max(touchIdentifier, 1))
        case let .twoFinger(phase, x1, y1, x2, y2):
            sendTwoFinger(phase: phase, x1: x1, y1: y1, x2: x2, y2: y2)
        case let .buttonPress(page, usage):
            sendArbitrary(page: page, usage: usage, op: Indigo.opDown)
            usleep(50_000)
            sendArbitrary(page: page, usage: usage, op: Indigo.opUp)
        case let .buttonDown(page, usage):
            sendArbitrary(page: page, usage: usage, op: Indigo.opDown)
        case let .buttonUp(page, usage):
            sendArbitrary(page: page, usage: usage, op: Indigo.opUp)
        case let .legacyButton(code):
            sendLegacy(code: code)
        case let .key(modifiers, usage):
            sendKey(modifiers: modifiers, usage: usage)
        case let .gesture(kind):
            runGesture(kind)
        }
    }

    // MARK: - two-finger (pinch / two-finger pan)

    /// The builder returns nil for ~60ms after a two-finger down while
    /// SimulatorKit settles multi-touch state — retry 12 × 5ms.
    private func sendTwoFinger(phase: TouchPhase, x1: Double, y1: Double,
                               x2: Double, y2: Double) {
        guard let fn = kit.mouseTwoFinger, let shape = Injector.twoFinger[phase] else {
            log("two-finger path unavailable")
            return
        }
        var p1 = CGPoint(x: x1, y: y1)
        var p2 = CGPoint(x: x2, y: y2)
        for _ in 0..<12 {
            let msg = withUnsafePointer(to: &p1) { p1Ref in
                withUnsafePointer(to: &p2) { p2Ref in
                    fn(p1Ref, p2Ref, Indigo.targetDigitizer, shape.event, shape.dir,
                       1.0, 1.0, 1.0, 1.0)
                }
            }
            if let msg { client.send(msg); return }
            usleep(5_000)
        }
        log("two-finger message build failed (phase=\(phase))")
    }

    // MARK: - buttons

    /// Volume / power / action / mute — any (page, usage) HID event.
    private func sendArbitrary(page: UInt32, usage: UInt32, op: UInt32) {
        guard let fn = kit.hidArbitrary else {
            log("HIDArbitrary unavailable (page=\(page) usage=\(usage))")
            return
        }
        guard let msg = fn(Indigo.targetDigitizer, page, usage, op) else { return }
        client.send(msg)
    }

    /// Home (0) / lock (1) via the legacy button service. SpringBoard
    /// honors the home event source even on Face ID devices.
    private func sendLegacy(code: UInt32) {
        guard let fn = kit.legacyButton else {
            log("legacy button unavailable (code=\(code))")
            return
        }
        if let down = fn(code, Indigo.opDown, Indigo.targetButton) { client.send(down) }
        usleep(50_000)
        if let up = fn(code, Indigo.opUp, Indigo.targetButton) { client.send(up) }
    }

    // MARK: - keyboard

    /// Modifier-down → key-down → key-up → modifier-up. Primary path is the
    /// keyboard-service builder (iOS treats events as real hardware keys);
    /// fallback routes page 0x07 through HIDArbitrary on older Xcodes.
    private func sendKey(modifiers: UInt8, usage: UInt32) {
        // USB HID modifier bitmap → modifier key usages 0xE0–0xE7.
        let held = (0..<8).filter { modifiers & (1 << $0) != 0 }.map { UInt32(0xE0 + $0) }
        for mod in held { sendKeyEvent(usage: mod, op: Indigo.opDown) }
        sendKeyEvent(usage: usage, op: Indigo.opDown)
        sendKeyEvent(usage: usage, op: Indigo.opUp)
        for mod in held.reversed() { sendKeyEvent(usage: mod, op: Indigo.opUp) }
    }

    private func sendKeyEvent(usage: UInt32, op: UInt32) {
        if let fn = kit.keyboard {
            if let msg = fn(usage, op) { client.send(msg) }
        } else if let fn = kit.hidArbitrary {
            if let msg = fn(Indigo.targetDigitizer, 0x07, usage, op) { client.send(msg) }
        } else {
            log("no keyboard HID path available")
        }
    }

    // MARK: - canned system gestures

    /// Timings verified empirically by baguette on iOS 26: a fast bottom-edge
    /// flick reads as Home; a slow drag with a long dwell reads as App
    /// Switcher; top-left vs top-right pulls select cover sheet vs
    /// Notification Center.
    private func runGesture(_ kind: GestureKind) {
        touchIdentifier &+= 1
        let id = max(touchIdentifier, 1)
        switch kind {
        case .swipeToHome:
            digitizer.swipe(from: CGPoint(x: 0.5, y: 0.998), to: CGPoint(x: 0.5, y: 0.30),
                            steps: 12, stepMs: 16, dwellMs: 0, edge: .bottom, identifier: id)
        case .appSwitcher:
            digitizer.swipe(from: CGPoint(x: 0.5, y: 0.998), to: CGPoint(x: 0.5, y: 0.58),
                            steps: 30, stepMs: 35, dwellMs: 900, edge: .bottom, identifier: id)
        case .notificationCenter:
            digitizer.swipe(from: CGPoint(x: 0.75, y: 0.002), to: CGPoint(x: 0.75, y: 0.55),
                            steps: 24, stepMs: 25, dwellMs: 0, edge: .top, identifier: id)
        case .lockScreen:
            digitizer.swipe(from: CGPoint(x: 0.25, y: 0.002), to: CGPoint(x: 0.25, y: 0.55),
                            steps: 24, stepMs: 25, dwellMs: 0, edge: .top, identifier: id)
        }
    }
}
