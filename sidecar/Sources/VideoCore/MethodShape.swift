/// Compares Objective-C method type encodings by shape — return and
/// argument types only.
///
/// A runtime type encoding such as `v56@0:8@16@24@?32@?40@?48` interleaves
/// types with stack offsets. The offsets are an ABI detail; the types are
/// the contract a hand-written `@convention(c)` or `@objc` call site relies
/// on. Checking the shape before calling a private selector turns an Apple
/// signature change into a logged fallback instead of a crash.
public enum MethodShape {
    /// The encoding with its offset digits removed:
    /// `v56@0:8@16` becomes `v@:@`.
    public static func of(_ encoding: String) -> String {
        encoding.filter { !$0.isNumber }
    }

    /// Whether `encoding` has exactly the `expected` shape.
    public static func matches(_ encoding: String, expected: String) -> Bool {
        of(encoding) == expected
    }
}
