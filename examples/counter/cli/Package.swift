// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "counter-cli",
    platforms: [
        .macOS(.v13),
    ],
    dependencies: [
        .package(path: "../generated/counter-client-swift"),
        .package(path: "../../../../javacard-rpc-client-swift"),  // runtime
    ],
    targets: [
        .executableTarget(
            name: "counter-cli",
            dependencies: [
                .product(name: "CounterClient", package: "counter-client-swift"),
                .product(name: "JavaCardRPCClient", package: "javacard-rpc-client-swift"),
            ],
            path: "Sources"
        ),
    ]
)
