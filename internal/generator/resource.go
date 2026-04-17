package generator

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// resource captures everything the generator needs to know about a single
// google.api.resource definition — the Go identifiers it will emit, the
// parsed name-field (if the definition came from a message), and the set of
// patterns that identify instances of the resource.
type resource struct {
	NameField     *protogen.Field
	ParseFunc     protogen.GoIdent
	FullParseFunc protogen.GoIdent
	ParsedType    protogen.GoIdent
	Type          resourceType
	Patterns      []pattern
}

// newMessageResource returns the resource defined on a message via the
// google.api.resource option, or nil if the message has no such annotation.
func newMessageResource(m *protogen.Message) (*resource, error) {
	opts, _ := m.Desc.Options().(*descriptorpb.MessageOptions)
	d, _ := proto.GetExtension(opts, annotations.E_Resource).(*annotations.ResourceDescriptor)
	if d == nil {
		return nil, nil
	}

	r, err := newBareResource(m.GoIdent.GoImportPath, d)
	if err != nil {
		return nil, err
	}

	fieldName := "name"
	if d.NameField != "" {
		fieldName = d.NameField
	}
	for _, f := range m.Fields {
		if string(f.Desc.Name()) == fieldName {
			r.NameField = f
			break
		}
	}
	if r.NameField == nil {
		return nil, fmt.Errorf("%v specifies %q as name field, but no field with that name exists", m.GoIdent, fieldName)
	}
	return r, nil
}

// newFileResources returns every resource declared at file scope via
// google.api.resource_definition.
func newFileResources(f *protogen.File) ([]*resource, error) {
	opts, _ := f.Desc.Options().(*descriptorpb.FileOptions)
	defs, _ := proto.GetExtension(opts, annotations.E_ResourceDefinition).([]*annotations.ResourceDescriptor)

	out := make([]*resource, 0, len(defs))
	for _, d := range defs {
		r, err := newBareResource(f.GoImportPath, d)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func newBareResource(importPath protogen.GoImportPath, d *annotations.ResourceDescriptor) (*resource, error) {
	t, err := newResourceType(d.Type)
	if err != nil {
		return nil, err
	}

	patterns := make([]pattern, 0, len(d.Pattern))
	for _, s := range d.Pattern {
		p, err := newPattern(s)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}

	return &resource{
		ParseFunc:     protogen.GoIdent{GoName: "Parse" + t.TypeName + "Name", GoImportPath: importPath},
		FullParseFunc: protogen.GoIdent{GoName: "ParseFull" + t.TypeName + "Name", GoImportPath: importPath},
		ParsedType:    protogen.GoIdent{GoName: "Parsed" + t.TypeName + "Name", GoImportPath: importPath},
		Type:          t,
		Patterns:      patterns,
	}, nil
}

// resourceType is the parsed form of a resource type string like
// "example.com/Thing" — {ServiceName: "example.com", TypeName: "Thing"}.
type resourceType struct {
	ServiceName string
	TypeName    string
}

func newResourceType(s string) (resourceType, error) {
	i := strings.IndexByte(s, '/')
	if i == -1 {
		return resourceType{}, fmt.Errorf("invalid resource type: %q", s)
	}
	return resourceType{ServiceName: s[:i], TypeName: s[i+1:]}, nil
}

// servicePrefix returns the "full name" prefix used for this resource —
// "//" + service name + "/".
func (r *resource) servicePrefix() string {
	return "//" + r.Type.ServiceName + "/"
}
