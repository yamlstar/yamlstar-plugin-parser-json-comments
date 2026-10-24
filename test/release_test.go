package release_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Mock publication tools and credential lookup so tests never use real remotes.
func TestMain(m *testing.M) {
	if state := os.Getenv("JSON_COMMENTS_RELEASE_TEST_STATE"); state != "" {
		os.Exit(mockReleaseCommand(state))
	}
	os.Exit(m.Run())
}

func mockReleaseCommand(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	state := map[string]string{}
	if err := json.Unmarshal(data, &state); err != nil {
		panic(err)
	}
	command := filepath.Base(os.Args[0]) + " " + strings.Join(os.Args[1:], " ")
	log, err := os.OpenFile(path+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(log, command)
	log.Close()
	switch {
	case command == "git status --porcelain":
		if state["dirty"] == "1" {
			fmt.Println(" M changed")
		}
	case command == "git branch --show-current":
		fmt.Println(state["branch"])
	case command == "git rev-parse HEAD":
		fmt.Println(state["head"])
	case command == "git ls-remote origin refs/heads/main":
		if state["remote"] != "" {
			fmt.Printf("%s\trefs/heads/main\n", state["remote"])
		}
	case strings.HasPrefix(command, "git merge-base --is-ancestor "):
		if state["diverged"] == "1" {
			return 1
		}
	case command == "git ls-remote --exit-code --tags origin refs/tags/v0.1.0":
		if state["tag"] != "1" {
			return 2
		}
		fmt.Printf("%s\trefs/tags/v0.1.0\n", state["head"])
	case command == "git push origin HEAD:main":
		if state["push-fails"] == "1" {
			return 1
		}
	case strings.HasPrefix(command, "gh release view "):
		if state["release"] != "1" {
			return 1
		}
	case strings.HasPrefix(command, "gh release create "):
		if state["create-fails"] == "1" {
			return 1
		}
		if state["missing-tag"] != "1" {
			state["tag"] = "1"
			data, err := json.Marshal(state)
			if err != nil {
				panic(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				panic(err)
			}
		}
	case strings.HasPrefix(command, "curl "):
		artifact := "https://repo.clojars.org/org/yamlstar/" +
			"yamlstar-plugin-parser-json-comments/0.1.0/" +
			"yamlstar-plugin-parser-json-comments-0.1.0"
		if !strings.HasSuffix(command, artifact+".pom") &&
			!strings.HasSuffix(command, artifact+".jar") {
			fmt.Fprintln(os.Stderr, "Unexpected Clojars URL")
			return 99
		}
		if state["check-fails"] == "1" {
			return 7
		}
		status := state["http-status"]
		if status == "" {
			status = "404"
			if state["clojars"] == "1" ||
				(state["incomplete"] == "1" && strings.HasSuffix(command, ".pom")) {
				status = "200"
			}
		}
		fmt.Print(status)
	case command == "lein deploy releases":
		if state["deploy-fails"] == "1" {
			return 1
		}
		user, token := "fixture-user", "fixture-token"
		if expected := state["expected-user"]; expected != "" {
			user = expected
		}
		if expected := state["expected-token"]; expected != "" {
			token = expected
		}
		if os.Getenv("CLOJARS_USERNAME") != user ||
			os.Getenv("CLOJARS_PASSWORD") != token {
			fmt.Fprintln(os.Stderr, "Incorrect deployment credentials")
			return 99
		}
		if state["missing-artifact"] != "1" {
			state["clojars"] = "1"
			data, err := json.Marshal(state)
			if err != nil {
				panic(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				panic(err)
			}
		}
	case strings.HasPrefix(command, "ys -e "):
		if state["ys-fails"] == "1" {
			return 1
		}
		switch os.Args[2] {
		case "print: _.clojars.user || ''":
			fmt.Print(state["file-user"])
		case "print: _.clojars.token || ''":
			fmt.Print(state["file-token"])
		default:
			fmt.Fprintln(os.Stderr, "Unexpected credential selector")
			return 99
		}
	default:
		fmt.Fprintln(os.Stderr, "Unexpected release command:", command)
		return 99
	}
	return 0
}

func TestRelease(t *testing.T) {
	script, err := filepath.Abs("../util/release")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version, dry, message string
		state                       map[string]string
		pushes, creates, deploys    int
	}{
		{name: "release", version: "0.1.0", deploys: 1, creates: 1},
		{name: "publish ahead", version: "0.1.0",
			state:   map[string]string{"remote": "older"},
			deploys: 1, pushes: 1, creates: 1},
		{name: "Clojars already published", version: "0.1.0", creates: 1,
			state: map[string]string{
				"clojars": "1", "no-user": "1", "no-token": "1",
			}},
		{name: "credentials from YAMLStar secrets", version: "0.1.0",
			deploys: 1, creates: 1,
			state: map[string]string{
				"secrets-file": "1", "no-user": "1", "no-token": "1",
				"file-user": "yamlstar-user", "file-token": "yamlstar-token",
				"expected-user": "yamlstar-user", "expected-token": "yamlstar-token",
			}},
		{name: "environment credentials take precedence", version: "0.1.0",
			deploys: 1, creates: 1,
			state: map[string]string{
				"secrets-file": "1", "file-user": "other", "file-token": "other",
			}},
		{name: "partial environment credentials", version: "0.1.0",
			deploys: 1, creates: 1,
			state: map[string]string{
				"secrets-file": "1", "no-token": "1", "file-user": "other",
				"file-token": "yamlstar-token", "expected-token": "yamlstar-token",
			}},
		{name: "dry run", version: "0.1.0", dry: "1",
			state: map[string]string{
				"remote": "older", "no-user": "1", "no-token": "1",
			}},
		{name: "dry run existing Clojars version", version: "0.1.0", dry: "1",
			state: map[string]string{"clojars": "1"}},
		{name: "missing version", message: "Usage:"},
		{name: "invalid version", version: "v0.1.0", message: "form X.Y.Z"},
		{name: "version mismatch", version: "0.2.0", message: "does not match"},
		{name: "dirty tree", version: "0.1.0", message: "not clean",
			state: map[string]string{"dirty": "1"}},
		{name: "wrong branch", version: "0.1.0", message: "must be main",
			state: map[string]string{"branch": "development"}},
		{name: "missing main", version: "0.1.0", message: "does not exist",
			state: map[string]string{"remote": ""}},
		{name: "diverged main", version: "0.1.0", message: "diverged",
			state: map[string]string{"diverged": "1"}},
		{name: "existing tag", version: "0.1.0", message: "already exists",
			state: map[string]string{"tag": "1"}},
		{name: "existing release", version: "0.1.0", message: "already exists",
			state: map[string]string{"release": "1"}},
		{name: "missing Clojars user", version: "0.1.0", message: "CLOJARS_USERNAME",
			state: map[string]string{"no-user": "1"}},
		{name: "missing Clojars token", version: "0.1.0", message: "CLOJARS_PASSWORD",
			state: map[string]string{"no-token": "1"}},
		{name: "missing secrets entry", version: "0.1.0", message: "CLOJARS_PASSWORD",
			state: map[string]string{"secrets-file": "1", "no-token": "1"}},
		{name: "credential lookup failure", version: "0.1.0", message: "Could not read",
			state: map[string]string{
				"secrets-file": "1", "no-token": "1", "ys-fails": "1",
			}},
		{name: "Clojars check failure", version: "0.1.0", message: "Could not check",
			state: map[string]string{"check-fails": "1"}},
		{name: "Clojars HTTP error", version: "0.1.0", message: "HTTP 503",
			state: map[string]string{"http-status": "503"}},
		{name: "incomplete Clojars version", version: "0.1.0", message: "Incomplete",
			state: map[string]string{"incomplete": "1"}},
		{name: "Clojars deployment failure", version: "0.1.0", message: "exit status",
			state: map[string]string{"deploy-fails": "1"}, deploys: 1},
		{name: "missing published artifact", version: "0.1.0", message: "did not publish",
			state: map[string]string{"missing-artifact": "1"}, deploys: 1},
		{name: "publication failure", version: "0.1.0", message: "exit status",
			state:   map[string]string{"remote": "older", "push-fails": "1"},
			deploys: 1, pushes: 1},
		{name: "release failure", version: "0.1.0", message: "exit status",
			state: map[string]string{"create-fails": "1"}, deploys: 1, creates: 1},
		{name: "missing created tag", version: "0.1.0", message: "did not create",
			state: map[string]string{"missing-tag": "1"}, deploys: 1, creates: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, tool := range []string{"git", "gh", "curl", "lein", "ys"} {
				if err := os.Symlink(executable, filepath.Join(dir, tool)); err != nil {
					t.Fatal(err)
				}
			}
			state := map[string]string{
				"branch": "main", "head": "current", "remote": "current",
			}
			for key, value := range tc.state {
				state[key] = value
			}
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "state.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			secrets := filepath.Join(dir, "secrets.yaml")
			if state["secrets-file"] == "1" {
				if err := os.WriteFile(secrets, []byte("clojars: {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			user, token := "fixture-user", "fixture-token"
			if state["no-user"] == "1" {
				user = ""
			}
			if state["no-token"] == "1" {
				token = ""
			}
			args := []string{script}
			if tc.version != "" {
				args = append(args, tc.version)
			}
			cmd := exec.Command("bash", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GH="+filepath.Join(dir, "gh"),
				"LEIN="+filepath.Join(dir, "lein"),
				"CURL="+filepath.Join(dir, "curl"),
				"YS="+filepath.Join(dir, "ys"),
				"YAMLSTAR_SECRETS_FILE="+secrets,
				"CLOJARS_USERNAME="+user, "CLOJARS_PASSWORD="+token,
				"RELEASE_VERSION=0.1.0", "RELEASE_DRY_RUN="+tc.dry,
				"GORACE=atexit_sleep_ms=0",
				"RELEASE_REPO=yamlstar/yamlstar-plugin-parser-json-comments",
				"JSON_COMMENTS_RELEASE_TEST_STATE="+path)
			output, err := cmd.CombinedOutput()
			for _, token := range []string{"fixture-token", "yamlstar-token"} {
				if strings.Contains(string(output), token) {
					t.Fatal("release output exposed a credential")
				}
			}
			if tc.message == "" {
				if err != nil {
					t.Fatalf("release: %v\n%s", err, output)
				}
			} else if err == nil || (tc.message != "exit status" &&
				!strings.Contains(string(output), tc.message)) {
				t.Fatalf("got %v, %s; want %q", err, output, tc.message)
			}
			commands, err := os.ReadFile(path + ".log")
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			log := string(commands)
			if strings.Count(log, "git push ") != tc.pushes ||
				strings.Count(log, "gh release create ") != tc.creates ||
				strings.Count(log, "lein deploy releases") != tc.deploys {
				t.Fatalf("unexpected publication commands:\n%s", log)
			}
			if tc.deploys > 0 {
				deploy := strings.Index(log, "lein deploy releases")
				for _, mutation := range []string{"git push ", "gh release create "} {
					if next := strings.Index(log, mutation); next >= 0 && next < deploy {
						t.Fatalf("GitHub publication preceded Clojars:\n%s", log)
					}
				}
			}
			if state["secrets-file"] == "1" && user != "" && token != "" &&
				strings.Contains(log, "ys -e ") {
				t.Fatal("file lookup overrode complete environment credentials")
			}
			if tc.creates > 0 && !strings.Contains(log,
				"gh release create v0.1.0 --repo "+
					"yamlstar/yamlstar-plugin-parser-json-comments --target current ") {
				t.Fatalf("wrong release target:\n%s", log)
			}
			if tc.dry != "" && !strings.Contains(string(output), "Would create v0.1.0") {
				t.Fatalf("missing dry-run preview: %s", output)
			}
			if tc.dry != "" && state["clojars"] != "1" &&
				!strings.Contains(string(output), "to Clojars") {
				t.Fatalf("missing Clojars dry-run preview: %s", output)
			}
		})
	}
}
