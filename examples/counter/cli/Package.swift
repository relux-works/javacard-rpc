// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "counter-cli",
    platforms: [
        .macOS(.v13),
    ],
    dependencies: [
        .package(path: "../../../../counter-client-swift"),
        .package(path: "../../../../appletrpc-client-swift"),
    ],
    targets: [
        .executableTarget(
            name: "counter-cli",
            dependencies: [
                .product(name: "CounterClient", package: "counter-client-swift"),
                .product(name: "AppletRPCClient", package: "appletrpc-client-swift"),
            ],
            path: "Sources"
        ),
    ]
)
