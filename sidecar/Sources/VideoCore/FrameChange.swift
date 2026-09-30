/// Decides whether a captured surface is worth encoding, from its IOSurface
/// seed and surface ID.
///
/// Encode when the content changed (the seed moved) or the surface was
/// swapped — the simulator double-buffers, so a swap can carry new content
/// under a seed that matches the other surface's. Only a keyframe
/// *request* forces an encode on an unchanged surface: a swap alone must
/// not force a keyframe, because the buffer ring rotates on nearly every
/// frame even on a still screen and an IDR each time pins the encoder at
/// max rate. The caller commits a frame only once the encoder accepted it,
/// so a frame dropped under encoder backpressure is retried on the next
/// capture instead of being remembered as sent.
/// Not thread-safe: use it from one serial queue.
public struct FrameChange {
    private var lastSeed: UInt32 = 0
    private var lastSurfaceID: UInt32 = 0

    /// Creates a detector that has seen no frame yet.
    public init() {}

    /// Whether this surface state should be encoded; `force` is a pending
    /// keyframe request, which must produce a frame even on a static screen.
    public func needsEncode(seed: UInt32, surfaceID: UInt32, force: Bool) -> Bool {
        force || seed != lastSeed || surfaceID != lastSurfaceID
    }

    /// Records the surface state the encoder accepted.
    public mutating func commit(seed: UInt32, surfaceID: UInt32) {
        lastSeed = seed
        lastSurfaceID = surfaceID
    }
}
