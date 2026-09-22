// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "devicedeck-sidecars",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "HIDProtocol"),
        .target(name: "SimCore"),
        .executableTarget(name: "devicedeck-hid", dependencies: ["HIDProtocol", "SimCore"]),
        .executableTarget(name: "devicedeck-video", dependencies: ["SimCore"]),
        .testTarget(name: "HIDProtocolTests", dependencies: ["HIDProtocol"]),
        .testTarget(name: "SimCoreTests", dependencies: ["SimCore"]),
    ]
)
