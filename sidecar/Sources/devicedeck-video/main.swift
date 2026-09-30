import Foundation
import IOSurface
import SimCore

// devicedeck-video — DeviceDeck's screen capture sidecar.
//
// Usage: devicedeck-video <udid|booted> [fps]
//        devicedeck-video --stdin-frames [fps]   (Android: see StdinFrames.swift)
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
// EOF. Capture is driven by SimulatorKit frame callbacks when available,
// else by polling (see Capture.swift); unchanged frames are skipped via
// IOSurface seed comparison.

let arguments = CommandLine.arguments
guard arguments.count >= 2 else {
    fatalStartup("usage: devicedeck-video <udid|booted> [fps]  |  devicedeck-video --stdin-frames [fps]")
}
if arguments[1] == StdinFrames.flag {
    StdinFrames.run(fps: arguments.count > 2 ? max(1, min(60, Int(arguments[2]) ?? 30)) : 30)
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

let capture = CaptureDriver(framebuffer: framebuffer, encoder: encoder,
                            keyframes: KeyframeRequest(), fps: fps)

// stdin command reader: 'K' → keyframe request; EOF → exit.
DispatchQueue.global().async {
    while true {
        autoreleasepool {
            let data = FileHandle.standardInput.readData(ofLength: 1)
            guard !data.isEmpty else { exit(0) }
            if data[0] == UInt8(ascii: "K") {
                capture.requestKeyframe()
            }
        }
    }
}

OrphanWatch.start()

log("capturing udid=\(udid) fps=\(fps)")
capture.start()

RunLoop.main.run()
