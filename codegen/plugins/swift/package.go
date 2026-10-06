package swift

import "fmt"

func GeneratePackageSwift(appletLower, clientName string) string {
	return fmt.Sprintf(`// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "%[1]s-client-swift",
    platforms: [
        .iOS(.v15),
        .macOS(.v12),
    ],
    products: [
        .library(name: "%[2]s", targets: ["%[2]s"]),
    ],
    targets: [
        .target(
            name: "%[2]s",
            path: "Sources/%[2]s"
        ),
    ]
)
`, appletLower, clientName)
}
