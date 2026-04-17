package generator

import "fmt"

// Options controls how resource name helpers are emitted. It is intentionally
// empty today — no runtime flags are exposed — but is kept as an explicit type
// so future options can be added without changing Generate's signature or the
// plugin entry point wiring.
type Options struct{}

// Set applies a single `name=value` plugin parameter to the options. The
// signature matches what protogen.Options.ParamFunc expects.
func (o *Options) Set(name, _ string) error {
	return fmt.Errorf("unknown plugin option %q", name)
}
