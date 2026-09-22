import Foundation
import XCTest
@testable import SimCore

/// SimulatorKit moved between Xcode releases; these pin both layouts so a
/// future move fails a test instead of every sidecar at launch.
final class DeveloperDirTests: XCTestCase {
    private let dev = "/Applications/Xcode.app/Contents/Developer"

    func testCandidatesCoverBothXcodeLayouts() {
        XCTAssertEqual(DeveloperDir.simulatorKitCandidates(dev), [
            "/Applications/Xcode.app/Contents/Developer/Library/PrivateFrameworks/SimulatorKit.framework/SimulatorKit",
            "/Applications/Xcode.app/Contents/SharedFrameworks/SimulatorKit.framework/SimulatorKit",
        ])
    }

    func testPathPrefersTheCandidateThatExists() throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString).appendingPathComponent("Contents")
        defer { try? FileManager.default.removeItem(at: root.deletingLastPathComponent()) }
        let devDir = root.appendingPathComponent("Developer").path
        let shared = root.appendingPathComponent("SharedFrameworks/SimulatorKit.framework")
        try FileManager.default.createDirectory(at: shared, withIntermediateDirectories: true)
        FileManager.default.createFile(atPath: shared.appendingPathComponent("SimulatorKit").path, contents: Data())

        XCTAssertEqual(DeveloperDir.simulatorKitPath(devDir),
                       shared.appendingPathComponent("SimulatorKit").standardizedFileURL.path)
    }

    func testPathFallsBackToLegacyWhenNeitherExists() {
        let missing = "/nonexistent/Contents/Developer"
        XCTAssertEqual(DeveloperDir.simulatorKitPath(missing),
                       DeveloperDir.simulatorKitCandidates(missing)[0])
    }
}
