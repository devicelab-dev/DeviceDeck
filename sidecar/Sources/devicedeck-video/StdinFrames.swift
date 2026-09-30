import Foundation
import Accelerate
import CoreVideo
import SimCore

/// `devicedeck-video --stdin-frames [fps]` — the encoder for Android.
///
/// The Android emulator's gRPC screenshot stream delivers raw RGB888 frames;
/// DeviceDeck pipes them in here so Android gets the same H.264 stream, and
/// the same encoder, as iOS instead of a PNG per frame. Output is the usual
/// framed stream on stdout (types 1–3, see main.swift).
///
/// stdin messages, one type byte each:
///   'K'  — encode the next frame (or the last one again) as a keyframe
///   'F'  — a frame: [width:u32BE][height:u32BE][length:u32BE][RGB888 bytes],
///          rows bottom-up, as the emulator sends them
/// EOF exits 0.
enum StdinFrames {
    /// The command-line switch that selects this mode.
    static let flag = "--stdin-frames"

    /// Read frames until EOF; never returns.
    static func run(fps: Int) -> Never {
        let writer = FrameWriter()
        let encoder = H264Encoder(fps: fps, bitrate: 4_000_000)
        encoder.onEncoded = { encoded in
            if let description = encoded.description {
                writer.write(type: 1, payload: description)
            }
            writer.write(type: encoded.isKeyframe ? 2 : 3, payload: encoded.avcc)
        }
        let converter = RGBToBGRA()
        var last: CVPixelBuffer?
        var wantKeyframe = true
        while true {
            autoreleasepool {
                switch readByte() {
                case UInt8(ascii: "K"):
                    wantKeyframe = true
                    if let last { encoder.encode(last, forceKeyframe: true); wantKeyframe = false }
                case UInt8(ascii: "F"):
                    guard let buffer = readFrame(converter: converter) else { return }
                    encoder.encode(buffer, forceKeyframe: wantKeyframe)
                    wantKeyframe = false
                    last = buffer
                default:
                    log("stdin-frames: unknown message — exiting")
                    exit(2)
                }
            }
        }
    }

    /// One type byte, or exit on EOF (the parent closed the pipe).
    private static func readByte() -> UInt8 {
        let data = FileHandle.standardInput.readData(ofLength: 1)
        guard let byte = data.first else { exit(0) }
        return byte
    }

    /// The rest of an 'F' message, converted; nil (logged) when malformed.
    private static func readFrame(converter: RGBToBGRA) -> CVPixelBuffer? {
        let header = readExact(12)
        let width = Int(header.u32BE(at: 0)), height = Int(header.u32BE(at: 4))
        let length = Int(header.u32BE(at: 8))
        let pixels = readExact(length)
        guard width > 0, height > 0, length == width * height * 3 else {
            log("stdin-frames: frame \(width)x\(height) with \(length) bytes is not RGB888 — dropped")
            return nil
        }
        return converter.convert(pixels, width: width, height: height)
    }

    /// Exactly count bytes, or exit on EOF mid-message.
    private static func readExact(_ count: Int) -> Data {
        var data = Data()
        while data.count < count {
            let chunk = FileHandle.standardInput.readData(ofLength: count - data.count)
            if chunk.isEmpty { exit(0) }
            data.append(chunk)
        }
        return data
    }
}

/// Turns bottom-up RGB888 into an upright BGRA pixel buffer, the input
/// VideoToolbox's H.264 encoder takes. Buffers come from a pool rebuilt when
/// the frame size changes (rotation), so steady streaming allocates nothing.
final class RGBToBGRA {
    private var pool: CVPixelBufferPool?
    private var poolSize = (0, 0)
    private var scratch = [UInt8]()

    /// Convert one frame; nil when a buffer cannot be had.
    func convert(_ rgb: Data, width: Int, height: Int) -> CVPixelBuffer? {
        guard let out = buffer(width: width, height: height) else { return nil }
        CVPixelBufferLockBaseAddress(out, [])
        defer { CVPixelBufferUnlockBaseAddress(out, []) }
        guard let base = CVPixelBufferGetBaseAddress(out) else { return nil }
        let rowBytes = width * 4
        if scratch.count != rowBytes * height { scratch = [UInt8](repeating: 0, count: rowBytes * height) }
        let ok = rgb.withUnsafeBytes { src -> Bool in
            scratch.withUnsafeMutableBytes { tmp -> Bool in
                var s = vImage_Buffer(data: UnsafeMutableRawPointer(mutating: src.baseAddress!),
                                      height: vImagePixelCount(height), width: vImagePixelCount(width), rowBytes: width * 3)
                var t = vImage_Buffer(data: tmp.baseAddress!, height: vImagePixelCount(height),
                                      width: vImagePixelCount(width), rowBytes: rowBytes)
                var d = vImage_Buffer(data: base, height: vImagePixelCount(height), width: vImagePixelCount(width),
                                      rowBytes: CVPixelBufferGetBytesPerRow(out))
                guard vImageConvert_RGB888toBGRA8888(&s, nil, 255, &t, false, vImage_Flags(kvImageNoFlags)) == kvImageNoError
                else { return false }
                // The emulator's rows run bottom-up; flip into the upright buffer.
                return vImageVerticalReflect_ARGB8888(&t, &d, vImage_Flags(kvImageNoFlags)) == kvImageNoError
            }
        }
        return ok ? out : nil
    }

    private func buffer(width: Int, height: Int) -> CVPixelBuffer? {
        if pool == nil || poolSize != (width, height) {
            let attrs: [CFString: Any] = [
                kCVPixelBufferPixelFormatTypeKey: kCVPixelFormatType_32BGRA,
                kCVPixelBufferWidthKey: width, kCVPixelBufferHeightKey: height,
                kCVPixelBufferIOSurfacePropertiesKey: [:] as CFDictionary,
            ]
            pool = nil
            CVPixelBufferPoolCreate(kCFAllocatorDefault, nil, attrs as CFDictionary, &pool)
            poolSize = (width, height)
        }
        guard let pool else { return nil }
        var out: CVPixelBuffer?
        CVPixelBufferPoolCreatePixelBuffer(kCFAllocatorDefault, pool, &out)
        return out
    }
}

private extension Data {
    /// Big-endian UInt32 at a byte offset from the start of the data.
    func u32BE(at offset: Int) -> UInt32 {
        let i = startIndex + offset
        return UInt32(self[i]) << 24 | UInt32(self[i + 1]) << 16 | UInt32(self[i + 2]) << 8 | UInt32(self[i + 3])
    }
}
