# YAMLStar JSON-comments parser

A reference-parser grammar extension for YAMLStar and go-yaml.
The Clojure source in this repository is canonical.
The Go package contains its generated namespace and shares the reference
parser engine, initialization, event types, and runtime lock.

```go
import "github.com/yamlstar/yamlstar-plugin-parser-json-comments/parser"

events, err := parser.Parse([]byte("answer: true// comment"))
```

```clojure
(require '[yamlstar-plugin.parser.json-comments :as parser])
(parser/parse "answer: true// comment")
```

Select `--plugin=parser=json-comments` in either host CLI.
For YAMLStar Python, use `yamlstar.parser('json-comments')`.

## go-yaml integration

The `goyaml` package provides the go-yaml adapter in this module.
Use it directly with `yaml.WithPlugin(goyaml.New())`:

```go
import (
    "github.com/yamlstar/yamlstar-plugin-parser-json-comments/goyaml"
    "go.yaml.in/yaml/v4"
)

var value any
err := yaml.Load([]byte("answer: true// comment"), &value,
    yaml.WithPlugin(goyaml.New()))
```

For named selection, call `goyaml.Register()` during application startup,
check its error, and then parse configuration with `yaml.OptsYAML`:

```go
if err := goyaml.Register(); err != nil {
    return err
}
options, err := yaml.OptsYAML("plugin: {parser: json-comments@v0.1.0}")
```

Registration accepts no plugin-specific configuration.
Duplicate registration and mismatched requested versions return errors.
Importing the package does not register it, and registration preserves the
native default parser.
`goyaml.Version` reports the linked parser version.

With go-yaml's generic CLI build support:

```bash
make cli \
  PLUGIN-PACKAGE=github.com/yamlstar/yamlstar-plugin-parser-json-comments/goyaml@v0.1.0 \
  PLUGIN=parser=json-comments
```

`PLUGIN-PACKAGE` links the adapter; `PLUGIN` selects the parser.
A build that only sets `PLUGIN-PACKAGE` retains native defaults and can select
the parser later through configuration or `--plugin=parser=json-comments`.

## Syntax

`//` comments run to the line ending.
`/* */` comments may span lines and do not nest.
Recognized comments are discarded while parsing the original input.
Whitespace is required before comments within ordinary unquoted text.
Complete JSON literals (`null`, `true`, `false`, and JSON numbers) permit
adjacent comments, as do JSON structural boundaries.
URLs and `foo/*bar*/`, `True//text`, and `01//text` remain literal text.
Quoted and block-scalar content retain comment markers.

Block comments can separate parts of a plain scalar.
`foo /* comment */ bar` produces `foo  bar`, preserving the two surrounding
spaces.
Adjacent JSON literals remain separate values: `[true/*c*/false]` is invalid.
The parser also permits adjacent `#` after quoted scalars, commas, `]`, and `}`.
Unterminated recognized block comments are errors.
Comments and source positions are not included in the event interface.

## Development

Makes installs development tools under `.cache/`.
The source artifact and Go dependency use reference parser version `0.2.6`.
For coordinated local development, the sibling checkouts under the host's
`repos/` directory are detected automatically.
`YAML_PARSER_DIR` can select the Clojure source checkout explicitly.
Generation otherwise uses the pinned Clojars source artifact.

```bash
make generate
make test
make test-jvm
make test-race
make check-generated
```

The Clojure and Go tests share `testdata/cases.edn`.
Consumers of the generated Go package need neither Gloat nor a C compiler.
Release the reference source and generated module before publishing this
plugin and updating downstream release builds.
