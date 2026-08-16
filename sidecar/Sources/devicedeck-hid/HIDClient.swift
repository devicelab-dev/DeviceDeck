import Foundation
import ObjectiveC
import SimCore

/// Wraps a SimulatorKit `SimDeviceLegacyHIDClient` bound to one simulator.
/// All Indigo messages — touch, button, key — go out through `send`.
final class HIDClient {
    private let client: NSObject
    private let sendFn: SendFn
    private let sendSel = NSSelectorFromString("sendWithMessage:freeWhenDone:completionQueue:completion:")

    private typealias SendFn = @convention(c) (
        NSObject, Selector, UnsafeMutableRawPointer, ObjCBool, AnyObject?, AnyObject?
    ) -> Void

    /// Resolve the device, create the HID client, and warm the pointer +
    /// mouse services (Simulator.app registers them before its first touch;
    /// so do we). Returns nil with a log on any failure.
    init?(udid: String, kit: SimKit) {
        guard let device = CoreSim.resolveDevice(udid: udid, developerDir: kit.developerDir) else {
            log("device not found (udid=\(udid))")
            return nil
        }
        guard let client = HIDClient.makeClient(device: device) else { return nil }
        guard let method = class_getInstanceMethod(type(of: client), sendSel) else {
            log("sendWithMessage: selector missing on SimDeviceLegacyHIDClient")
            return nil
        }
        self.client = client
        self.sendFn = unsafeBitCast(method_getImplementation(method), to: SendFn.self)
        warmServices(kit: kit)
    }

    /// Dispatch one Indigo message; ownership transfers (freeWhenDone).
    func send(_ message: UnsafeMutableRawPointer) {
        sendFn(client, sendSel, message, ObjCBool(true), nil, nil)
    }

    // MARK: - private

    /// The pointer and mouse HID services must exist guest-side before
    /// messages routed at them are meaningful; 20ms settle matches
    /// Simulator.app's observed behavior.
    private func warmServices(kit: SimKit) {
        for create in [kit.createPointerService, kit.createMouseService] {
            guard let create, let msg = create() else { continue }
            send(msg)
            usleep(20_000)
        }
    }

    private static func makeClient(device: NSObject) -> NSObject? {
        guard let cls = NSClassFromString("_TtC12SimulatorKit24SimDeviceLegacyHIDClient") else {
            log("SimDeviceLegacyHIDClient class not found")
            return nil
        }
        guard let allocated = ObjC.alloc(cls) else { return nil }
        let initSel = NSSelectorFromString("initWithDevice:error:")
        guard let method = class_getInstanceMethod(type(of: allocated), initSel) else {
            log("initWithDevice:error: missing")
            return nil
        }
        typealias InitFn = @convention(c) (
            NSObject, Selector, NSObject, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> NSObject?
        var err: NSError?
        let made = unsafeBitCast(method_getImplementation(method), to: InitFn.self)(
            allocated, initSel, device, &err)
        if let err { log("SimDeviceLegacyHIDClient init failed: \(err.localizedDescription)") }
        return made
    }
}
