// Package backend demonstrates a separate module that needs only pluginapi.
package backend

import (
	"fmt"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

type Backend struct{}

var _ pluginapi.Plugin = Backend{}

func (Backend) Generate(schema *pluginapi.Schema, options pluginapi.Options) ([]pluginapi.File, error) {
	if schema == nil {
		return nil, fmt.Errorf("validated schema required")
	}
	return []pluginapi.File{
		{Name: "build.manifest", Data: []byte(schema.Applet.Version)},
		{Name: "src/backend.txt", Data: []byte(options.Namespace + "\n" + options.StreamMemory + "\n" + options.SimulatorDependency + "\n" + schema.Applet.StreamWorkspace)},
	}, nil
}
