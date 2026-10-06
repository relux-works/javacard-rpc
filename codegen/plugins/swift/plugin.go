// Package swift adapts the Swift renderer to the independent plugin API.
package swift

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/render"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

type Plugin struct{}

var _ pluginapi.Plugin = Plugin{}

func (Plugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	source, err := render.GenerateSwiftClient(s, o.Namespace)
	if err != nil {
		return nil, err
	}
	return []pluginapi.File{{Name: s.Applet.Name + "Client.swift", Data: source}}, nil
}
