import Foundation
import IOSurface
import SimCore

/// Fetches the simulator's live framebuffer IOSurface. The device's IO
/// ports expose `com.apple.framebuffer.display` entries whose descriptor
/// carries the surface the guest renders into; we re-fetch per frame so
/// rotation or surface replacement is picked up automatically. Recipe from
/// baguette's SimulatorKitFramebufferPorts — see ATTRIBUTION.md.
struct Framebuffer {
    private let device: NSObject

    init(device: NSObject) {
        self.device = device
    }

    /// The current main-display surface: the largest live framebuffer
    /// surface (phone display; external planes are out of scope for v1).
    /// When no framebuffer ports exist yet — headless boot, nothing has
    /// asked the guest for a display — `updateIOPorts` makes CoreSimulator
    /// materialize them (once; existing ports must not be refreshed out
    /// from under the guest).
    func currentSurface() -> IOSurface? {
        guard let io = ioObject() else { return nil }
        var ports = framebufferPorts(on: io)
        if ports.isEmpty {
            io.perform(NSSelectorFromString("updateIOPorts"))
            ports = framebufferPorts(on: io)
        }
        var best: IOSurface?
        var bestArea = 0
        for port in ports {
            guard let surface = surface(of: port) else { continue }
            let area = IOSurfaceGetWidth(surface) * IOSurfaceGetHeight(surface)
            if area > bestArea {
                best = surface
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

    private func surface(of port: NSObject) -> IOSurface? {
        let descSel = NSSelectorFromString("descriptor")
        let surfSel = NSSelectorFromString("framebufferSurface")
        guard port.responds(to: descSel),
              let desc = port.perform(descSel)?.takeUnretainedValue() as? NSObject,
              desc.responds(to: surfSel),
              let surfObj = desc.perform(surfSel)?.takeUnretainedValue() else { return nil }
        let surface = unsafeBitCast(surfObj, to: IOSurface.self)
        guard IOSurfaceGetWidth(surface) > 0, IOSurfaceGetHeight(surface) > 0 else { return nil }
        return surface
    }
}
