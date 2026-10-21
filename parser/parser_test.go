package parser_test

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/glojurelang/glojure/pkg/glj"
	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/yamlstar/yamlstar-plugin-parser-json-comments/parser"
	reference "github.com/yamlstar/yamlstar-plugin-parser-reference/parser"
)

func TestFixtures(t *testing.T) {
	if err := reference.Initialize(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../testdata/cases.edn")
	if err != nil {
		t.Fatal(err)
	}
	cases := glj.Var("clojure.core", "read-string").Invoke(string(data))
	for seq := lang.Seq(cases); seq != nil; seq = seq.Next() {
		item := seq.First()
		field := func(key string) string {
			v := lang.Get(item, lang.NewKeyword(key))
			if v == nil {
				return ""
			}
			return v.(string)
		}
		t.Run(field("name"), func(t *testing.T) {
			got, err := parser.Parse([]byte(field("input")))
			if message := field("error"); message != "" {
				if err == nil || !strings.Contains(err.Error(), message) {
					t.Fatalf("error = %v, want %s", err, message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, err := reference.Parse([]byte(field("want")))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("events = %+v; want %+v", got, want)
			}
		})
	}
}

func TestConcurrentParsers(t *testing.T) {
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 10 {
				events, err := parser.Parse([]byte("true// comment"))
				if err != nil || len(events) < 3 || events[2].Value != "true" {
					t.Errorf("extension: %+v, %v", events, err)
				}
				events, err = reference.Parse([]byte("true// comment"))
				if err != nil || len(events) < 3 || events[2].Value != "true// comment" {
					t.Errorf("reference: %+v, %v", events, err)
				}
			}
		}()
	}
	wait.Wait()
}

func TestInvalidUTF8(t *testing.T) {
	if _, err := parser.Parse([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
