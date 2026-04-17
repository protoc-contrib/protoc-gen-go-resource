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
strongly typed `Parsed<Type>Name` value, parse functions for both short
(`things/{thing}`) and full (`//example.com/things/{thing}`) forms, and
`Name()` / `FullName()` methods that round-trip back to strings. Fields
annotated with `google.api.resource_reference` gain `Parse<Field>()`
convenience methods on the owning message — including across proto
packages.

This project is a fork of
[ucarion/protoc-gen-go-resource](https://github.com/ucarion/protoc-gen-go-resource).
The code generator has been rewritten on top of
`google.golang.org/protobuf/compiler/protogen` (replacing the previous
`text/template`-based renderer), the layout now mirrors the rest of the
`protoc-contrib` plugins, and the build and release pipeline runs on Nix +
`release-please`.

## Features

- **Single-pattern resources** — emits `type Parsed<Type>Name struct { ... }`
  with one field per `{variable}` segment plus
  `Parse<Type>Name` / `ParseFull<Type>Name` / `Name()` / `FullName()`.
- **Multi-pattern resources** — emits a sealed `Parsed<Type>Name`
  interface, one `Parsed<Type>Name_<N>` struct per pattern, and a
  polymorphic `Parse<Type>Name` that tries each pattern in order.
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
- **`ParseFullName()` on messages** — resource messages get both
  `ParseName()` (short form) and `ParseFullName()` (full
  `//service/...` form).

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

type ParsedBookName struct {
    PublisherID string
    BookID      string
}

func ParseBookName(s string) (ParsedBookName, error) {
    parts := strings.Split(s, "/")
    if len(parts) != 4 {
        return ParsedBookName{}, fmt.Errorf("parse %q: bad number of segments, want: 4, got: %d", s, len(parts))
    }
    // ... literal-segment checks elided ...
    return ParsedBookName{PublisherID: parts[1], BookID: parts[3]}, nil
}

func ParseFullBookName(s string) (ParsedBookName, error) { /* strips "//example.com/" */ }

func (n ParsedBookName) Name() string     { return "publishers/" + n.PublisherID + "/books/" + n.BookID }
func (n ParsedBookName) FullName() string { return "//example.com/" + n.Name() }

func (x *Book) ParseName() (ParsedBookName, error)            { /* delegates to ParseBookName */ }
func (x *Book) ParseFullName() (ParsedBookName, error)        { /* delegates to ParseFullBookName */ }
func (x *Book) ParseAuthor() (ParsedAuthorName, error)        { /* delegates to ParseAuthorName */ }
```

For multi-pattern resources (e.g. `Book` with both
`publishers/{publisher}/books/{book}` and `authors/{author}/books/{book}`),
the plugin additionally emits a sealed interface and a polymorphic parser
that tries each pattern in declaration order.

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
