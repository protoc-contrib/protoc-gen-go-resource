package testpb_test

import (
	"testing"

	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/external"
	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/multipattern"
	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/namefield"
	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/reference"
	"github.com/protoc-contrib/protoc-gen-go-resource/internal/generator/testpb/simple"
)

// TestSimple_Parse covers a one-segment resource (things/{thing}) and its
// error paths: wrong literal prefix, wrong segment count, and the empty
// string. The checked error messages are also the plugin's public API —
// callers rely on them to distinguish failure modes.
func TestSimple_Parse(t *testing.T) {
	cases := []struct {
		in  string
		out simple.ParsedThingName
		err string
	}{
		{in: "things/foo", out: simple.ParsedThingName{ThingID: "foo"}},
		{in: "thing/foo", err: `parse "thing/foo": bad segment 0, want: "things", got: "thing"`},
		{in: "things/foo/bar", err: `parse "things/foo/bar": bad number of segments, want: 2, got: 3`},
		{in: "", err: `parse "": bad number of segments, want: 2, got: 1`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := simple.ParseThingName(tc.in)
			checkErr(t, err, tc.err)
			if got != tc.out {
				t.Fatalf("got %+v, want %+v", got, tc.out)
			}
		})
	}
}

func TestSimple_ParseFull(t *testing.T) {
	cases := []struct {
		in  string
		out simple.ParsedThingName
		err string
	}{
		{in: "//example.com/things/foo", out: simple.ParsedThingName{ThingID: "foo"}},
		{in: "example.com/things/foo", err: `parse "example.com/things/foo": invalid prefix, want: "//example.com/"`},
		{in: "//example.com/thing/foo", err: `parse "thing/foo": bad segment 0, want: "things", got: "thing"`},
		{in: "", err: `parse "": invalid prefix, want: "//example.com/"`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := simple.ParseFullThingName(tc.in)
			checkErr(t, err, tc.err)
			if got != tc.out {
				t.Fatalf("got %+v, want %+v", got, tc.out)
			}
		})
	}
}

func TestSimple_Name(t *testing.T) {
	n := simple.ParsedThingName{ThingID: "foo"}
	if got, want := n.Name(), "things/foo"; got != want {
		t.Errorf("Name: got %q, want %q", got, want)
	}
	if got, want := n.FullName(), "//example.com/things/foo"; got != want {
		t.Errorf("FullName: got %q, want %q", got, want)
	}
}

// TestSimple_Multipart covers a multi-segment single-pattern resource
// (projects/{project}/things/{thing}). Name() round-trips through Parse().
func TestSimple_Multipart(t *testing.T) {
	in := "projects/p/things/t"
	got, err := simple.ParseProjectThingName(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := simple.ParsedProjectThingName{ProjectID: "p", ThingID: "t"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if roundTrip := got.Name(); roundTrip != in {
		t.Fatalf("roundtrip: got %q, want %q", roundTrip, in)
	}
}

func TestSimple_ParseMethod(t *testing.T) {
	thing := &simple.Thing{Name: "things/foo"}
	got, err := thing.ParseName()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != (simple.ParsedThingName{ThingID: "foo"}) {
		t.Fatalf("got %+v", got)
	}
}

// TestMultiPattern covers multi-pattern resources: ParseBookName tries each
// pattern and returns the first match, reporting the aggregated errors
// otherwise.
func TestMultiPattern(t *testing.T) {
	cases := []struct {
		in  string
		out multipattern.ParsedBookName
		err string
	}{
		{in: "publishers/p/books/b", out: multipattern.ParsedBookName_0{PublisherID: "p", BookID: "b"}},
		{in: "authors/a/books/b", out: multipattern.ParsedBookName_1{AuthorID: "a", BookID: "b"}},
		{
			in: "nope/x/books/y",
			err: `no pattern matches input: pattern 0: parse "nope/x/books/y": bad segment 0, want: "publishers", got: "nope"; ` +
				`pattern 1: parse "nope/x/books/y": bad segment 0, want: "authors", got: "nope"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := multipattern.ParseBookName(tc.in)
			checkErr(t, err, tc.err)
			if tc.err == "" && got != tc.out {
				t.Fatalf("got %+v, want %+v", got, tc.out)
			}
		})
	}
}

func TestMultiPattern_FullPrefix(t *testing.T) {
	got, err := multipattern.ParseFullBookName("//example.com/authors/a/books/b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := multipattern.ParsedBookName_1{AuthorID: "a", BookID: "b"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// bad prefix
	_, err = multipattern.ParseFullBookName("bad/authors/a/books/b")
	if err == nil {
		t.Fatal("expected error on bad prefix")
	}
}

func TestMultiPattern_Name(t *testing.T) {
	var n multipattern.ParsedBookName = multipattern.ParsedBookName_1{AuthorID: "a", BookID: "b"}
	if got, want := n.Name(), "authors/a/books/b"; got != want {
		t.Fatalf("Name: got %q, want %q", got, want)
	}
	if got, want := n.FullName(), "//example.com/authors/a/books/b"; got != want {
		t.Fatalf("FullName: got %q, want %q", got, want)
	}
}

func TestNameField(t *testing.T) {
	p := &namefield.Person{PersonName: "persons/alice"}
	got, err := p.ParsePersonName()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != (namefield.ParsedPersonName{PersonID: "alice"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestExternal(t *testing.T) {
	got, err := external.ParseExternalName("external/x")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != (external.ParsedExternalName{ExternalID: "x"}) {
		t.Fatalf("got %+v", got)
	}
}

// TestReference exercises both same-package and cross-package resource
// references. CrossPackage/CrossPackageExternal return names parsed by the
// referent's package — simple and external respectively.
func TestReference(t *testing.T) {
	foo := &reference.Foo{Name: "foos/x", Bar: "bars/y"}
	barParsed, err := foo.ParseBar()
	if err != nil {
		t.Fatalf("ParseBar: %v", err)
	}
	if barParsed != (reference.ParsedBarName{BarID: "y"}) {
		t.Fatalf("got %+v", barParsed)
	}

	xpkg := &reference.CrossPackage{ThingName: "things/z"}
	thingParsed, err := xpkg.ParseThingName()
	if err != nil {
		t.Fatalf("ParseThingName: %v", err)
	}
	if thingParsed != (simple.ParsedThingName{ThingID: "z"}) {
		t.Fatalf("got %+v", thingParsed)
	}

	xpkgExt := &reference.CrossPackageExternal{ExternalName: "external/w"}
	extParsed, err := xpkgExt.ParseExternalName()
	if err != nil {
		t.Fatalf("ParseExternalName: %v", err)
	}
	if extParsed != (external.ParsedExternalName{ExternalID: "w"}) {
		t.Fatalf("got %+v", extParsed)
	}
}

func checkErr(t *testing.T, got error, want string) {
	t.Helper()
	var gotMsg string
	if got != nil {
		gotMsg = got.Error()
	}
	if gotMsg != want {
		t.Fatalf("err mismatch:\n got:  %q\n want: %q", gotMsg, want)
	}
}
