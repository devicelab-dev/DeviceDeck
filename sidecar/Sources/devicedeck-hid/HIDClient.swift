import Foundation
import ObjectiveC

/// Wraps a SimulatorKit `SimDeviceLegacyHIDClient` bound to one simulator.
/// All Indigo messages — touch, button, key — go out through `send`.
///
/// Objective-C methods are invoked via `class_getInstanceMethod` +
/// `method_getImplementation`, never `class_getMethodImplementation` alone:
/// Xcode 26 returns a non-NULL forwarding trampoline for removed selectors
/// that crashes with "unrecognized selector" when called.
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
        guard let device = HIDClient.resolveDevice(udid: udid, developerDir: kit.developerDir) else {
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

    private static func resolveDevice(udid: String, developerDir: String) -> NSObject? {
        guard let cls = NSClassFromString("SimServiceContext"),
              let ctx = invokeClass(cls, "sharedServiceContextForDeveloperDir:error:",
                                    with: developerDir as NSString),
              let set = invokeInstance(ctx, "defaultDeviceSetWithError:") else { return nil }
        let devices = (set.value(forKey: "availableDevices") as? [NSObject]) ?? []
        if udid == "booted" {
            // CoreSimulator state 3 = booted.
            return devices.first { ($0.value(forKey: "state") as? NSNumber)?.uintValue == 3 }
        }
        return devices.first { ($0.value(forKey: "UDID") as? NSUUID)?.uuidString == udid }
    }

    private static func makeClient(device: NSObject) -> NSObject? {
        guard let cls = NSClassFromString("_TtC12SimulatorKit24SimDeviceLegacyHIDClient") else {
            log("SimDeviceLegacyHIDClient class not found")
            return nil
        }
        guard let allocated = alloc(cls) else { return nil }
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

    private static func alloc(_ cls: AnyClass) -> NSObject? {
        let sel = NSSelectorFromString("alloc")
        guard let meta = object_getClass(cls),
              let method = class_getInstanceMethod(meta, sel) else { return nil }
        typealias AllocFn = @convention(c) (AnyClass, Selector) -> NSObject?
        return unsafeBitCast(method_getImplementation(method), to: AllocFn.self)(cls, sel)
    }

    /// Call a class method taking one object arg and an NSError out-param.
    private static func invokeClass(_ cls: AnyClass, _ selector: String,
                                    with arg: AnyObject) -> NSObject? {
        let sel = NSSelectorFromString(selector)
        guard let meta = object_getClass(cls),
              let method = class_getInstanceMethod(meta, sel) else { return nil }
        typealias Fn = @convention(c) (
            AnyClass, Selector, AnyObject, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> NSObject?
        var err: NSError?
        return unsafeBitCast(method_getImplementation(method), to: Fn.self)(cls, sel, arg, &err)
    }

    /// Call an instance method taking only an NSError out-param.
    private static func invokeInstance(_ obj: NSObject, _ selector: String) -> NSObject? {
        let sel = NSSelectorFromString(selector)
        guard let method = class_getInstanceMethod(type(of: obj), sel) else { return nil }
        typealias Fn = @convention(c) (
            NSObject, Selector, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> NSObject?
        var err: NSError?
        return unsafeBitCast(method_getImplementation(method), to: Fn.self)(obj, sel, &err)
    }
}
