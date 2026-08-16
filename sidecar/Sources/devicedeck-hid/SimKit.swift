import Foundation
import CoreGraphics
import SimCore

// Function shapes of the private SimulatorKit / IOKit C entry points.
// Signatures were reverse-engineered by the baguette and tapflow projects
// (see ATTRIBUTION.md) and verified against Xcode 26:
//
//  - IndigoHIDMessageForMouseNSEvent, 9-arg form: two-finger touch only.
//    direction: 1=down, 0=move, 2=up; trailing doubles fill d0–d3.
//  - IndigoHIDMessageForHIDArbitrary is (target, page, usage, op) on
//    iOS 26 — NOT (page, usage, op, timestamp) as some bridges assume.
//  - IndigoHIDMessageForButton is (code, op, target); target 0x33 is the
//    legacy button service. Release op must be 2 — 0 crashes backboardd.
typealias MouseTwoFingerFn = @convention(c) (
    UnsafePointer<CGPoint>, UnsafePointer<CGPoint>, UInt32, UInt32, UInt32,
    Double, Double, Double, Double
) -> UnsafeMutableRawPointer?
typealias HIDArbitraryFn = @convention(c) (UInt32, UInt32, UInt32, UInt32) -> UnsafeMutableRawPointer?
typealias LegacyButtonFn = @convention(c) (UInt32, UInt32, UInt32) -> UnsafeMutableRawPointer?
typealias KeyboardFn = @convention(c) (UInt32, UInt32) -> UnsafeMutableRawPointer?
typealias ServiceFn = @convention(c) () -> UnsafeMutableRawPointer?
typealias CreateDigitizerFn = @convention(c) (
    CFAllocator?, UInt64, UInt32,
    UInt32, UInt32, UInt32, UInt32,
    Double, Double, Double, Double, Double,
    Bool, Bool, UInt32
) -> Unmanaged<CFTypeRef>?
typealias CreateFingerFn = @convention(c) (
    CFAllocator?, UInt64,
    UInt32, UInt32, UInt32,
    Double, Double, Double, Double, Double,
    Bool, Bool, UInt32
) -> Unmanaged<CFTypeRef>?
typealias AppendEventFn = @convention(c) (CFTypeRef, CFTypeRef, UInt32) -> Void
typealias TrackpadWrapFn = @convention(c) (UnsafeRawPointer) -> UnsafeMutableRawPointer?

/// Indigo wire constants shared across dispatch paths.
enum Indigo {
    /// Touch digitizer routing target — the service Simulator.app itself uses.
    static let targetDigitizer: UInt32 = 0x32
    /// Legacy button service target (home / lock).
    static let targetButton: UInt32 = 0x33
    static let opDown: UInt32 = 1
    static let opUp: UInt32 = 2
}

/// Loads CoreSimulator + SimulatorKit and resolves the C entry points.
/// Optional fields are capabilities a given Xcode may not ship; dispatch
/// paths degrade (and log) rather than abort when one is missing.
struct SimKit {
    let developerDir: String
    let mouseTwoFinger: MouseTwoFingerFn?
    let hidArbitrary: HIDArbitraryFn?
    let legacyButton: LegacyButtonFn?
    let keyboard: KeyboardFn?
    let createPointerService: ServiceFn?
    let createMouseService: ServiceFn?
    let createDigitizer: CreateDigitizerFn?
    let createFinger: CreateFingerFn?
    let appendEvent: AppendEventFn?
    let trackpadWrap: TrackpadWrapFn?

    /// Whether the iOS 26-safe digitizer path is fully available. Without
    /// it there is no tap path — the mouse-event fallback regressed on
    /// Xcode 26 (taps drop or become Home gestures), so we require it.
    var hasDigitizerPath: Bool {
        createDigitizer != nil && createFinger != nil && appendEvent != nil && trackpadWrap != nil
    }

    /// dlopen both frameworks and resolve every symbol, or exit: a sidecar
    /// that cannot inject anything has no reason to keep the pipe open.
    static func load() -> SimKit {
        let dev = DeveloperDir.find()
        CoreSim.load()
        guard let kit = dlopen(DeveloperDir.simulatorKitPath(dev), RTLD_NOW | RTLD_GLOBAL) else {
            fatalStartup("SimulatorKit dlopen failed: \(dlerrorString())")
        }
        // IOKit symbols live in the dyld shared cache; an explicit handle
        // avoids RTLD_DEFAULT, which Swift cannot import.
        let ioKit = dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", RTLD_NOW | RTLD_GLOBAL)

        func sym<T>(_ handle: UnsafeMutableRawPointer?, _ name: String, as _: T.Type) -> T? {
            dlsym(handle, name).map { unsafeBitCast($0, to: T.self) }
        }
        return SimKit(
            developerDir: dev,
            mouseTwoFinger: sym(kit, "IndigoHIDMessageForMouseNSEvent", as: MouseTwoFingerFn.self),
            hidArbitrary: sym(kit, "IndigoHIDMessageForHIDArbitrary", as: HIDArbitraryFn.self),
            legacyButton: sym(kit, "IndigoHIDMessageForButton", as: LegacyButtonFn.self),
            keyboard: sym(kit, "IndigoHIDMessageForKeyboardArbitrary", as: KeyboardFn.self),
            createPointerService: sym(kit, "IndigoHIDMessageToCreatePointerService", as: ServiceFn.self),
            createMouseService: sym(kit, "IndigoHIDMessageToCreateMouseService", as: ServiceFn.self),
            createDigitizer: sym(ioKit, "IOHIDEventCreateDigitizerEvent", as: CreateDigitizerFn.self),
            createFinger: sym(ioKit, "IOHIDEventCreateDigitizerFingerEvent", as: CreateFingerFn.self),
            appendEvent: sym(ioKit, "IOHIDEventAppendEvent", as: AppendEventFn.self),
            trackpadWrap: sym(kit, "IndigoHIDMessageForTrackpadEventFromHIDEventRef", as: TrackpadWrapFn.self)
        )
    }
}
