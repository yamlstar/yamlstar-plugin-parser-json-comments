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

// Mock both publication tools so these tests cannot contact a real remote.
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
		pushes, creates             int
	}{
		{name: "release", version: "0.1.0", creates: 1},
		{name: "publish ahead", version: "0.1.0",
			state: map[string]string{"remote": "older"}, pushes: 1, creates: 1},
		{name: "dry run", version: "0.1.0", dry: "1",
			state: map[string]string{"remote": "older"}},
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
		{name: "publication failure", version: "0.1.0", message: "exit status",
			state:  map[string]string{"remote": "older", "push-fails": "1"},
			pushes: 1},
		{name: "release failure", version: "0.1.0", message: "exit status",
			state: map[string]string{"create-fails": "1"}, creates: 1},
		{name: "missing created tag", version: "0.1.0", message: "did not create",
			state: map[string]string{"missing-tag": "1"}, creates: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, tool := range []string{"git", "gh"} {
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
			args := []string{script}
			if tc.version != "" {
				args = append(args, tc.version)
			}
			cmd := exec.Command("bash", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GH="+filepath.Join(dir, "gh"),
				"RELEASE_VERSION=0.1.0", "RELEASE_DRY_RUN="+tc.dry,
				"GORACE=atexit_sleep_ms=0",
				"RELEASE_REPO=yamlstar/yamlstar-plugin-parser-json-comments",
				"JSON_COMMENTS_RELEASE_TEST_STATE="+path)
			output, err := cmd.CombinedOutput()
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
				strings.Count(log, "gh release create ") != tc.creates {
				t.Fatalf("unexpected publication commands:\n%s", log)
			}
			if tc.creates > 0 && !strings.Contains(log,
				"gh release create v0.1.0 --repo "+
					"yamlstar/yamlstar-plugin-parser-json-comments --target current ") {
				t.Fatalf("wrong release target:\n%s", log)
			}
			if tc.dry != "" && !strings.Contains(string(output), "Would create v0.1.0") {
				t.Fatalf("missing dry-run preview: %s", output)
			}
		})
	}
}
