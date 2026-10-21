// Package jsoncommentsparser exposes the JSON-comments parser.
package jsoncommentsparser

import "github.com/yamlstar/yamlstar-plugin-parser-json-comments/parser"

type Event = parser.Event

func Parse(input []byte) ([]Event, error) { return parser.Parse(input) }
