// Package generator emits Go helpers for parsing and reconstructing
// Google API resource names declared via google.api.resource and
// google.api.resource_reference annotations.
package generator

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	generatedFilenameSuffix = ".pb.resource.go"

	fmtPackage     = protogen.GoImportPath("fmt")
	stringsPackage = protogen.GoImportPath("strings")
	errorsPackage  = protogen.GoImportPath("errors")
	uuidPackage    = protogen.GoImportPath("github.com/google/uuid")
)

// Generate walks every proto file in the plugin request, builds a registry of
// resource definitions across all files (so cross-file references resolve),
// and emits a *.pb.resource.go companion for every file scheduled for
// generation that actually contains a resource or a resource reference.
func Generate(plugin *protogen.Plugin, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}

	reg := newRegistry()
	for _, f := range plugin.Files {
		if err := reg.walkFile(f); err != nil {
			return err
		}
	}
	reg.annotateFormats()

	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}
		if err := generateFile(plugin, f, reg, opts); err != nil {
			return err
		}
	}
	return nil
}

// registry indexes every resource definition discovered during the walk,
// keyed by proto file, by resource type (so google.api.resource_reference
// fields can resolve to their referent across package boundaries), and by
// the owning message (so collectReferences doesn't have to re-parse the
// google.api.resource annotation).
type registry struct {
	byFile    map[*protogen.File][]*resource
	byType    map[resourceType]*resource
	byMessage map[*protogen.Message]*resource
	// createRequests indexes top- and nested-level messages whose proto name
	// matches "Create<TypeName>Request" (AIP-133). The annotator reads
	// google.api.field_info off the request's <seg>_id field to decide whether
	// a pattern variable is typed as uuid.UUID. Last-writer-wins on name
	// collisions across files — AIP-133 uses one Create request per resource
	// type, so a collision indicates a user-side mistake we don't try to
	// diagnose here.
	createRequests map[string]*protogen.Message
	all            []*resource // insertion order for deterministic lookups
}

func newRegistry() *registry {
	return &registry{
		byFile:         map[*protogen.File][]*resource{},
		byType:         map[resourceType]*resource{},
		byMessage:      map[*protogen.Message]*resource{},
		createRequests: map[string]*protogen.Message{},
	}
}

func (r *registry) insert(f *protogen.File, res *resource) {
	r.byFile[f] = append(r.byFile[f], res)
	r.byType[res.Type] = res
	r.all = append(r.all, res)
	if res.Message != nil {
		r.byMessage[res.Message] = res
	}
}

// findByPattern returns the resource (and matching pattern index) whose
// pattern exactly matches p — same segments, same variable names. Iteration
// is in insertion order to keep generation deterministic.
func (r *registry) findByPattern(p pattern) (*resource, int, bool) {
	for _, res := range r.all {
		for i, rp := range res.Patterns {
			if patternsEqual(rp, p) {
				return res, i, true
			}
		}
	}
	return nil, 0, false
}

// patternsEqual compares two patterns by structure (segment names and whether
// each is a variable). Segment Format is intentionally excluded: format is a
// downstream projection that must not break parent-lookup topology — the
// mismatch case is detected and reported by lookupParent instead.
func patternsEqual(a, b pattern) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Var != b[i].Var {
			return false
		}
	}
	return true
}

func (r *registry) walkFile(f *protogen.File) error {
	fileResources, err := newFileResources(f)
	if err != nil {
		return fmt.Errorf("%s: file-level resources: %w", f.Desc.Path(), err)
	}
	for _, res := range fileResources {
		r.insert(f, res)
	}

	var walk func(msgs []*protogen.Message) error
	walk = func(msgs []*protogen.Message) error {
		for _, m := range msgs {
			res, err := newMessageResource(m)
			if err != nil {
				return fmt.Errorf("%v: %w", m.GoIdent, err)
			}
			if res != nil {
				r.insert(f, res)
			}
			name := string(m.Desc.Name())
			if strings.HasPrefix(name, "Create") && strings.HasSuffix(name, "Request") {
				r.createRequests[name] = m
			}
			if err := walk(m.Messages); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(f.Messages)
}

// annotateFormats fills segment.Format for every variable segment whose
// owning resource has a Create<Resource>Request with
// google.api.field_info.format set on its <seg>_id field. The owning
// resource is the one whose pattern matches the prefix of p up to and
// including the segment — so a child pattern's parent segments pick up
// their format from the parent resource's Create request, and
// lookupParent's consistency check is automatically satisfied.
func (r *registry) annotateFormats() {
	for _, res := range r.all {
		for pi, p := range res.Patterns {
			for si, seg := range p {
				if !seg.Var {
					continue
				}
				owner, _, ok := r.findByPattern(p[:si+1])
				if !ok {
					continue
				}
				crMsg, ok := r.createRequests["Create"+owner.Type.TypeName+"Request"]
				if !ok {
					continue
				}
				idField := seg.Name + "_id"
				for _, f := range crMsg.Fields {
					if string(f.Desc.Name()) != idField {
						continue
					}
					if sf := segmentFormatFromField(f); sf != formatString {
						res.Patterns[pi][si].Format = sf
					}
					break
				}
			}
		}
	}
}

func generateFile(plugin *protogen.Plugin, f *protogen.File, reg *registry, opts *Options) error {
	resources := reg.byFile[f]
	refs, err := collectReferences(f, reg, opts)
	if err != nil {
		return err
	}
	if len(resources) == 0 && len(refs) == 0 {
		return nil
	}

	g := plugin.NewGeneratedFile(f.GeneratedFilenamePrefix+generatedFilenameSuffix, f.GoImportPath)
	g.P("// Code generated by protoc-gen-go-resource. DO NOT EDIT.")
	g.P("// source: ", f.Desc.Path())
	g.P()
	g.P("package ", f.GoPackageName)
	g.P()

	for _, res := range resources {
		if err := emitResource(g, res, reg); err != nil {
			return err
		}
	}
	for _, ref := range refs {
		emitReference(g, ref)
	}
	return nil
}

// emitResource emits the parse/reconstruct helpers for a single resource.
// Single-pattern resources produce a struct; multi-pattern resources produce
// one struct per pattern plus a sealed interface and a polymorphic parser.
func emitResource(g *protogen.GeneratedFile, r *resource, reg *registry) error {
	if len(r.Patterns) == 1 {
		parent, err := lookupParent(r.Patterns[0], reg)
		if err != nil {
			return err
		}
		emitPatternStruct(g, r, r.ParsedType.GoName, r.ParseFunc.GoName, r.FullParseFunc.GoName, "", parent, r.Patterns[0])
		return nil
	}

	embed := "mustEmbed" + r.ParsedType.GoName
	typeNames := variantNames(r, reg)
	implNames := make([]string, len(r.Patterns))
	for i, p := range r.Patterns {
		typeName := typeNames[i]
		funcName := "Parse" + typeName
		parent, err := lookupParent(p, reg)
		if err != nil {
			return err
		}
		// Per-pattern struct + short parser only; the polymorphic full
		// parser below handles prefix stripping, so emitting one per
		// pattern would be dead code.
		emitPatternStruct(g, r, typeName, funcName, "", embed, parent, p)
		implNames[i] = funcName
	}
	emitMultiPatternInterface(g, r, embed, implNames)
	return nil
}

// variantNames returns the Go type names for each pattern of a multi-pattern
// resource. A variant whose pattern has a registered parent is named
// "<ParentType><ResourceType>Name" — e.g., BookName with parent Publisher
// becomes PublisherBookName. A variant falls back to "<ResourceType>Name_N"
// when: its pattern has no parent, the parent isn't registered, or the
// candidate name would collide with another variant's candidate.
func variantNames(r *resource, reg *registry) []string {
	candidates := make([]string, len(r.Patterns))
	count := map[string]int{}
	for i, p := range r.Patterns {
		parentP, ok := parentPattern(p)
		if !ok {
			continue
		}
		parentRes, _, found := reg.findByPattern(parentP)
		if !found {
			continue
		}
		cand := parentRes.Type.TypeName + r.Type.TypeName + "Name"
		candidates[i] = cand
		count[cand]++
	}
	names := make([]string, len(r.Patterns))
	for i, cand := range candidates {
		if cand != "" && count[cand] == 1 {
			names[i] = cand
		} else {
			names[i] = r.ParsedType.GoName + "_" + strconv.Itoa(i)
		}
	}
	return names
}

// parentBinding describes how to build a Parent() method for a pattern.
// It is nil when the pattern has no parent, or when the parent pattern isn't
// matched by any registered resource.
type parentBinding struct {
	// ReturnType is what Parent() declares as its return — the matched
	// resource's generated type (a struct for single-pattern resources, an
	// interface for multi-pattern).
	ReturnType protogen.GoIdent
	// Variant is the concrete struct to construct. For single-pattern
	// parents, Variant == ReturnType. For multi-pattern parents, it's the
	// specific ParsedType_N that matched.
	Variant protogen.GoIdent
	// Pattern is the matched parent pattern — used to build the struct
	// literal field list.
	Pattern pattern
}

func lookupParent(p pattern, reg *registry) (*parentBinding, error) {
	parentP, ok := parentPattern(p)
	if !ok {
		return nil, nil
	}
	parentRes, idx, found := reg.findByPattern(parentP)
	if !found {
		return nil, nil
	}
	matched := parentRes.Patterns[idx]
	// Parent() assigns child fields straight into the parent struct, so the
	// field types must line up. patternsEqual matches on Name+Var only, so
	// Format consistency is verified here and surfaced as a codegen error.
	for i, ps := range matched {
		if !ps.Var {
			continue
		}
		if ps.Format != p[i].Format {
			return nil, fmt.Errorf(
				"resource %q: segment %q format (%s) disagrees with parent %q (%s); annotate %s_id consistently on both messages",
				resourceTypeString(reg, p), ps.Name, p[i].Format, resourceTypeString(reg, matched), ps.Format, ps.Name,
			)
		}
	}
	variant := parentRes.ParsedType
	if len(parentRes.Patterns) > 1 {
		variant = protogen.GoIdent{
			GoName:       variantNames(parentRes, reg)[idx],
			GoImportPath: parentRes.ParsedType.GoImportPath,
		}
	}
	return &parentBinding{
		ReturnType: parentRes.ParsedType,
		Variant:    variant,
		Pattern:    matched,
	}, nil
}

// resourceTypeString returns the declared resource type ("service/Type") that
// owns pattern p, used in error messages. Falls back to the pattern string
// when no resource matches (shouldn't happen for patterns coming out of the
// registry).
func resourceTypeString(reg *registry, p pattern) string {
	if res, _, ok := reg.findByPattern(p); ok {
		return res.Type.ServiceName + "/" + res.Type.TypeName
	}
	return patternString(p)
}

// emitPatternStruct emits a struct, its short Parse function, and the
// Name/FullName methods. If fullFuncName is non-empty a ParseFull companion is
// emitted too. If parent is non-nil, a Parent method returning the parent
// resource's generated type is emitted. If embedFunc is non-empty, a sealed
// interface marker method is emitted so the struct can satisfy the
// multi-pattern interface.
func emitPatternStruct(g *protogen.GeneratedFile, r *resource, typeName, funcName, fullFuncName, embedFunc string, parent *parentBinding, p pattern) {
	prefix := r.servicePrefix()
	patStr := patternString(p)
	fullType := r.Type.ServiceName + "/" + r.Type.TypeName

	if embedFunc != "" {
		g.P("// ", typeName, " is the ", strconv.Quote(patStr), " variant of ", r.ParsedType.GoName, ".")
	} else {
		g.P("// ", typeName, " is the parsed form of a ", strconv.Quote(fullType), " resource name (pattern ", strconv.Quote(patStr), ").")
	}
	g.P("type ", typeName, " struct {")
	for _, s := range p {
		if s.Var {
			g.P(s.FieldName(), " ", segmentGoType(g, s))
		}
	}
	g.P("}")
	g.P()

	g.P("// ", funcName, " parses s as ", typeName, " (pattern ", strconv.Quote(patStr), ").")
	g.P("func ", funcName, "(s string) (", typeName, ", error) {")
	g.P("parts := ", stringsPackage.Ident("Split"), "(s, \"/\")")
	g.P("if len(parts) != ", len(p), " {")
	g.P("return ", typeName, "{}, ", fmtPackage.Ident("Errorf"), "(\"parse %q: bad number of segments, want: ", len(p), ", got: %d\", s, len(parts))")
	g.P("}")
	g.P("var out ", typeName)
	for i, s := range p {
		if s.Var {
			g.P("if parts[", i, "] == \"\" {")
			g.P("return ", typeName, "{}, ", fmtPackage.Ident("Errorf"), "(\"parse %q: empty value for segment ", i, "\", s)")
			g.P("}")
			if s.Format == formatUUID4 {
				g.P("v", i, ", err := ", uuidPackage.Ident("Parse"), "(parts[", i, "])")
				g.P("if err != nil {")
				g.P("return ", typeName, "{}, ", fmtPackage.Ident("Errorf"), "(\"parse %q: segment ", i, ": %w\", s, err)")
				g.P("}")
				g.P("out.", s.FieldName(), " = v", i)
				continue
			}
			g.P("out.", s.FieldName(), " = parts[", i, "]")
			continue
		}
		g.P("if parts[", i, "] != ", strconv.Quote(s.Name), " {")
		g.P("return ", typeName, "{}, ", fmtPackage.Ident("Errorf"), "(\"parse %q: bad segment ", i, ", want: %q, got: %q\", s, ", strconv.Quote(s.Name), ", parts[", i, "])")
		g.P("}")
	}
	g.P("return out, nil")
	g.P("}")
	g.P()

	if fullFuncName != "" {
		g.P("// ", fullFuncName, " parses s as the fully-qualified form of ", typeName, " (prefix ", strconv.Quote(prefix), ").")
		g.P("func ", fullFuncName, "(s string) (", typeName, ", error) {")
		g.P("if !", stringsPackage.Ident("HasPrefix"), "(s, ", strconv.Quote(prefix), ") {")
		g.P("return ", typeName, "{}, ", fmtPackage.Ident("Errorf"), "(\"parse %q: invalid prefix, want: %q\", s, ", strconv.Quote(prefix), ")")
		g.P("}")
		g.P("return ", funcName, "(", stringsPackage.Ident("TrimPrefix"), "(s, ", strconv.Quote(prefix), "))")
		g.P("}")
		g.P()
	}

	g.P("// String returns the relative resource name ", strconv.Quote(patStr), " and implements fmt.Stringer.")
	g.P("func (n ", typeName, ") String() string {")
	g.P("return ", nameExpression(p))
	g.P("}")
	g.P()

	g.P("// FullName returns the fully-qualified resource name prefixed with ", strconv.Quote(prefix), ".")
	g.P("func (n ", typeName, ") FullName() string {")
	g.P("return ", strconv.Quote(prefix), " + n.String()")
	g.P("}")
	g.P()

	g.P("// MarshalText implements encoding.TextMarshaler and emits the relative resource name.")
	g.P("func (n ", typeName, ") MarshalText() ([]byte, error) {")
	g.P("return []byte(n.String()), nil")
	g.P("}")
	g.P()

	g.P("// UnmarshalText implements encoding.TextUnmarshaler by parsing b as a relative ", typeName, ".")
	g.P("func (n *", typeName, ") UnmarshalText(b []byte) error {")
	g.P("parsed, err := ", funcName, "(string(b))")
	g.P("if err != nil {")
	g.P("return err")
	g.P("}")
	g.P("*n = parsed")
	g.P("return nil")
	g.P("}")
	g.P()

	if parent != nil {
		emitParent(g, typeName, parent)
	}

	if embedFunc != "" {
		g.P("func (n ", typeName, ") ", embedFunc, "() {}")
		g.P()
	}
}

// emitParent emits a Parent() method that returns the parent resource's
// generated type. Fields on the parent pattern are mapped by variable name
// from the receiver — patternsEqual already guaranteed those names match.
func emitParent(g *protogen.GeneratedFile, typeName string, parent *parentBinding) {
	g.P("// Parent returns the parent ", parent.ReturnType.GoName, " derived from this resource's fields.")
	g.P("func (n ", typeName, ") Parent() ", parent.ReturnType, " {")
	g.P("return ", parent.Variant, "{")
	for _, s := range parent.Pattern {
		if s.Var {
			g.P(s.FieldName(), ": n.", s.FieldName(), ",")
		}
	}
	g.P("}")
	g.P("}")
	g.P()
}

// parentPattern returns the parent of p — the pattern with its trailing
// collection/{variable} pair removed. Patterns that don't end in a literal
// segment followed by a variable segment (e.g. the top-level
// "things/{thing}") have no parent.
func parentPattern(p pattern) (pattern, bool) {
	if len(p) < 4 || !p[len(p)-1].Var || p[len(p)-2].Var {
		return nil, false
	}
	return p[:len(p)-2], true
}

// segmentGoType returns the value to feed to g.P as the Go type of a variable
// segment's struct field. For the default string format it returns the literal
// "string"; for formatUUID4 it returns a qualified ident into github.com/google/uuid
// so the import is registered.
func segmentGoType(g *protogen.GeneratedFile, s segment) any {
	if s.Format == formatUUID4 {
		return g.QualifiedGoIdent(uuidPackage.Ident("UUID"))
	}
	return "string"
}

// nameExpression builds the string-concatenation expression used in String().
// For "projects/{project}/books/{book}" it yields: "projects/" + n.ProjectID + "/books/" + n.BookID
// UUID4 segments render through their .String() method so the field type stays
// uuid.UUID in the struct.
func nameExpression(p pattern) string {
	var parts []string
	var literal string
	flushLiteral := func() {
		if literal != "" {
			parts = append(parts, strconv.Quote(literal))
			literal = ""
		}
	}
	for i, s := range p {
		if i > 0 {
			literal += "/"
		}
		if s.Var {
			flushLiteral()
			expr := "n." + s.FieldName()
			if s.Format == formatUUID4 {
				expr += ".String()"
			}
			parts = append(parts, expr)
			continue
		}
		literal += s.Name
	}
	flushLiteral()

	var expr string
	for i, part := range parts {
		if i > 0 {
			expr += " + "
		}
		expr += part
	}
	return expr
}

func emitMultiPatternInterface(g *protogen.GeneratedFile, r *resource, embed string, implNames []string) {
	prefix := r.servicePrefix()
	fullType := r.Type.ServiceName + "/" + r.Type.TypeName

	g.P("// ", r.ParsedType.GoName, " is the parsed form of a ", strconv.Quote(fullType), " resource name. It is a sealed interface with one implementation per declared pattern.")
	g.P("type ", r.ParsedType.GoName, " interface {")
	g.P("String() string")
	g.P("FullName() string")
	g.P("MarshalText() ([]byte, error)")
	g.P(embed, "()")
	g.P("}")
	g.P()

	g.P("// ", r.ParseFunc.GoName, " parses s as ", r.ParsedType.GoName, ", trying each pattern in declaration order and returning the first match.")
	g.P("func ", r.ParseFunc.GoName, "(s string) (", r.ParsedType.GoName, ", error) {")
	g.P("var errs []error")
	for _, name := range implNames {
		g.P("{")
		g.P("res, err := ", name, "(s)")
		g.P("if err == nil {")
		g.P("return res, nil")
		g.P("}")
		g.P("errs = append(errs, err)")
		g.P("}")
	}
	g.P("return nil, ", fmtPackage.Ident("Errorf"), "(\"parse %q: no pattern matches: %w\", s, ", errorsPackage.Ident("Join"), "(errs...))")
	g.P("}")
	g.P()

	g.P("// ", r.FullParseFunc.GoName, " parses the fully-qualified form of ", r.ParsedType.GoName, " (prefix ", strconv.Quote(prefix), ") and delegates to ", r.ParseFunc.GoName, ".")
	g.P("func ", r.FullParseFunc.GoName, "(s string) (", r.ParsedType.GoName, ", error) {")
	g.P("if !", stringsPackage.Ident("HasPrefix"), "(s, ", strconv.Quote(prefix), ") {")
	g.P("return nil, ", fmtPackage.Ident("Errorf"), "(\"parse %q: invalid prefix, want: %q\", s, ", strconv.Quote(prefix), ")")
	g.P("}")
	g.P("return ", r.ParseFunc.GoName, "(", stringsPackage.Ident("TrimPrefix"), "(s, ", strconv.Quote(prefix), "))")
	g.P("}")
	g.P()
}

// reference is a message field that carries a resource name the generator can
// parse — either the message's own name field (from google.api.resource) or
// any field carrying google.api.resource_reference.
type reference struct {
	Owner      *protogen.Message
	FieldName  string           // Go name of the field whose value should be parsed
	MethodName string           // full Go method name (e.g. "ParseName", "ParseFullName", "ParseBar")
	ParsedType protogen.GoIdent // return type
	ParseFunc  protogen.GoIdent // callee that does the parsing
}

// collectReferences walks every message in the file and returns the parse
// helper methods to emit on that message, in deterministic order, with
// duplicate-method detection.
func collectReferences(f *protogen.File, reg *registry, opts *Options) ([]reference, error) {
	var out []reference
	var walk func(msgs []*protogen.Message) error
	walk = func(msgs []*protogen.Message) error {
		for _, m := range msgs {
			seen := map[string]bool{}
			record := func(ref reference) error {
				if seen[ref.MethodName] {
					return fmt.Errorf("%v: method %s would be emitted twice", m.GoIdent, ref.MethodName)
				}
				seen[ref.MethodName] = true
				out = append(out, ref)
				return nil
			}

			if res := reg.byMessage[m]; res != nil {
				base := res.NameField.GoName
				if err := record(reference{
					Owner:      m,
					FieldName:  res.NameField.GoName,
					MethodName: "Parse" + base,
					ParsedType: res.ParsedType,
					ParseFunc:  res.ParseFunc,
				}); err != nil {
					return err
				}
				if err := record(reference{
					Owner:      m,
					FieldName:  res.NameField.GoName,
					MethodName: "ParseFull" + base,
					ParsedType: res.ParsedType,
					ParseFunc:  res.FullParseFunc,
				}); err != nil {
					return err
				}
			}

			for _, field := range m.Fields {
				ref, ok, err := fieldReference(field, reg, opts)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if err := record(ref); err != nil {
					return err
				}
			}

			if err := walk(m.Messages); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(f.Messages); err != nil {
		return nil, err
	}
	return out, nil
}

func fieldReference(field *protogen.Field, reg *registry, opts *Options) (reference, bool, error) {
	opt, _ := field.Desc.Options().(*descriptorpb.FieldOptions)
	rr, _ := proto.GetExtension(opt, annotations.E_ResourceReference).(*annotations.ResourceReference)
	if rr == nil {
		return reference{}, false, nil
	}
	// Ignore fields that don't set "type", set it to the wildcard, or only
	// carry a child_type reference — the generator can't bind those to a
	// concrete pattern.
	if rr.Type == "" || rr.Type == "*" {
		return reference{}, false, nil
	}
	if field.Desc.Kind() != protoreflect.StringKind || field.Desc.IsList() || field.Desc.IsMap() {
		return reference{}, false, fmt.Errorf("%v: resource_reference must be on a singular string field", field.GoIdent)
	}
	rt, err := newResourceType(rr.Type)
	if err != nil {
		return reference{}, false, fmt.Errorf("%v: resource reference: %w", field.GoIdent, err)
	}
	res, ok := reg.byType[rt]
	if !ok {
		if opts.AllowUnresolvedRefs {
			return reference{}, false, nil
		}
		return reference{}, false, fmt.Errorf("%v: reference to unknown type %q (set allow_unresolved_refs=true to skip)", field.GoIdent, rr.Type)
	}
	return reference{
		Owner:      field.Parent,
		FieldName:  field.GoName,
		MethodName: "Parse" + field.GoName,
		ParsedType: res.ParsedType,
		ParseFunc:  res.ParseFunc,
	}, true, nil
}

func emitReference(g *protogen.GeneratedFile, ref reference) {
	g.P("// ", ref.MethodName, " parses x.", ref.FieldName, " as ", ref.ParsedType.GoName, ".")
	g.P("func (x *", ref.Owner.GoIdent, ") ", ref.MethodName, "() (", ref.ParsedType, ", error) {")
	g.P("return ", ref.ParseFunc, "(x.", ref.FieldName, ")")
	g.P("}")
	g.P()
}

// patternString renders a parsed pattern back to its canonical form —
// "foo/{bar}/baz/{qux}" — for use in generated doc comments.
func patternString(p pattern) string {
	var sb strings.Builder
	for i, s := range p {
		if i > 0 {
			sb.WriteByte('/')
		}
		if s.Var {
			sb.WriteByte('{')
			sb.WriteString(s.Name)
			sb.WriteByte('}')
		} else {
			sb.WriteString(s.Name)
		}
	}
	return sb.String()
}
