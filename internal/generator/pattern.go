package generator

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
)

// pattern is a parsed resource name pattern — a sequence of literal or
// variable segments as defined by AIP-122 (e.g. "projects/{project}/books/{book}").
type pattern []segment

// newPattern parses a resource pattern string into its segments. A variable
// segment is written as "{name}"; every other segment is a literal that must
// match exactly at parse time.
func newPattern(s string) (pattern, error) {
	var out pattern
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, "{") {
			if !strings.HasSuffix(part, "}") {
				return nil, fmt.Errorf("invalid segment: %q", part)
			}
			out = append(out, segment{
				Name: strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}"),
				Var:  true,
			})
			continue
		}
		out = append(out, segment{Name: part})
	}
	return out, nil
}

type segment struct {
	Name string
	Var  bool
}

// FieldName returns the Go struct field name for a variable segment. For
// example the segment {project} becomes ProjectID.
func (s segment) FieldName() string {
	return strcase.ToCamel(s.Name + "_ID")
}
