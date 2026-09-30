import Foundation

/// The target frame rate as a ceiling on how often captures start.
///
/// Frame callbacks fire whenever the guest composites, which can outrun the
/// stream's fps; the coalescer bounds the backlog to one, and this bounds
/// the rate: a capture requested sooner than one interval after the last
/// one started waits out the remainder. Waiting (rather than dropping)
/// matters — the delayed capture reads the latest surface, so the final
/// frame of a burst is always encoded. Times are seconds on any monotonic
/// clock. Not thread-safe: use it from one serial queue.
public struct CaptureCadence {
    /// Seconds between capture starts at the target rate.
    public let interval: TimeInterval
    private var lastStart: TimeInterval?

    /// A cadence for `fps` frames per second; values below 1 clamp to 1 so
    /// a bad argument can never divide by zero or spin.
    public init(fps: Int) {
        interval = 1.0 / Double(max(1, fps))
    }

    /// How long a capture requested at `now` must wait so that it starts no
    /// sooner than one interval after the previous start. Zero before the
    /// first capture, and never negative.
    public func delay(at now: TimeInterval) -> TimeInterval {
        guard let lastStart else { return 0 }
        return max(0, lastStart + interval - now)
    }

    /// Records that a capture started at `now`.
    public mutating func began(at now: TimeInterval) {
        lastStart = now
    }
}
