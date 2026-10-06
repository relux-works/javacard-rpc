// Package swift adapts the Swift renderer to the independent plugin API.
package swift

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/render"
	"github.com/relux-works/javacard-rpc/codegen/plugins/internal/packagefiles"
	"github.com/relux-works/javacard-rpc/pluginapi"
	"strings"
)

type Plugin struct{}

var _ pluginapi.Plugin = Plugin{}

func (Plugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	source, err := render.GenerateSwiftClient(s, o.Namespace)
	if err != nil {
		return nil, err
	}
	client := packagefiles.Stem(s.Applet.Name) + "Client"
	return []pluginapi.File{
		{Name: "Package.swift", Data: []byte(GeneratePackageSwift(strings.ToLower(packagefiles.Stem(s.Applet.Name)), client))},
		{Name: "Sources/" + client + "/" + client + ".swift", Data: source},
	}, nil
}
