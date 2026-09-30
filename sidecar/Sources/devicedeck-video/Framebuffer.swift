import Foundation
import IOSurface
import SimCore

/// Fetches the simulator's live framebuffer IOSurface. The device's IO
/// ports expose `com.apple.framebuffer.display` entries whose descriptor
/// carries the surface the guest renders into. Recipe from baguette's
/// SimulatorKitFramebufferPorts — see ATTRIBUTION.md.
///
/// The display's descriptor is resolved once and kept: walking the device's
/// IO ports and every port's descriptor calls into CoreSimulator, and doing
/// that on every poll (30 times a second, even on a still screen) is load the
/// simulator service does not need. The surface itself is still fetched from
/// the kept descriptor each poll — the simulator swaps buffers, and a
/// rotation replaces the surface — and a descriptor that stops yielding one
/// is dropped and resolved again. Only the capture queue uses it.
final class Framebuffer {
    private let device: NSObject
    private var display: NSObject?

    /// Called with each newly resolved display descriptor, so frame
    /// callbacks can follow the display when its descriptor is replaced.
    var onDisplayChange: ((NSObject) -> Void)?

    init(device: NSObject) {
        self.device = device
    }

    /// The current main-display surface, from the kept descriptor when it
    /// still yields one, else from a fresh resolve.
    func currentSurface() -> IOSurface? {
        if let display, let surface = surface(ofDescriptor: display) {
            return surface
        }
        display = resolveDisplay()
        guard let display else { return nil }
        onDisplayChange?(display)
        return surface(ofDescriptor: display)
    }

    /// The descriptor of the largest live framebuffer (the phone display;
    /// external planes are out of scope for v1). When no framebuffer ports
    /// exist yet — headless boot, nothing has asked the guest for a display —
    /// `updateIOPorts` makes CoreSimulator materialize them (once; existing
    /// ports must not be refreshed out from under the guest).
    private func resolveDisplay() -> NSObject? {
        guard let io = ioObject() else { return nil }
        var ports = framebufferPorts(on: io)
        if ports.isEmpty {
            io.perform(NSSelectorFromString("updateIOPorts"))
            ports = framebufferPorts(on: io)
        }
        var best: NSObject?
        var bestArea = 0
        for port in ports {
            guard let desc = descriptor(of: port), let surface = surface(ofDescriptor: desc) else { continue }
            let area = IOSurfaceGetWidth(surface) * IOSurfaceGetHeight(surface)
            if area > bestArea {
                best = desc
                bestArea = area
            }
        }
        return best
    }

    private func ioObject() -> NSObject? {
        let sel = NSSelectorFromString("io")
        guard device.responds(to: sel) else { return nil }
        return device.perform(sel)?.takeUnretainedValue() as? NSObject
    }

    private func framebufferPorts(on io: NSObject) -> [NSObject] {
        guard let ports = io.value(forKey: "deviceIOPorts") as? [NSObject] else { return [] }
        let pidSel = NSSelectorFromString("portIdentifier")
        return ports.filter { port in
            guard port.responds(to: pidSel),
                  let pid = port.perform(pidSel)?.takeUnretainedValue() else { return false }
            return "\(pid)" == "com.apple.framebuffer.display"
        }
    }

    private func descriptor(of port: NSObject) -> NSObject? {
        let descSel = NSSelectorFromString("descriptor")
        guard port.responds(to: descSel) else { return nil }
        return port.perform(descSel)?.takeUnretainedValue() as? NSObject
    }

    private func surface(ofDescriptor desc: NSObject) -> IOSurface? {
        let surfSel = NSSelectorFromString("framebufferSurface")
        guard desc.responds(to: surfSel),
              let surfObj = desc.perform(surfSel)?.takeUnretainedValue() else { return nil }
        let surface = unsafeBitCast(surfObj, to: IOSurface.self)
        guard IOSurfaceGetWidth(surface) > 0, IOSurfaceGetHeight(surface) > 0 else { return nil }
        return surface
    }
}
