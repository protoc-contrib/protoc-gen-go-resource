// Command protoc-gen-go-resource is a protoc plugin that emits Go helpers for
// parsing and reconstructing Google API resource names declared via the
// google.api.resource and google.api.resource_reference annotations.
package main

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator"
)

func main() {
	opts := &generator.Options{}
	protogen.Options{
		ParamFunc: opts.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(
			pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
				pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS,
		)
		plugin.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_2023
		plugin.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2023
		return generator.Generate(plugin, opts)
	})
}
