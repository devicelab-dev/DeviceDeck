import Foundation
import ObjectiveC
import XPC
import DTUHID
import SimCore

// Host side of an XPC connection to `dtuhidd`'s digitizer service inside a
// simulator. The recipe — look the service's Mach port up in the simulator's
// bootstrap namespace, wrap it with the private
// `xpc_endpoint_create_mach_port_4sim`, and mark the connection
// simulator-to-host with `xpc_connection_enable_sim2host_4sim` (without which
// the service sees the peer but never a payload) — and the liveness timing
// follow facebook/idb's `SimulatorXPCConnection.swift` and
// `SimulatorDTUHIDConnection.swift` (MIT — see ATTRIBUTION.md).

/// One live connection to `dtuhidd`. Sends are fire-and-forget and keep
/// their order on the connection.
final class DTUHIDClient {
    private let connection: xpc_connection_t

    private init(connection: xpc_connection_t) {
        self.connection = connection
    }

    /// Send one message built by `DTUHIDWire`.
    func send(_ message: xpc_object_t) {
        xpc_connection_send_message(connection, message)
    }

    /// Close the connection.
    func close() {
        xpc_connection_cancel(connection)
    }

    // MARK: - connecting

    /// Timings from idb's measurements. The first probe demand-launches the
    /// daemon, so it pays its cold start; the backoff matches the daemon's
    /// 10s minimum runtime, which makes launchd throttle a respawn.
    enum Timing {
        static let livenessTimeout: DispatchTimeInterval = .seconds(4)
        static let replyTailMicros: UInt32 = 200_000
        static let backoffMicros: UInt32 = 4_000_000
    }

    /// Connect and prove a daemon answers, trying up to `attempts` times.
    /// Nil when the service cannot be reached — the caller keeps the legacy
    /// path, which works until the daemon takes input over.
    static func connect(device: NSObject, attempts: Int) -> DTUHIDClient? {
        for attempt in 1...max(attempts, 1) {
            if let client = attemptOnce(device: device) { return client }
            log("dtuhidd did not answer (attempt \(attempt) of \(attempts))")
            if attempt < attempts { usleep(Timing.backoffMicros) }
        }
        return nil
    }

    /// One attempt: a fresh connection each time, since a cancelled XPC
    /// connection cannot be resumed.
    private static func attemptOnce(device: NSObject) -> DTUHIDClient? {
        guard let connection = makeConnection(device: device) else { return nil }
        let client = DTUHIDClient(connection: connection)
        guard client.answersBarrier() else {
            client.close()
            return nil
        }
        usleep(Timing.replyTailMicros) // device-open and dispatch after activation
        return client
    }

    /// Round-trip the barrier probe. A reply that is an XPC error is no.
    private func answersBarrier() -> Bool {
        let done = DispatchSemaphore(value: 0)
        let answered = Flag()
        xpc_connection_send_message_with_reply(connection, DTUHIDWire.barrier(), nil) { reply in
            if xpc_get_type(reply) != XPC_TYPE_ERROR { answered.set() }
            done.signal()
        }
        return done.wait(timeout: .now() + Timing.livenessTimeout) == .success && answered.value
    }

    private static func makeConnection(device: NSObject) -> xpc_connection_t? {
        guard let sym = Sim2Host.load() else {
            log("dtuhidd: _4sim XPC symbols not in this process")
            return nil
        }
        guard let port = lookupPort(device: device, service: DTUHIDWire.digitizerService) else { return nil }
        // Both create functions return +1; the endpoint consumes the port's send right.
        guard let endpoint = sym.endpointFromPort(port, 0, 0)?.takeRetainedValue() as? xpc_object_t else {
            log("dtuhidd: endpoint from port failed")
            return nil
        }
        let connection = xpc_connection_create_from_endpoint(endpoint)
        sym.enableSim2Host(connection)
        xpc_connection_set_event_handler(connection) { event in
            if xpc_get_type(event) == XPC_TYPE_ERROR { log("dtuhidd connection event: \(event)") }
        }
        xpc_connection_resume(connection)
        return connection
    }

    /// `-[SimDevice lookup:error:]`: the service's Mach port in the
    /// simulator's bootstrap namespace, or nil.
    private static func lookupPort(device: NSObject, service: String) -> mach_port_t? {
        let sel = NSSelectorFromString("lookup:error:")
        guard let method = class_getInstanceMethod(type(of: device), sel) else {
            log("dtuhidd: SimDevice lookup:error: missing")
            return nil
        }
        typealias LookupFn = @convention(c) (
            NSObject, Selector, NSString, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> mach_port_t
        var err: NSError?
        let port = unsafeBitCast(method_getImplementation(method), to: LookupFn.self)(
            device, sel, service as NSString, &err)
        guard port != MACH_PORT_NULL else {
            log("dtuhidd: lookup \(service) failed: \(err?.localizedDescription ?? "no port")")
            return nil
        }
        return port
    }
}

/// The private simulator-to-host XPC entry points, resolved from the
/// process image.
private struct Sim2Host {
    typealias EndpointFromPort = @convention(c) (mach_port_t, UInt64, UInt64) -> Unmanaged<AnyObject>?
    typealias EnableSim2Host = @convention(c) (xpc_connection_t) -> Void

    let endpointFromPort: EndpointFromPort
    let enableSim2Host: EnableSim2Host

    static func load() -> Sim2Host? {
        guard let handle = dlopen(nil, RTLD_NOW),
              let endpoint = dlsym(handle, "xpc_endpoint_create_mach_port_4sim"),
              let enable = dlsym(handle, "xpc_connection_enable_sim2host_4sim") else { return nil }
        return Sim2Host(endpointFromPort: unsafeBitCast(endpoint, to: EndpointFromPort.self),
                        enableSim2Host: unsafeBitCast(enable, to: EnableSim2Host.self))
    }
}

/// A thread-safe one-way flag.
private final class Flag {
    private let lock = NSLock()
    private var isSet = false

    func set() { lock.lock(); isSet = true; lock.unlock() }

    var value: Bool { lock.lock(); defer { lock.unlock() }; return isSet }
}

/// Which transport carries touch and keys. Starts on the legacy path; once
/// `dtuhidd` answers, it switches for good. Read on the injection queue,
/// written by the background connect.
final class InputTransport {
    private let lock = NSLock()
    private var client: DTUHIDClient?

    /// The DTUHID connection when one is live.
    var dtuhid: DTUHIDClient? {
        lock.lock(); defer { lock.unlock() }
        return client
    }

    /// Adopt a live connection.
    func adopt(_ client: DTUHIDClient) {
        lock.lock(); self.client = client; lock.unlock()
    }

    /// The CoreSimulator version loaded in this process (e.g. "1155.4"), read
    /// from the bundle that vends SimDevice. The system framework can be newer
    /// than the selected Xcode, so this — not the Xcode version — decides.
    static var loadedCoreSimulatorVersion: String? {
        guard let cls = NSClassFromString("SimDevice") else { return nil }
        return Bundle(for: cls).infoDictionary?["CFBundleVersion"] as? String
    }
}
