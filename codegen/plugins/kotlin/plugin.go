// Package kotlin adapts the Kotlin renderer to the independent plugin API.
package kotlin

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/render"
	"github.com/relux-works/javacard-rpc/codegen/plugins/internal/packagefiles"
	"github.com/relux-works/javacard-rpc/pluginapi"
	"path"
	"strings"
)

type Plugin struct{}

var _ pluginapi.Plugin = Plugin{}

func (Plugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	source, err := render.GenerateKotlinClient(s, o.Namespace)
	if err != nil {
		return nil, err
	}
	stem := strings.ToLower(packagefiles.Stem(s.Applet.Name))
	return []pluginapi.File{
		{Name: "settings.gradle.kts", Data: []byte(GenerateKotlinSettingsGradle(stem))},
		{Name: "build.gradle.kts", Data: []byte(GenerateKotlinBuildGradle(stem, o.Namespace, s.Applet.Version))},
		{Name: path.Join("src/main/kotlin", strings.ReplaceAll(o.Namespace, ".", "/"), render.KotlinSourceFileName(s.Applet.Name)), Data: source},
	}, nil
}
