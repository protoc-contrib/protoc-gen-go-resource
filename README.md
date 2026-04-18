# protoc-gen-go-resource

[![CI](https://github.com/protoc-contrib/protoc-gen-go-resource/actions/workflows/ci.yml/badge.svg)](https://github.com/protoc-contrib/protoc-gen-go-resource/actions/workflows/ci.yml)
[![Coverage](https://raw.githubusercontent.com/protoc-contrib/protoc-gen-go-resource/main/.github/octocov/badge.svg)](https://github.com/protoc-contrib/protoc-gen-go-resource/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/protoc-contrib/protoc-gen-go-resource?include_prereleases)](https://github.com/protoc-contrib/protoc-gen-go-resource/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE.md)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![protoc](https://img.shields.io/badge/protoc-compatible-blue)](https://protobuf.dev)

A [protoc](https://protobuf.dev) plugin that emits Go helpers for parsing
and reconstructing Google API resource names. Given a message annotated
with [`google.api.resource`](https://aip.dev/122), the plugin generates a
strongly typed `<Type>Name` value, parse functions for both short
(`things/{thing}`) and full (`//example.com/things/{thing}`) forms,
`Name()` / `FullName()` / `String()` methods that round-trip back to
strings, and a `Parent()` method returning the parent resource's generated
type. Fields annotated with `google.api.resource_reference` gain
`Parse<Field>()` convenience methods on the owning message — including
across proto packages.

This project is a fork of
[ucarion/protoc-gen-go-resource](https://github.com/ucarion/protoc-gen-go-resource).
The code generator has been rewritten on top of
`google.golang.org/protobuf/compiler/protogen` (replacing the previous
`text/template`-based renderer), the layout now mirrors the rest of the
`protoc-contrib` plugins, and the build and release pipeline runs on Nix +
`release-please`.

## Features

- **Single-pattern resources** — emits `type <Type>Name struct { ... }`
  with one field per `{variable}` segment plus
  `Parse<Type>Name` / `ParseFull<Type>Name` / `Name()` / `FullName()` /
  `String()`.
- **Multi-pattern resources** — emits a sealed `<Type>Name` interface,
  one struct per pattern named after its parent
  (e.g. `PublisherBookName`, `AuthorBookName`), and a polymorphic
  `Parse<Type>Name` that tries each pattern in order. Variants fall back
  to `<Type>Name_<N>` suffixes only when parents aren't declared or
  collide.
- **Parent navigation** — child resources get a `Parent()` method
  returning the matched parent's generated type (e.g.
  `ProjectThingName.Parent()` returns `ProjectName`). Each pattern of a
  multi-pattern resource returns its own parent type.
- **`fmt.Stringer`** — every generated name type implements `String()`,
  returning the relative name, so it prints cleanly with `%v` / `%s`.
- **Resource references** — every field annotated with
  `google.api.resource_reference` (including cross-package references)
  gains a `Parse<Field>()` method on the owning message that delegates to
  the referent's parser.
- **File-level resources** — `google.api.resource_definition` at file
  scope emits parsers even without a backing message.
- **Sensible skips** — `child_type` and wildcard (`"*"`) references are
  silently skipped because the generator has no concrete pattern to bind
  them to.
- **Runtime validation** — generated parsers reject empty variable
  segments (e.g. `things/`) per AIP-122.
- **UUID-typed segments** — declare `(google.api.field_info).format = UUID4`
  on a mirror `<var>_id` field of the resource message to have the
  generated struct field typed as `uuid.UUID` and validated at parse
  time. See [UUID segments](#uuid-segments).
- **`ParseFullName()` on messages** — resource messages get both
  `ParseName()` (short form) and `ParseFullName()` (full
  `//service/...` form).
- **Godoc comments** — every generated struct, interface, function, and
  method carries a one-line doc comment surfacing the pattern and
  resource type.

## Options

Pass plugin options via `--go-resource_opt=key=value` (protoc) or the
`opt:` list under the plugin entry in `buf.gen.yaml`.

| Option                  | Default | Effect                                                                                                                                |
| ----------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `allow_unresolved_refs` | `false` | When `true`, `google.api.resource_reference` fields whose target type isn't in the compilation unit are skipped instead of erroring.  |

## Cross-package references

If a field has `(google.api.resource_reference).type = "example.com/Thing"`
and `Thing` is defined in a different `.proto` file, the referent's file
**must be imported** by the referring file — even if no proto symbol from
it is used — so the generator can see both resources at codegen time. See
[`internal/generator/testpb/reference/reference.proto`](internal/generator/testpb/reference/reference.proto)
for a worked example.

## Installation

```bash
go install github.com/protoc-contrib/protoc-gen-go-resource/cmd/protoc-gen-go-resource@latest
```

## Usage

### With buf

Add the plugin to your `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/proto/go
    opt:
      - paths=source_relative
  - local: protoc-gen-go-resource
    out: gen/proto/go
    opt:
      - paths=source_relative
```

Then run:

```bash
buf generate
```

### With protoc

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --go-resource_out=. --go-resource_opt=paths=source_relative \
  -I proto/ \
  proto/example/v1/example.proto
```

## Example

Given this proto:

```proto
syntax = "proto3";
package example;

import "google/api/resource.proto";

message Publisher {
  option (google.api.resource) = {
    type: "example.com/Publisher"
    pattern: "publishers/{publisher}"
  };
  string name = 1;
}

message Book {
  option (google.api.resource) = {
    type: "example.com/Book"
    pattern: "publishers/{publisher}/books/{book}"
  };

  string name = 1;
  string author = 2 [(google.api.resource_reference) = {
    type: "example.com/Author"
  }];
}
```

The plugin emits (alongside `example.pb.go`) an `example.pb.resource.go`
file with:

```go
// Code generated by protoc-gen-go-resource. DO NOT EDIT.
// source: example.proto

package example

import (
    "fmt"
    "strings"
)

// BookName is the parsed form of a "example.com/Book" resource name
// (pattern "publishers/{publisher}/books/{book}").
type BookName struct {
    PublisherID string
    BookID      string
}

// ParseBookName parses s as BookName (pattern "publishers/{publisher}/books/{book}").
func ParseBookName(s string) (BookName, error) { /* ... */ }

// ParseFullBookName parses the fully-qualified form ("//example.com/...").
func ParseFullBookName(s string) (BookName, error) { /* ... */ }

func (n BookName) Name() string     { return "publishers/" + n.PublisherID + "/books/" + n.BookID }
func (n BookName) FullName() string { return "//example.com/" + n.Name() }
func (n BookName) String() string   { return n.Name() }

// Parent returns the parent PublisherName derived from this resource's fields.
func (n BookName) Parent() PublisherName {
    return PublisherName{PublisherID: n.PublisherID}
}

func (x *Book) ParseName() (BookName, error)            { /* delegates to ParseBookName */ }
func (x *Book) ParseFullName() (BookName, error)        { /* delegates to ParseFullBookName */ }
func (x *Book) ParseAuthor() (AuthorName, error)        { /* delegates to ParseAuthorName */ }
```

For multi-pattern resources (e.g. `Book` with both
`publishers/{publisher}/books/{book}` and `authors/{author}/books/{book}`),
the plugin emits a sealed `BookName` interface, one struct per pattern
named after its parent (`PublisherBookName`, `AuthorBookName`), and a
polymorphic `ParseBookName` that tries each pattern in declaration order.
Each variant's `Parent()` returns the matching parent type.

## UUID segments

By default the plugin types every `{variable}` segment as `string`. When
a segment's value is a UUID, you can declare that at the proto level and
have the generated struct carry `uuid.UUID` directly — no call-site
`uuid.Parse` boilerplate, and invalid UUIDs are rejected by
`Parse<Type>Name` instead of surfacing later.

Annotate a mirror `<var>_id` field on the resource message with
[`google.api.field_info`](https://github.com/googleapis/googleapis/blob/master/google/api/field_info.proto):

```proto
import "google/api/field_info.proto";
import "google/api/resource.proto";

message Collection {
  option (google.api.resource) = {
    type: "example.com/Collection"
    pattern: "collections/{collection}"
  };

  string name = 1;
  string collection_id = 2 [(google.api.field_info).format = UUID4];
}
```

The matching rule is: for each pattern variable `{x}`, if the resource
message carries a field named `x_id` with
`field_info.format = UUID4`, segment `{x}` is typed as `uuid.UUID`.
Other fields are unaffected. With the annotation above the generator
emits:

```go
type CollectionName struct {
    CollectionID uuid.UUID
}

func ParseCollectionName(s string) (CollectionName, error) {
    // ... segment check ...
    v1, err := uuid.Parse(parts[1])
    if err != nil {
        return CollectionName{}, fmt.Errorf("parse %q: segment 1: %w", s, err)
    }
    out.CollectionID = v1
    return out, nil
}

func (n CollectionName) String() string {
    return "collections/" + n.CollectionID.String()
}
```

`github.com/google/uuid` is imported automatically.

**Parent/child consistency.** If a child resource declares `{x}` as
UUID4, every parent resource that shares that variable name must declare
it as UUID4 too (via the same annotation on its own mirror field).
Otherwise `Parent()` cannot flow the typed field into the parent struct
and the generator fails with an explicit error like
`resource "example.com/Item": segment "organization" format (uuid4)
disagrees with parent "example.com/Organization" (string); annotate
organization_id consistently on both messages`.

**Scope.** File-level `google.api.resource_definition` resources have
no backing message and therefore no field to carry `field_info` —
their segments stay `string`. Only `FORMAT_UUID4` is recognized today;
other formats (`IPV4`, `IPV6`, …) fall back to `string`.

## CI Integration

Gate builds on the generated `*.pb.resource.go` files being up-to-date by
running `buf generate` in CI and failing if the worktree is dirty — see
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) for the exact step.

## Contributing

To set up a development environment with [Nix](https://nixos.org):

```bash
nix develop
go test ./...
```

Or, without Nix, ensure `go`, `protoc`, and `buf` are on your `PATH`.

To regenerate the fixtures in `internal/generator/testpb/`, run `buf
generate` from the repository root.

## License

[MIT](LICENSE.md)
