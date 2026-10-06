package codegen

import "github.com/relux-works/javacard-rpc/codegen/internal/render"

// Compatibility types and entry points delegate to the unchanged renderers.
type StreamMemory = render.StreamMemory
type JavaOptions = render.JavaOptions
type JavaGenerationResult = render.JavaGenerationResult

const (
	StreamMemoryClearOnDeselect = render.StreamMemoryClearOnDeselect
	StreamMemoryClearOnReset    = render.StreamMemoryClearOnReset
)

func GenerateJavaSkeleton(s *Schema, packageName string) (*JavaGenerationResult, error) {
	return render.GenerateJavaSkeleton(s, packageName)
}

func GenerateJavaSkeletonWithOptions(s *Schema, packageName string, options JavaOptions) (*JavaGenerationResult, error) {
	return render.GenerateJavaSkeletonWithOptions(s, packageName, options)
}

func GenerateSwiftClient(s *Schema, moduleName string) ([]byte, error) {
	return render.GenerateSwiftClient(s, moduleName)
}

func GenerateKotlinClient(s *Schema, packageName string) ([]byte, error) {
	return render.GenerateKotlinClient(s, packageName)
}

func GenerateKotlinBuildGradle(appletLower, packageName, version string) string {
	return render.GenerateKotlinBuildGradle(appletLower, packageName, version)
}

func GenerateKotlinSettingsGradle(appletLower string) string {
	return render.GenerateKotlinSettingsGradle(appletLower)
}

func DefaultKotlinPackage(appletName string) string { return render.DefaultKotlinPackage(appletName) }
func KotlinSourceFileName(appletName string) string { return render.KotlinSourceFileName(appletName) }
