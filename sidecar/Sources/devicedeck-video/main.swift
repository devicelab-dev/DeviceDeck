import Foundation
import IOSurface
import SimCore

// devicedeck-video — DeviceDeck's screen capture sidecar.
//
// Usage: devicedeck-video <udid|booted> [fps]
//
// Captures the simulator's framebuffer IOSurface, encodes H.264 (AVCC),
// and writes framed messages to stdout:
//
//   [type:u8][length:u32BE][payload]
//     type 1 = avcC decoder description (once per encoder session)
//     type 2 = keyframe AVCC sample
//     type 3 = delta AVCC sample
//
// stdin accepts single-byte commands: 'K' forces the next frame to be a
// keyframe (the server sends it when a new viewer joins so they can start
// decoding without waiting out the keyframe interval). Exits 0 on stdin
// EOF. Unchanged frames are skipped via IOSurface seed comparison.

let arguments = CommandLine.arguments
guard arguments.count >= 2 else {
    fatalStartup("usage: devicedeck-video <udid|booted> [fps]")
}
let udid = arguments[1]
let fps = arguments.count > 2 ? max(1, min(60, Int(arguments[2]) ?? 30)) : 30

CoreSim.load()
let developerDir = DeveloperDir.find()
guard let device = CoreSim.resolveDevice(udid: udid, developerDir: developerDir) else {
    fatalStartup("device not found (udid=\(udid))")
}
let framebuffer = Framebuffer(device: device)

/// Serializes framed writes to stdout — encoder output lands on VT's queue
/// while meta frames come from the capture loop.
final class FrameWriter: @unchecked Sendable {
    private let lock = NSLock()
    private let out = FileHandle.standardOutput

    func write(type: UInt8, payload: Data) {
        var frame = Data([type])
        var lenBE = UInt32(payload.count).bigEndian
        withUnsafeBytes(of: &lenBE) { frame.append(contentsOf: $0) }
        frame.append(payload)
        lock.lock()
        defer { lock.unlock() }
        do {
            try out.write(contentsOf: frame)
        } catch {
            // Parent closed the pipe — nothing left to stream to.
            exit(0)
        }
    }
}

let writer = FrameWriter()
let encoder = H264Encoder(fps: fps, bitrate: 4_000_000)
encoder.onEncoded = { encoded in
    if let description = encoded.description {
        writer.write(type: 1, payload: description)
    }
    writer.write(type: encoded.isKeyframe ? 2 : 3, payload: encoded.avcc)
}

/// Set from the stdin reader; consumed (and cleared) by the capture loop.
final class KeyframeRequest: @unchecked Sendable {
    private let lock = NSLock()
    private var pending = true // first frame is always a keyframe request

    func request() {
        lock.lock()
        pending = true
        lock.unlock()
    }

    func consume() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        let was = pending
        pending = false
        return was
    }
}

let keyframeRequest = KeyframeRequest()

// stdin command reader: 'K' → keyframe request; EOF → exit.
DispatchQueue.global().async {
    while true {
        let data = FileHandle.standardInput.readData(ofLength: 1)
        guard !data.isEmpty else { exit(0) }
        if data[0] == UInt8(ascii: "K") {
            keyframeRequest.request()
        }
    }
}

OrphanWatch.start()

log("capturing udid=\(udid) fps=\(fps)")

// Capture loop: poll the surface at the target rate, encode when content
// changed (seed moved) or a keyframe was requested.
let interval = 1.0 / Double(fps)
var lastSeed: UInt32 = 0
var lastSurfaceID: IOSurfaceID = 0
DispatchQueue.global(qos: .userInteractive).async {
    while true {
        let started = Date()
        if let surface = framebuffer.currentSurface() {
            let seed = IOSurfaceGetSeed(surface)
            let surfaceID = IOSurfaceGetID(surface)
            let force = keyframeRequest.consume() || surfaceID != lastSurfaceID
            if force || seed != lastSeed {
                encoder.encode(surface, forceKeyframe: force)
                lastSeed = seed
                lastSurfaceID = surfaceID
            }
        }
        let elapsed = Date().timeIntervalSince(started)
        if elapsed < interval {
            usleep(UInt32((interval - elapsed) * 1_000_000))
        }
    }
}

RunLoop.main.run()
