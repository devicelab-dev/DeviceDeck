// Pure capture-scheduling logic for devicedeck-video, kept free of
// CoreSimulator so it can be unit-tested. The coalescer recipe is from
// baguette's PendingCapture (commit 41648ad) — see ATTRIBUTION.md.

/// Whether a framebuffer capture is already queued and waiting to run.
///
/// The simulator composites frames faster than a capture can complete:
/// reading `framebufferSurface` is a synchronous XPC round-trip to
/// CoreSimulatorService, not a local property read. Scheduling one capture
/// per frame notification builds a queue of duplicates that each pay that
/// round-trip, and the queue grows faster than it drains once the
/// round-trip is slower than the frame interval — baguette measured a
/// 9GB runaway from exactly this.
///
/// While a capture is queued, further notifications fold into it, because
/// every capture reads the *latest* surface — a duplicate would fetch the
/// same frame. Once the queued capture starts running, the next
/// notification schedules a fresh one, so a newer frame is never lost.
/// Not thread-safe: use it from one serial queue.
public struct PendingCapture {
    private var scheduled = false

    /// Creates an idle coalescer — nothing pending.
    public init() {}

    /// `true` when the caller must schedule a capture because none is
    /// pending. Repeat requests return `false` and coalesce onto it.
    public mutating func request() -> Bool {
        if scheduled { return false }
        scheduled = true
        return true
    }

    /// The queued capture has started. Notifications from here on describe
    /// frames it may not have read, so they must schedule again.
    public mutating func begin() {
        scheduled = false
    }
}
