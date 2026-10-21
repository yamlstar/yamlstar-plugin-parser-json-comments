// Copyright 2026 The go-yaml Project Contributors
// SPDX-License-Identifier: Apache-2.0

// Package goyaml provides the JSON-comments parser for go-yaml.
package goyaml

import (
	"errors"
	"fmt"

	jsoncommentsparser "github.com/yamlstar/yamlstar-plugin-parser-json-comments/parser"
	"go.yaml.in/yaml/v4"
)

// Plugin parses YAML with the YAMLStar JSON-comments parser.
type Plugin struct{}

var _ yaml.ParserPlugin = (*Plugin)(nil)

// Version is the linked JSON-comments parser release version.
const Version = jsoncommentsparser.Version

// New creates a JSON-comments parser plugin.
func New() *Plugin { return &Plugin{} }

// Register enables parser=json-comments in yaml.OptsYAML.
// Call it during startup before parsing configuration.
// Registration preserves the native default and rejects duplicate calls.
func Register() error {
	return yaml.RegisterPlugin(yaml.PluginRegistration{
		API: "parser", Name: "json-comments", Version: Version,
		Factory: func(cfg map[string]any) (any, error) {
			if len(cfg) != 0 {
				return nil, errors.New(
					"JSON-comments parser configuration must be empty")
			}
			return New(), nil
		},
	})
}

// Parse implements yaml.ParserPlugin.
func (p *Plugin) Parse(input []byte) ([]yaml.PluginEvent, error) {
	source, err := jsoncommentsparser.Parse(input)
	if err != nil {
		return nil, err
	}
	events := make([]yaml.PluginEvent, len(source))
	for i, event := range source {
		events[i] = yaml.PluginEvent{
			Type: event.Type, Value: event.Value, Anchor: event.Anchor,
			Tag: event.Tag, Style: event.Style, Flow: event.Flow,
			Explicit: event.Explicit,
		}
		if event.Type == "alias" {
			events[i].Anchor = event.Name
		}
		if event.Version != "" {
			switch event.Version {
			case "1.1":
				events[i].Version = &yaml.VersionDirective{
					Major: 1, Minor: 1,
				}
			case "1.2":
				events[i].Version = &yaml.VersionDirective{
					Major: 1, Minor: 2,
				}
			default:
				return nil, fmt.Errorf(
					"JSON-comments parser: unsupported YAML version %q",
					event.Version)
			}
		}
	}
	return events, nil
}
