import Foundation

/// Phase of a touch sequence: initial press, position update, or lift.
public enum TouchPhase: Equatable {
    case down, move, up
}

/// Screen edge a touch is flagged with. Edge-flagged touches route to
/// iOS's system gesture recognizers (home indicator, status bar) instead
/// of the foreground app's pan handlers.
public enum Edge: UInt8 {
    case none = 0, left = 1, top = 2, right = 3, bottom = 4
}

/// Canned system gestures the sidecar performs as a whole recipe —
/// step counts, timing, and dwell are tuned per gesture and would be
/// wasteful to stream frame-by-frame from the server.
public enum GestureKind: UInt8 {
    case swipeToHome = 1, appSwitcher = 2, notificationCenter = 3, lockScreen = 4
}

/// One decoded stdin frame. The wire format is a 1-byte type followed by
/// a fixed-length payload per type (see `payloadLength(forType:)`):
///
///   0x01–0x03  touch down/move/up   [x:f32BE][y:f32BE][edge:u8]        9 bytes
///   0x04–0x06  2-finger down/move/up [x1][y1][x2][y2] all f32BE       16 bytes
///   0x07       button press         [page:u32BE][usage:u32BE]          8 bytes
///   0x08       button down          [page:u32BE][usage:u32BE]          8 bytes
///   0x09       button up            [page:u32BE][usage:u32BE]          8 bytes
///   0x0A       legacy button        [code:u32BE]                       4 bytes
///   0x0B       key press            [modifiers:u8][usage:u32BE]        5 bytes
///   0x0C       gesture              [kind:u8]                          1 byte
///
/// Coordinates are normalized 0–1. Key modifiers use the USB HID modifier
/// bitmap (bit0=LeftCtrl, bit1=LeftShift, bit2=LeftAlt, bit3=LeftGUI, …).
public enum Frame: Equatable {
    case touch(TouchPhase, x: Double, y: Double, edge: Edge)
    case twoFinger(TouchPhase, x1: Double, y1: Double, x2: Double, y2: Double)
    case buttonPress(page: UInt32, usage: UInt32)
    case buttonDown(page: UInt32, usage: UInt32)
    case buttonUp(page: UInt32, usage: UInt32)
    case legacyButton(code: UInt32)
    case key(modifiers: UInt8, usage: UInt32)
    case gesture(GestureKind)

    /// Payload byte count for a frame type, or nil for an unknown type.
    /// The reader uses this to know how much to pull off stdin before parsing.
    public static func payloadLength(forType type: UInt8) -> Int? {
        switch type {
        case 0x01...0x03: return 9
        case 0x04...0x06: return 16
        case 0x07...0x09: return 8
        case 0x0A: return 4
        case 0x0B: return 5
        case 0x0C: return 1
        default: return nil
        }
    }

    /// Decode a frame from its type byte and exact-length payload.
    /// Returns nil for unknown types, wrong payload lengths, or payloads
    /// carrying out-of-range enum values — the caller drops such frames.
    public static func parse(type: UInt8, payload: Data) -> Frame? {
        guard payload.count == payloadLength(forType: type) else { return nil }
        switch type {
        case 0x01...0x03:
            guard let edge = Edge(rawValue: payload[payload.startIndex + 8]) else { return nil }
            return .touch(phase(for: type, base: 0x01),
                          x: payload.f32BE(at: 0), y: payload.f32BE(at: 4), edge: edge)
        case 0x04...0x06:
            return .twoFinger(phase(for: type, base: 0x04),
                              x1: payload.f32BE(at: 0), y1: payload.f32BE(at: 4),
                              x2: payload.f32BE(at: 8), y2: payload.f32BE(at: 12))
        case 0x07:
            return .buttonPress(page: payload.u32BE(at: 0), usage: payload.u32BE(at: 4))
        case 0x08:
            return .buttonDown(page: payload.u32BE(at: 0), usage: payload.u32BE(at: 4))
        case 0x09:
            return .buttonUp(page: payload.u32BE(at: 0), usage: payload.u32BE(at: 4))
        case 0x0A:
            return .legacyButton(code: payload.u32BE(at: 0))
        case 0x0B:
            return .key(modifiers: payload[payload.startIndex], usage: payload.u32BE(at: 1))
        case 0x0C:
            guard let kind = GestureKind(rawValue: payload[payload.startIndex]) else { return nil }
            return .gesture(kind)
        default:
            return nil
        }
    }

    private static func phase(for type: UInt8, base: UInt8) -> TouchPhase {
        switch type - base {
        case 0: return .down
        case 1: return .move
        default: return .up
        }
    }
}

extension Data {
    /// Read a big-endian float32 at `offset` (relative to startIndex) as Double.
    func f32BE(at offset: Int) -> Double {
        Double(Float(bitPattern: u32BE(at: offset)))
    }

    /// Read a big-endian uint32 at `offset` (relative to startIndex).
    func u32BE(at offset: Int) -> UInt32 {
        let start = startIndex + offset
        return subdata(in: start..<start + 4).withUnsafeBytes {
            $0.load(as: UInt32.self).bigEndian
        }
    }
}
