import Foundation
import IOSurface
import SimCore
import VideoCore

/// A pending keyframe request. Set from the stdin reader thread, consumed
/// (and cleared) by the capture queue, hence the lock.
final class KeyframeRequest: @unchecked Sendable {
    private let lock = NSLock()
    private var pending = true // first frame is always a keyframe request

    /// Asks for the next encoded frame to be a keyframe.
    func request() {
        lock.lock()
        pending = true
        lock.unlock()
    }

    /// Whether a keyframe was requested since the last call; clears it.
    func consume() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        let was = pending
        pending = false
        return was
    }
}

/// Drives simulator capture: decides *when* to read the framebuffer and
/// hands changed surfaces to the encoder.
///
/// Two modes, chosen at start and logged:
///
/// - **callbacks** — SimulatorKit notifies on every composited frame
///   (`ScreenCallbacks`), so a still screen costs no XPC round-trips. A
///   slow idle floor still captures now and then, in case a notification is
///   missed (baguette saw quiet screens stop delivering them), so a stale
///   frame heals within `idleFloor`.
/// - **poll** — the pre-callback behaviour: a capture every frame interval.
///   Used when this Xcode does not declare the callback API or the display
///   descriptor does not answer it.
///
/// Either way every trigger — callback, timer tick, keyframe request —
/// goes through `scheduleCapture`, where `PendingCapture` keeps at most one
/// capture queued (latest frame wins) and `CaptureCadence` holds captures
/// to the target fps as a ceiling. All state lives on one serial queue.
final class CaptureDriver: @unchecked Sendable {
    /// How often callback mode captures with no notification. Long enough
    /// that an idle screen costs about one round-trip a second instead of
    /// one per frame; short enough that a missed notification is brief.
    static let idleFloor: TimeInterval = 1.0

    private let queue = DispatchQueue(label: "devicedeck.video.capture", qos: .userInteractive)
    private let framebuffer: Framebuffer
    private let encoder: H264Encoder
    private let keyframes: KeyframeRequest
    private var pending = PendingCapture()
    private var cadence: CaptureCadence
    private var change = FrameChange()
    private var callbacks: ScreenCallbacks?
    private var timer: DispatchSourceTimer?

    /// A driver capturing `framebuffer` into `encoder` at most `fps` times a second.
    init(framebuffer: Framebuffer, encoder: H264Encoder, keyframes: KeyframeRequest, fps: Int) {
        self.framebuffer = framebuffer
        self.encoder = encoder
        self.keyframes = keyframes
        self.cadence = CaptureCadence(fps: fps)
    }

    /// Picks the capture mode and takes the first capture. Surfaces often
    /// populate only once callbacks are registered, so the timer keeps
    /// pulling until one appears.
    func start() {
        queue.async { [self] in
            if ScreenCallbacks.isDeclared() {
                enableCallbacks()
            } else {
                startPolling(reason: "SimScreen callback API not declared by this CoreSimulator")
            }
            scheduleCapture()
        }
    }

    /// Requests a keyframe and captures now, so a joining viewer gets a
    /// frame even when the screen is still and no notification will come.
    func requestKeyframe() {
        keyframes.request()
        queue.async { [self] in scheduleCapture() }
    }

    private func enableCallbacks() {
        callbacks = ScreenCallbacks(queue: queue) { [weak self] in self?.scheduleCapture() }
        framebuffer.onDisplayChange = { [weak self] display in self?.bind(display) }
        startTimer(interval: Self.idleFloor)
        log("capture mode=callbacks idle-floor=\(Self.idleFloor)s")
    }

    /// Follows a newly resolved display descriptor; one that does not
    /// answer the callback API drops the driver back to polling.
    private func bind(_ display: NSObject) {
        guard let callbacks, !callbacks.bind(to: display) else { return }
        self.callbacks = nil
        framebuffer.onDisplayChange = nil
        startPolling(reason: "display descriptor does not answer registerScreenCallbacks")
    }

    private func startPolling(reason: String) {
        startTimer(interval: cadence.interval)
        log("capture mode=poll interval=\(cadence.interval)s (\(reason))")
    }

    private func startTimer(interval: TimeInterval) {
        timer?.cancel()
        let source = DispatchSource.makeTimerSource(queue: queue)
        source.schedule(deadline: .now() + interval, repeating: interval)
        source.setEventHandler { [weak self] in self?.scheduleCapture() }
        source.resume()
        timer = source
    }

    /// Queues one capture unless one is already waiting, delayed as needed
    /// to respect the fps ceiling. Runs on `queue` only.
    private func scheduleCapture() {
        guard pending.request() else { return }
        let delay = cadence.delay(at: Self.now())
        queue.asyncAfter(deadline: .now() + delay) { [weak self] in self?.runCapture() }
    }

    /// Runs the queued capture inside its own autorelease pool. The queue
    /// never drains a pool of its own between work items at this rate, and
    /// every capture autoreleases the XPC reply, the IOSurface and
    /// CVPixelBuffer wrappers and the frame-properties dictionary; left to
    /// accumulate, that grew the process to tens of GB (~70MB/s measured
    /// under sustained encoding). The body is a separate method because a
    /// `return` inside the `autoreleasepool` closure would only leave the
    /// closure.
    private func runCapture() {
        pending.begin()
        cadence.began(at: Self.now())
        autoreleasepool { performCapture() }
    }

    private func performCapture() {
        guard let surface = framebuffer.currentSurface() else { return }
        let seed = IOSurfaceGetSeed(surface)
        let surfaceID = IOSurfaceGetID(surface)
        let force = keyframes.consume()
        guard change.needsEncode(seed: seed, surfaceID: surfaceID, force: force) else { return }
        if encoder.encode(surface, forceKeyframe: force) {
            change.commit(seed: seed, surfaceID: surfaceID)
        } else if force {
            // Dropped under encoder backpressure: keep the request so the
            // next capture (a notification or the timer) delivers it.
            keyframes.request()
        }
    }

    private static func now() -> TimeInterval {
        ProcessInfo.processInfo.systemUptime
    }
}
