package goyaml_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yamlstar/yamlstar-plugin-parser-json-comments/goyaml"
	"go.yaml.in/yaml/v4"
	"go.yaml.in/yaml/v4/plugin/limit"
)

func TestFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/comments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Input, Want, Error string }
	if err := yaml.Load(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var got, want []any
			err := yaml.Load([]byte(tc.Input), &got,
				yaml.WithAllDocuments(), yaml.WithPlugin(goyaml.New()))
			if tc.Error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.Error) {
					t.Fatalf("got %v, want %s", err, tc.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Load([]byte(tc.Want), &want, yaml.WithAllDocuments()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestLoaderAndNodes(t *testing.T) {
	input := "--- &a [!!str 1, 'two'] // comment\n"
	loader, err := yaml.NewLoader(strings.NewReader(input), yaml.WithPlugin(goyaml.New()))
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := loader.Load(&node); err != nil {
		t.Fatal(err)
	}
	seq := node.Content[0]
	if seq.Style&yaml.FlowStyle == 0 || seq.Anchor != "a" ||
		seq.Content[0].Tag != "!!str" ||
		seq.Content[1].Style&yaml.SingleQuotedStyle == 0 {
		t.Fatalf("lost node syntax: %#v", seq)
	}
	// A buffered parser still exposes documents through successive Load calls.
	loader, err = yaml.NewLoader(
		strings.NewReader("--- true// first\n--- false\n"),
		yaml.WithPlugin(goyaml.New()))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []bool{true, false} {
		var got bool
		if err := loader.Load(&got); err != nil || got != want {
			t.Fatalf("got %v, %v", got, err)
		}
	}
	for range 2 {
		if err := loader.Load(&node); !errors.Is(err, io.EOF) {
			t.Fatalf("want EOF: %v", err)
		}
	}
}

func TestRegistration(t *testing.T) {
	// The registry is global; isolate startup behavior from other test runs.
	if os.Getenv("JSON_COMMENTS_REGISTER_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegistration$")
		cmd.Env = append(os.Environ(), "JSON_COMMENTS_REGISTER_TEST=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("registration subprocess: %v\n%s", err, output)
		}
		return
	}
	if _, err := yaml.OptsYAML("plugin: {parser: json-comments}"); err == nil {
		t.Fatal("import registered the adapter implicitly")
	}
	if err := goyaml.Register(); err != nil {
		t.Fatal(err)
	}
	for _, config := range []string{
		"plugin: {parser: json-comments}",
		"plugin: {parser: json-comments@v" + goyaml.Version + "}",
		"plugin: {parser: {name: json-comments, version: " +
			goyaml.Version + "}}",
	} {
		opt, err := yaml.OptsYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		var got bool
		if err := yaml.Load([]byte("true// yes"), &got, opt); err != nil || !got {
			t.Fatalf("config %q: got %v, %v", config, got, err)
		}
	}
	for _, tc := range []struct{ config, message string }{
		{"plugin: {parser: {name: json-comments, unknown: true}}",
			"configuration must be empty"},
		{"plugin: {parser: json-comments@v0.0.1}", "version mismatch"},
	} {
		if _, err := yaml.OptsYAML(tc.config); err == nil ||
			!strings.Contains(err.Error(), tc.message) {
			t.Fatalf("config %q: got %v, want %q", tc.config, err, tc.message)
		}
	}
	for _, config := range []string{"{}", "plugin: {parser: true}"} {
		opt, err := yaml.OptsYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		if err := yaml.Load([]byte("true// text"), &got, opt); err != nil ||
			got != "true// text" {
			t.Fatalf("native default changed: got %q, %v", got, err)
		}
	}
	if err := goyaml.Register(); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate registration: %v", err)
	}
}

func TestEventMetadata(t *testing.T) {
	for _, version := range []string{"1.1", "1.2"} {
		t.Run(version, func(t *testing.T) {
			input := "%YAML " + version + "\n--- &a [!!str 1, 'two', *a]\n...\n"
			events, err := goyaml.New().Parse([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 9 || events[0].Type != "stream_start" ||
				events[8].Type != "stream_end" {
				t.Fatalf("incomplete stream: %+v", events)
			}
			doc, seq, scalar, quoted, alias, end :=
				events[1], events[2], events[3], events[4], events[5], events[7]
			if doc.Version == nil || doc.Version.Major != 1 ||
				doc.Version.Minor != int(version[2]-'0') || !doc.Explicit ||
				!end.Explicit {
				t.Fatalf("lost document metadata: %+v, %+v", doc, end)
			}
			if !seq.Flow || seq.Anchor != "a" || scalar.Value != "1" ||
				scalar.Tag != "tag:yaml.org,2002:str" || quoted.Style != "single" ||
				quoted.Value != "two" || alias.Type != "alias" ||
				alias.Anchor != "a" {
				t.Fatalf("lost node metadata: %+v", events)
			}
			for _, event := range events {
				if event.StartMark.Line != 0 || event.StartMark.Column != 0 ||
					event.EndMark.Line != 0 || event.EndMark.Column != 0 {
					t.Fatalf("invented source position: %+v", event)
				}
			}
		})
	}
}

func TestLimitsAndErrors(t *testing.T) {
	plugin := yaml.WithPlugin(goyaml.New())
	for _, tc := range []struct {
		input string
		limit yaml.Option
	}{
		{"[[[0]]] // depth", yaml.WithPlugin(limit.New(limit.DepthValue(2)))},
		{"[&a 1, *a] // aliases", yaml.WithPlugin(limit.New(limit.AliasValue(0)))},
	} {
		var got any
		if err := yaml.Load([]byte(tc.input), &got, plugin, tc.limit); err == nil {
			t.Fatalf("limit ignored for %s", tc.input)
		}
	}
	var got any
	if err := yaml.Load([]byte{0xff}, &got, plugin); err == nil ||
		!strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("got %v", err)
	}
	if err := yaml.Load([]byte("[a, a]"), &got, plugin, plugin); err == nil {
		t.Fatal("multiple parser plugins accepted")
	}
	loader, err := yaml.NewLoader(bytes.NewBufferString("--- true\n--- false"),
		plugin, yaml.WithSingleDocument())
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.Load(&got); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load(&got); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestConcurrentLoads(t *testing.T) {
	plugin := goyaml.New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				var got map[string]bool
				err := yaml.Load([]byte("{a: true /* yes */}"), &got, yaml.WithPlugin(plugin))
				if err != nil || !got["a"] {
					t.Errorf("got %v, %v", got, err)
				}
			}
		}()
	}
	wg.Wait()
}
