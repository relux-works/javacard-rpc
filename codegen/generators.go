package codegen

import (
	kotlin "github.com/relux-works/javacard-rpc-client-kotlin/codegen"
	swift "github.com/relux-works/javacard-rpc-client-swift/codegen"
	javacard "github.com/relux-works/javacard-rpc-server-javacard/codegen"
	"github.com/relux-works/javacard-rpc/pluginapi"
	"path"
	"strings"
)

// Compatibility types and entry points delegate to the released target backends.
type StreamMemory string
type JavaOptions struct{ StreamMemory StreamMemory }
type JavaGenerationResult struct {
	TransportSource         []byte // CounterTransport.java
	SkeletonSource          []byte // CounterSkeleton.java
	StreamEndpointSource    []byte // CounterStreamEndpoint.java, streamed schemas only
	StreamRuntimeSource     []byte // CounterBoundedStreamRuntime.java, streamed schemas only
	StreamAPDUAdapterSource []byte // CounterStreamAPDUAdapter.java, streamed schemas only
	TransportName           string // e.g. "CounterTransport"
	SkeletonName            string // e.g. "CounterSkeleton"
	StreamEndpointName      string // e.g. "CounterStreamEndpoint"
	StreamRuntimeName       string // e.g. "CounterBoundedStreamRuntime"
	StreamAPDUAdapterName   string // e.g. "CounterStreamAPDUAdapter"
}

const (
	StreamMemoryClearOnDeselect StreamMemory = "clear_on_deselect"
	StreamMemoryClearOnReset    StreamMemory = "clear_on_reset"
)

func GenerateJavaSkeleton(s *Schema, packageName string) (*JavaGenerationResult, error) {
	return GenerateJavaSkeletonWithOptions(s, packageName, JavaOptions{})
}
func GenerateJavaSkeletonWithOptions(s *Schema, packageName string, options JavaOptions) (*JavaGenerationResult, error) {
	files, err := (javacard.Plugin{}).Generate(s, pluginapi.Options{Namespace: packageName, StreamMemory: string(options.StreamMemory), SimulatorDependency: "com.klinec:jcardsim:3.0.5.9"})
	if err != nil {
		return nil, err
	}
	result := &JavaGenerationResult{}
	for _, f := range files {
		name := strings.TrimSuffix(path.Base(f.Name), ".java")
		switch {
		case strings.HasSuffix(name, "Transport"):
			result.TransportName = name
			result.TransportSource = f.Data
		case strings.HasSuffix(name, "Skeleton"):
			result.SkeletonName = name
			result.SkeletonSource = f.Data
		case strings.HasSuffix(name, "StreamEndpoint"):
			result.StreamEndpointName = name
			result.StreamEndpointSource = f.Data
		case strings.HasSuffix(name, "BoundedStreamRuntime"):
			result.StreamRuntimeName = name
			result.StreamRuntimeSource = f.Data
		case strings.HasSuffix(name, "StreamAPDUAdapter"):
			result.StreamAPDUAdapterName = name
			result.StreamAPDUAdapterSource = f.Data
		}
	}
	stem := strings.TrimSuffix(result.SkeletonName, "Skeleton")
	if result.StreamEndpointName == "" {
		result.StreamEndpointName = stem + "StreamEndpoint"
	}
	if result.StreamRuntimeName == "" {
		result.StreamRuntimeName = stem + "BoundedStreamRuntime"
	}
	if result.StreamAPDUAdapterName == "" {
		result.StreamAPDUAdapterName = stem + "StreamAPDUAdapter"
	}
	return result, nil
}
func GenerateSwiftClient(s *Schema, moduleName string) ([]byte, error) {
	files, err := (swift.Plugin{}).Generate(s, pluginapi.Options{Namespace: moduleName})
	if err != nil {
		return nil, err
	}
	return files[len(files)-1].Data, nil
}
func GenerateKotlinClient(s *Schema, packageName string) ([]byte, error) {
	return kotlin.GenerateKotlinClient(s, packageName)
}
func GenerateKotlinBuildGradle(appletLower, packageName, version string) string {
	return kotlin.GenerateKotlinBuildGradle(appletLower, packageName, version)
}
func GenerateKotlinSettingsGradle(appletLower string) string {
	return kotlin.GenerateKotlinSettingsGradle(appletLower)
}
func DefaultKotlinPackage(appletName string) string { return kotlin.DefaultKotlinPackage(appletName) }
func KotlinSourceFileName(appletName string) string { return kotlin.KotlinSourceFileName(appletName) }
