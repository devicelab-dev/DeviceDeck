import Foundation
import ObjectiveC
import VideoCore

/// The two `SimScreen` protocol methods a framebuffer display descriptor
/// answers, declared so Swift sends them with `objc_msgSend`. Nothing
/// conforms to this: a descriptor is reinterpreted as it only after
/// `ScreenCallbacks` has checked it answers both selectors.
@objc private protocol SimScreenMethods {
    /// Starts frame / surfaces-changed / properties-changed notifications,
    /// delivered on `callbackQueue`, keyed by `uuid` for unregistering.
    @objc(registerScreenCallbacksWithUUID:callbackQueue:frameCallback:surfacesChangedCallback:propertiesChangedCallback:)
    func register(
        uuid: NSUUID,
        callbackQueue: DispatchQueue,
        frameCallback: @escaping @convention(block) () -> Void,
        surfacesChangedCallback: @escaping @convention(block) () -> Void,
        propertiesChangedCallback: @escaping @convention(block) () -> Void
    )

    /// Stops the notifications registered under `uuid`.
    @objc(unregisterScreenCallbacksWithUUID:)
    func unregister(uuid: NSUUID)
}

/// SimulatorKit's frame notifications on the display descriptor — the push
/// alternative to polling `framebufferSurface`, which is a synchronous XPC
/// round-trip to CoreSimulatorService on every call. With callbacks the
/// sidecar asks for the surface only when the guest actually composited.
/// Recipe from baguette's SimulatorKitScreen (commit 41648ad) — see
/// ATTRIBUTION.md.
///
/// How the API is resolved safely: the descriptor is a ROCKit remote proxy
/// (`ROCKRemoteProxy`) whose methods reach CoreSimulatorService through
/// `forwardInvocation:`, so `class_getInstanceMethod` on its class never
/// finds them, and `class_getMethodImplementation` would hand back a
/// forwarding trampoline that crashes if the selector is truly gone. The
/// gate is therefore two checks, both before any call: CoreSimDeviceIO's
/// `SimScreen` protocol must declare both selectors with the argument
/// shapes this file sends (`isDeclared`), and the live descriptor must
/// answer `respondsToSelector:` for them (`bind`). Either failing means the
/// caller falls back to polling.
///
/// Only the capture queue touches an instance, and the callbacks are
/// delivered on that same queue, so no locking is needed.
final class ScreenCallbacks {
    /// The registration selector, as CoreSimDeviceIO names it.
    static let registerSelector = NSSelectorFromString(
        "registerScreenCallbacksWithUUID:callbackQueue:frameCallback:" +
            "surfacesChangedCallback:propertiesChangedCallback:")
    /// The matching unregistration selector.
    static let unregisterSelector = NSSelectorFromString("unregisterScreenCallbacksWithUUID:")

    private let queue: DispatchQueue
    private let onFrame: () -> Void
    private var bound: (descriptor: NSObject, uuid: NSUUID)?

    /// `onFrame` runs on `queue` whenever the guest produced a frame or
    /// swapped its surfaces (a rotation replaces them).
    init(queue: DispatchQueue, onFrame: @escaping () -> Void) {
        self.queue = queue
        self.onFrame = onFrame
    }

    /// Whether this Xcode's CoreSimulator declares the callback API with the
    /// signatures this file calls. Checked once, before any descriptor
    /// exists, to choose between callback and polling capture.
    static func isDeclared() -> Bool {
        guard let proto = objc_getProtocol("SimScreen") else { return false }
        return declares(proto, registerSelector, shape: "v@:@@@?@?@?")
            && declares(proto, unregisterSelector, shape: "v@:@")
    }

    /// Moves the registration to `descriptor`, dropping any earlier one.
    /// Returns `false`, registering nothing, when the descriptor does not
    /// answer the callback selectors.
    func bind(to descriptor: NSObject) -> Bool {
        if let bound, bound.descriptor === descriptor { return true }
        unbind()
        guard descriptor.responds(to: Self.registerSelector),
              descriptor.responds(to: Self.unregisterSelector) else { return false }
        let uuid = NSUUID()
        let fire: @convention(block) () -> Void = { [weak self] in self?.onFrame() }
        let ignore: @convention(block) () -> Void = {}
        methods(of: descriptor).register(
            uuid: uuid, callbackQueue: queue,
            frameCallback: fire, surfacesChangedCallback: fire, propertiesChangedCallback: ignore)
        bound = (descriptor, uuid)
        return true
    }

    /// Stops notifications from the bound descriptor, if any. `bind` only
    /// binds descriptors that answered the unregister selector.
    func unbind() {
        guard let bound else { return }
        methods(of: bound.descriptor).unregister(uuid: bound.uuid)
        self.bound = nil
    }

    private func methods(of descriptor: NSObject) -> SimScreenMethods {
        unsafeBitCast(descriptor, to: SimScreenMethods.self)
    }

    private static func declares(_ proto: Protocol, _ sel: Selector, shape: String) -> Bool {
        let desc = protocol_getMethodDescription(proto, sel, true, true)
        guard let types = desc.types else { return false }
        return MethodShape.matches(String(cString: types), expected: shape)
    }
}
