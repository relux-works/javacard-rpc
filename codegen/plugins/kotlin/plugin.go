// Package kotlin adapts the Kotlin renderer to the independent plugin API.
package kotlin

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/render"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

type Plugin struct{}

var _ pluginapi.Plugin = Plugin{}

func (Plugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	source, err := render.GenerateKotlinClient(s, o.Namespace)
	if err != nil {
		return nil, err
	}
	return []pluginapi.File{{Name: render.KotlinSourceFileName(s.Applet.Name), Data: source}}, nil
}
