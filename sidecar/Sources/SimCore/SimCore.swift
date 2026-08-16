import Foundation
import ObjectiveC

// Shared plumbing for DeviceDeck's sidecars: CoreSimulator loading, device
// resolution, Objective-C invocation helpers, and logging. Derived from
// tapflow's helper patterns — see ATTRIBUTION.md.

/// stderr log line tagged with the process name; stdout belongs to each
/// sidecar's data protocol.
public func log(_ message: String) {
    let tag = ProcessInfo.processInfo.processName
    FileHandle.standardError.write(Data((tag + ": " + message + "\n").utf8))
}

public func dlerrorString() -> String {
    dlerror().map { String(cString: $0) } ?? "unknown dlerror"
}

public func fatalStartup(_ message: String) -> Never {
    log("fatal: " + message)
    exit(1)
}

/// Locates the active Xcode developer directory, preferring one that
/// actually ships SimulatorKit.
public enum DeveloperDir {
    public static func find() -> String {
        let selected = runXcodeSelect()
        if !selected.isEmpty, shipsSimulatorKit(selected) { return selected }
        let apps = (try? FileManager.default.contentsOfDirectory(atPath: "/Applications")) ?? []
        for app in apps.sorted() where app.hasPrefix("Xcode") && app.hasSuffix(".app") {
            let dir = "/Applications/\(app)/Contents/Developer"
            if shipsSimulatorKit(dir) { return dir }
        }
        return selected.isEmpty ? "/Applications/Xcode.app/Contents/Developer" : selected
    }

    public static func simulatorKitPath(_ developerDir: String) -> String {
        (developerDir as NSString)
            .appendingPathComponent("Library/PrivateFrameworks/SimulatorKit.framework/SimulatorKit")
    }

    private static func runXcodeSelect() -> String {
        let pipe = Pipe()
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/xcode-select")
        task.arguments = ["-p"]
        task.standardOutput = pipe
        do { try task.run() } catch { return "" }
        task.waitUntilExit()
        return String(data: pipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    private static func shipsSimulatorKit(_ dir: String) -> Bool {
        FileManager.default.fileExists(atPath: simulatorKitPath(dir))
    }
}

/// CoreSimulator access: framework loading and device resolution.
public enum CoreSim {
    /// dlopen CoreSimulator or exit — no sidecar can work without it.
    public static func load() {
        guard dlopen("/Library/Developer/PrivateFrameworks/CoreSimulator.framework/CoreSimulator",
                     RTLD_NOW | RTLD_GLOBAL) != nil else {
            fatalStartup("CoreSimulator dlopen failed: \(dlerrorString())")
        }
    }

    /// Resolve a SimDevice by UDID, or the first booted device for "booted".
    public static func resolveDevice(udid: String, developerDir: String) -> NSObject? {
        guard let cls = NSClassFromString("SimServiceContext"),
              let ctx = ObjC.invokeClass(cls, "sharedServiceContextForDeveloperDir:error:",
                                         with: developerDir as NSString),
              let set = ObjC.invokeInstance(ctx, "defaultDeviceSetWithError:") else { return nil }
        let devices = (set.value(forKey: "availableDevices") as? [NSObject]) ?? []
        if udid == "booted" {
            // CoreSimulator state 3 = booted.
            return devices.first { ($0.value(forKey: "state") as? NSNumber)?.uintValue == 3 }
        }
        return devices.first { ($0.value(forKey: "UDID") as? NSUUID)?.uuidString == udid }
    }
}

/// Objective-C invocation helpers. All lookups go through
/// `class_getInstanceMethod`, never `class_getMethodImplementation` alone:
/// Xcode 26 returns a non-NULL forwarding trampoline for removed selectors
/// that crashes with "unrecognized selector" when called.
public enum ObjC {
    public static func alloc(_ cls: AnyClass) -> NSObject? {
        let sel = NSSelectorFromString("alloc")
        guard let meta = object_getClass(cls),
              let method = class_getInstanceMethod(meta, sel) else { return nil }
        typealias AllocFn = @convention(c) (AnyClass, Selector) -> NSObject?
        return unsafeBitCast(method_getImplementation(method), to: AllocFn.self)(cls, sel)
    }

    /// Call a class method taking one object arg and an NSError out-param.
    public static func invokeClass(_ cls: AnyClass, _ selector: String,
                                   with arg: AnyObject) -> NSObject? {
        let sel = NSSelectorFromString(selector)
        guard let meta = object_getClass(cls),
              let method = class_getInstanceMethod(meta, sel) else { return nil }
        typealias Fn = @convention(c) (
            AnyClass, Selector, AnyObject, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> NSObject?
        var err: NSError?
        return unsafeBitCast(method_getImplementation(method), to: Fn.self)(cls, sel, arg, &err)
    }

    /// Call an instance method taking only an NSError out-param.
    public static func invokeInstance(_ obj: NSObject, _ selector: String) -> NSObject? {
        let sel = NSSelectorFromString(selector)
        guard let method = class_getInstanceMethod(type(of: obj), sel) else { return nil }
        typealias Fn = @convention(c) (
            NSObject, Selector, AutoreleasingUnsafeMutablePointer<NSError?>
        ) -> NSObject?
        var err: NSError?
        return unsafeBitCast(method_getImplementation(method), to: Fn.self)(obj, sel, &err)
    }
}
