// Package parser parses YAML with JSON-style comments.
package parser

import (
	_ "github.com/yamlstar/yamlstar-plugin-parser-json-comments/internal/glojure/pkg/yamlstar_plugin/parser/json_comments"
	reference "github.com/yamlstar/yamlstar-plugin-parser-reference/parser"
)

const Version = "0.1.0"

// Event describes one YAML parser event.
type Event = reference.Event

// Parse returns YAML events from the original UTF-8 source.
// Calls share the reference parser's runtime lock.
func Parse(input []byte) ([]Event, error) {
	return reference.ParseWith(input, "yamlstar-plugin.parser.json-comments")
}
