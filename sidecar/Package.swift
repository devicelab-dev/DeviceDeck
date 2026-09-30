// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "devicedeck-sidecars",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "HIDProtocol"),
        .target(name: "SimCore"),
        .target(name: "DTUHID", dependencies: ["HIDProtocol"]),
        .executableTarget(name: "devicedeck-hid", dependencies: ["HIDProtocol", "SimCore", "DTUHID"]),
        .target(name: "VideoCore"),
        .executableTarget(name: "devicedeck-video", dependencies: ["SimCore", "VideoCore"]),
        .testTarget(name: "HIDProtocolTests", dependencies: ["HIDProtocol"]),
        .testTarget(name: "SimCoreTests", dependencies: ["SimCore"]),
        .testTarget(name: "DTUHIDTests", dependencies: ["DTUHID", "HIDProtocol"]),
        .testTarget(name: "VideoCoreTests", dependencies: ["VideoCore"]),
    ]
)
