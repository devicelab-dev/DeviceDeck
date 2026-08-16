// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "devicedeck-hid",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "HIDProtocol"),
        .executableTarget(name: "devicedeck-hid", dependencies: ["HIDProtocol"]),
        .testTarget(name: "HIDProtocolTests", dependencies: ["HIDProtocol"]),
    ]
)
