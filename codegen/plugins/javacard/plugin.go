// Package javacard adapts the Java Card renderer to the independent plugin API.
package javacard

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/render"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

type Plugin struct{}

var _ pluginapi.Plugin = Plugin{}

func (Plugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	r, err := render.GenerateJavaSkeletonWithOptions(s, o.Namespace,
		render.JavaOptions{StreamMemory: render.StreamMemory(o.StreamMemory)})
	if err != nil {
		return nil, err
	}
	files := []pluginapi.File{
		{Name: r.TransportName + ".java", Data: r.TransportSource},
		{Name: r.SkeletonName + ".java", Data: r.SkeletonSource},
	}
	for _, f := range []pluginapi.File{
		{Name: r.StreamEndpointName + ".java", Data: r.StreamEndpointSource},
		{Name: r.StreamRuntimeName + ".java", Data: r.StreamRuntimeSource},
		{Name: r.StreamAPDUAdapterName + ".java", Data: r.StreamAPDUAdapterSource},
	} {
		if len(f.Data) > 0 {
			files = append(files, f)
		}
	}
	return files, nil
}
