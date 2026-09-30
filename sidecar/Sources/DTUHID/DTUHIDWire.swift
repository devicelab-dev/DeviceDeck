import Foundation
import HIDProtocol
import XPC

// Wire format of `dtuhidd`, the input daemon Xcode 27 (CoreSimulator 1155.4)
// puts inside every simulator. Once it is active the guest drops touch and
// keyboard events sent the legacy Indigo way, so input must travel as plain
// XPC dictionaries to its digitizer service instead. Message shapes, value
// types and the version gate follow facebook/idb's `DTUHIDModels.swift`,
// `SimulatorDigitizerHIDTransport.swift` and
// `SimulatorHIDTransportSelection.swift` (MIT — see ATTRIBUTION.md).

/// Builds the XPC messages `dtuhidd`'s digitizer service decodes, and decides
/// when a toolchain has the daemon at all. Pure: no connection, no device.
public enum DTUHIDWire {

    /// The guest service that takes touch, keyboard and button events.
    public static let digitizerService = "com.apple.coredevice.feature.remote.hid.digitizer"

    /// The first CoreSimulator version that injects `dtuhidd` into the guest.
    public static let firstCoreSimulatorVersion = "1155.4"

    /// Whether a CoreSimulator version ships `dtuhidd`. Compared numerically,
    /// so 1155.10 is newer than 1155.4. Nil (not loaded) means no.
    public static func ships(coreSimulatorVersion: String?) -> Bool {
        guard let version = coreSimulatorVersion, !version.isEmpty else { return false }
        return version.compare(firstCoreSimulatorVersion, options: .numeric) != .orderedAscending
    }

    /// The daemon's edge value for a protocol edge. Its numbering (top 1,
    /// left 2, bottom 3, right 4) differs from the frame protocol's.
    public static func edge(_ edge: Edge) -> UInt64 {
        switch edge {
        case .none: return 0
        case .top: return 1
        case .left: return 2
        case .bottom: return 3
        case .right: return 4
        }
    }

    /// The per-contact phase: start, position, end. The frame protocol's
    /// explicit down/move/up maps onto it directly.
    public static func eventType(_ phase: TouchPhase) -> UInt64 {
        switch phase {
        case .down: return 0
        case .move: return 1
        case .up: return 2
        }
    }

    /// One-finger touch at a normalized (0–1, top-left) point.
    public static func touch(x: Double, y: Double, phase: TouchPhase, edge: Edge) -> xpc_object_t {
        let payload = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_value(payload, "pointOne", point(x: x, y: y))
        xpc_dictionary_set_uint64(payload, "eventType", eventType(phase))
        xpc_dictionary_set_uint64(payload, "edge", DTUHIDWire.edge(edge))
        xpc_dictionary_set_uint64(payload, "target", 0)
        return envelope("IndigoDigitizerEvent", payload: payload)
    }

    /// Two-finger touch (pinch, two-finger pan) at normalized points.
    public static func twoFinger(x1: Double, y1: Double, x2: Double, y2: Double,
                                 phase: TouchPhase) -> xpc_object_t {
        let payload = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_value(payload, "pointOne", point(x: x1, y: y1))
        xpc_dictionary_set_value(payload, "pointTwo", point(x: x2, y: y2))
        xpc_dictionary_set_uint64(payload, "eventType", eventType(phase))
        xpc_dictionary_set_uint64(payload, "edge", 0)
        xpc_dictionary_set_uint64(payload, "target", 0)
        return envelope("IndigoDigitizerEvent", payload: payload)
    }

    /// A keyboard key (USB HID usage, page 0x07) going down or up.
    public static func key(usage: UInt32, down: Bool) -> xpc_object_t {
        let payload = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_uint64(payload, "usageCode", UInt64(usage))
        xpc_dictionary_set_uint64(payload, "state", buttonState(down))
        return envelope("IndigoKeyboardButtonEvent", payload: payload)
    }

    /// A hardware button by HID usage page and usage (volume, power, …).
    public static func button(page: UInt32, usage: UInt32, down: Bool) -> xpc_object_t {
        let payload = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_uint64(payload, "usagePage", UInt64(page))
        xpc_dictionary_set_uint64(payload, "usageCode", UInt64(usage))
        xpc_dictionary_set_uint64(payload, "state", buttonState(down))
        return envelope("IndigoButtonEvent", payload: payload)
    }

    /// The liveness probe: a barrier key event with usage 0 ("no event"), so
    /// the daemon answers without the guest seeing a keypress. A reply is the
    /// only proof a daemon is behind the service — sends to a dead one report
    /// no error.
    public static func barrier() -> xpc_object_t {
        let payload = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_uint64(payload, "usageCode", 0)
        xpc_dictionary_set_uint64(payload, "state", buttonState(false))
        return envelope("IndigoKeyboardButtonEvent", payload: payload, isBarrier: true)
    }

    // MARK: - private

    /// The daemon's button state is 1-based: down 1, up 2 (0 is rejected).
    private static func buttonState(_ down: Bool) -> UInt64 { down ? 1 : 2 }

    private static func point(x: Double, y: Double) -> xpc_object_t {
        let p = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_double(p, "x", x)
        xpc_dictionary_set_double(p, "y", y)
        return p
    }

    /// The envelope every message shares; `featureIdentifier` is the service
    /// the message is sent to.
    private static func envelope(_ type: String, payload: xpc_object_t, isBarrier: Bool = false) -> xpc_object_t {
        let message = xpc_dictionary_create(nil, nil, 0)
        xpc_dictionary_set_string(message, "messageType", type)
        xpc_dictionary_set_bool(message, "isBarrier", isBarrier)
        xpc_dictionary_set_string(message, "featureIdentifier", digitizerService)
        xpc_dictionary_set_value(message, "payload", payload)
        return message
    }
}
