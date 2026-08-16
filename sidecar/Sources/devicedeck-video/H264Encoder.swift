import Foundation
import CoreVideo
import CoreMedia
import VideoToolbox
import IOSurface

/// Real-time H.264 encoder backed by `VTCompressionSession`, tuned for
/// low-latency streaming (no frame reordering, low-latency rate control).
/// Submission is fire-and-forget: output fires on VT's own queue via
/// `onEncoded`, so the capture loop never blocks on encoder slowness.
/// Derived from baguette's H264Encoder — see ATTRIBUTION.md.
final class H264Encoder: @unchecked Sendable {
    struct Encoded {
        /// avcC parameter-set blob — emitted once per session on the first IDR.
        let description: Data?
        let isKeyframe: Bool
        /// Length-prefixed AVCC NAL bytes.
        let avcc: Data
    }

    var onEncoded: (@Sendable (Encoded) -> Void)?

    private var session: VTCompressionSession?
    private var width: Int32 = 0
    private var height: Int32 = 0
    private let fps: Int32
    private let bitrate: Int
    private var emittedDescription = false
    private var frameCount: Int64 = 0

    init(fps: Int, bitrate: Int) {
        self.fps = Int32(fps)
        self.bitrate = bitrate
    }

    deinit {
        if let session { VTCompressionSessionInvalidate(session) }
    }

    /// Zero-copy wrap the IOSurface into a CVPixelBuffer and submit it.
    func encode(_ surface: IOSurface, forceKeyframe: Bool) {
        var pb: Unmanaged<CVPixelBuffer>?
        let status = CVPixelBufferCreateWithIOSurface(
            kCFAllocatorDefault, surface,
            [kCVPixelBufferPixelFormatTypeKey: kCVPixelFormatType_32BGRA] as CFDictionary,
            &pb
        )
        guard status == kCVReturnSuccess, let pixelBuffer = pb?.takeRetainedValue() else { return }
        encode(pixelBuffer, forceKeyframe: forceKeyframe)
    }

    private func encode(_ pixelBuffer: CVPixelBuffer, forceKeyframe: Bool) {
        let w = Int32(CVPixelBufferGetWidth(pixelBuffer))
        let h = Int32(CVPixelBufferGetHeight(pixelBuffer))
        if session == nil || w != width || h != height {
            width = w
            height = h
            rebuildSession()
        }
        guard let session else { return }

        let frameProps: NSDictionary? = forceKeyframe
            ? [kVTEncodeFrameOptionKey_ForceKeyFrame: kCFBooleanTrue!] as NSDictionary
            : nil
        frameCount += 1
        VTCompressionSessionEncodeFrame(
            session,
            imageBuffer: pixelBuffer,
            presentationTimeStamp: CMTime(value: frameCount, timescale: fps),
            duration: .invalid,
            frameProperties: frameProps,
            infoFlagsOut: nil
        ) { [weak self] status, _, sampleBuffer in
            guard let self, status == noErr, let sb = sampleBuffer,
                  let encoded = self.extract(from: sb) else { return }
            self.onEncoded?(encoded)
        }
    }

    private func rebuildSession() {
        if let session {
            VTCompressionSessionInvalidate(session)
            self.session = nil
        }
        var sess: VTCompressionSession?
        let status = VTCompressionSessionCreate(
            allocator: kCFAllocatorDefault,
            width: width, height: height,
            codecType: kCMVideoCodecType_H264,
            encoderSpecification: [
                kVTVideoEncoderSpecification_EnableLowLatencyRateControl: kCFBooleanTrue!,
            ] as CFDictionary,
            imageBufferAttributes: nil,
            compressedDataAllocator: kCFAllocatorDefault,
            outputCallback: nil,
            refcon: nil,
            compressionSessionOut: &sess
        )
        guard status == noErr, let sess else { return }

        // Rejected properties return non-noErr; intentionally ignored —
        // VT accepts what the encoder supports and streams regardless.
        let props: [(CFString, Any)] = [
            (kVTCompressionPropertyKey_RealTime, kCFBooleanTrue!),
            (kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_High_AutoLevel),
            (kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse!),
            (kVTCompressionPropertyKey_AverageBitRate, NSNumber(value: bitrate)),
            (kVTCompressionPropertyKey_ExpectedFrameRate, NSNumber(value: fps)),
            // One keyframe per 2s: recovery points for late joiners without
            // paying IDR cost every frame.
            (kVTCompressionPropertyKey_MaxKeyFrameInterval, NSNumber(value: Int(fps) * 2)),
        ]
        for (key, value) in props {
            VTSessionSetProperty(sess, key: key, value: value as CFTypeRef)
        }
        VTCompressionSessionPrepareToEncodeFrames(sess)
        session = sess
        emittedDescription = false
    }

    private func extract(from sample: CMSampleBuffer) -> Encoded? {
        let isKeyframe = !notSync(sample)
        guard let dataBuf = CMSampleBufferGetDataBuffer(sample) else { return nil }
        var totalLength = 0
        var dataPointer: UnsafeMutablePointer<Int8>?
        guard CMBlockBufferGetDataPointer(
            dataBuf, atOffset: 0, lengthAtOffsetOut: nil,
            totalLengthOut: &totalLength, dataPointerOut: &dataPointer
        ) == noErr, let dataPointer else { return nil }

        var description: Data?
        if isKeyframe, !emittedDescription,
           let format = CMSampleBufferGetFormatDescription(sample) {
            description = avcCBlob(from: format)
            emittedDescription = description != nil
        }
        return Encoded(
            description: description,
            isKeyframe: isKeyframe,
            avcc: Data(bytes: dataPointer, count: totalLength)
        )
    }

    private func notSync(_ sample: CMSampleBuffer) -> Bool {
        guard let attachments = CMSampleBufferGetSampleAttachmentsArray(sample, createIfNecessary: false),
              CFArrayGetCount(attachments) > 0,
              let dict = CFArrayGetValueAtIndex(attachments, 0) else { return false }
        let cfDict = unsafeBitCast(dict, to: CFDictionary.self)
        return CFDictionaryContainsKey(
            cfDict, Unmanaged.passUnretained(kCMSampleAttachmentKey_NotSync).toOpaque())
    }

    /// avcC parameter-set blob (ISO/IEC 14496-15 §5.2.4.1) — what
    /// WebCodecs needs as `description` to decode AVCC-framed H.264.
    private func avcCBlob(from format: CMFormatDescription) -> Data? {
        var spsPtr: UnsafePointer<UInt8>?
        var spsSize = 0
        var spsCount = 0
        var nalSize: Int32 = 0
        guard CMVideoFormatDescriptionGetH264ParameterSetAtIndex(
            format, parameterSetIndex: 0,
            parameterSetPointerOut: &spsPtr, parameterSetSizeOut: &spsSize,
            parameterSetCountOut: &spsCount, nalUnitHeaderLengthOut: &nalSize
        ) == noErr, let spsPtr else { return nil }
        var ppsPtr: UnsafePointer<UInt8>?
        var ppsSize = 0
        guard CMVideoFormatDescriptionGetH264ParameterSetAtIndex(
            format, parameterSetIndex: 1,
            parameterSetPointerOut: &ppsPtr, parameterSetSizeOut: &ppsSize,
            parameterSetCountOut: nil, nalUnitHeaderLengthOut: nil
        ) == noErr, let ppsPtr else { return nil }

        let sps = UnsafeBufferPointer(start: spsPtr, count: spsSize)
        let pps = UnsafeBufferPointer(start: ppsPtr, count: ppsSize)
        var blob = Data([0x01, sps[1], sps[2], sps[3], 0xFF, 0xE1])
        blob.append(UInt8((spsSize >> 8) & 0xFF))
        blob.append(UInt8(spsSize & 0xFF))
        blob.append(contentsOf: sps)
        blob.append(0x01)
        blob.append(UInt8((ppsSize >> 8) & 0xFF))
        blob.append(UInt8(ppsSize & 0xFF))
        blob.append(contentsOf: pps)
        return blob
    }
}
