package generator

import (
	"fmt"
	"strconv"
)

// Options controls how resource name helpers are emitted.
type Options struct {
	// AllowUnresolvedRefs, when true, causes google.api.resource_reference
	// fields whose target type isn't registered (typically because the
	// referent .proto wasn't part of the compilation unit) to be skipped
	// silently instead of aborting generation.
	AllowUnresolvedRefs bool
}

// Set applies a single `name=value` plugin parameter to the options. The
// signature matches what protogen.Options.ParamFunc expects.
func (o *Options) Set(name, value string) error {
	switch name {
	case "allow_unresolved_refs":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %q: %w", name, err)
		}
		o.AllowUnresolvedRefs = v
		return nil
	default:
		return fmt.Errorf("unknown plugin option %q", name)
	}
}
