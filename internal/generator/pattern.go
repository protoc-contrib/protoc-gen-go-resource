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
	if s == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	var out pattern
	seen := map[string]bool{}
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, "{") {
			if !strings.HasSuffix(part, "}") {
				return nil, fmt.Errorf("invalid segment: %q", part)
			}
			name := part[1 : len(part)-1]
			if name == "" {
				return nil, fmt.Errorf("empty variable name in pattern %q", s)
			}
			if seen[name] {
				return nil, fmt.Errorf("duplicate variable %q in pattern %q", name, s)
			}
			seen[name] = true
			out = append(out, segment{Name: name, Var: true})
			continue
		}
		out = append(out, segment{Name: part})
	}
	return out, nil
}

type segment struct {
	Name   string
	Var    bool
	Format segmentFormat
}

// segmentFormat is the declared value type of a variable segment. It defaults
// to formatString; other values are populated from google.api.field_info on the
// owning resource message.
type segmentFormat int

const (
	formatString segmentFormat = iota
	formatUUID4
)

// String renders the format for use in codegen error messages.
func (f segmentFormat) String() string {
	switch f {
	case formatUUID4:
		return "uuid4"
	default:
		return "string"
	}
}

// FieldName returns the Go struct field name for a variable segment. For
// example the segment {project} becomes ProjectID. The "ID" suffix is appended
// literally rather than routed through strcase so the acronym survives
// regardless of the strcase version in use (v0.3 normalizes "_ID" → "Id").
func (s segment) FieldName() string {
	return strcase.ToCamel(s.Name) + "ID"
}
