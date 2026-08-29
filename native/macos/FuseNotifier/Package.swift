// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "FuseNotifier",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "FuseNotifierProtocol", targets: ["FuseNotifierProtocol"]),
        .library(name: "FuseNotifierCore", targets: ["FuseNotifierCore"]),
        .executable(name: "FuseNotifier", targets: ["FuseNotifier"]),
    ],
    targets: [
        .target(name: "FuseNotifierProtocol"),
        .target(name: "FuseNotifierCore", dependencies: ["FuseNotifierProtocol"]),
        .executableTarget(
            name: "FuseNotifier",
            dependencies: ["FuseNotifierProtocol", "FuseNotifierCore"]
        ),
        .executableTarget(
            name: "FuseNotifierProtocolBehaviorTests",
            dependencies: ["FuseNotifierProtocol"],
            path: "Tests/FuseNotifierProtocolTests"
        ),
        .executableTarget(
            name: "FuseNotifierCoreBehaviorTests",
            dependencies: ["FuseNotifierCore", "FuseNotifierProtocol"],
            path: "Tests/FuseNotifierCoreTests"
        ),
    ]
)
