package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type codexPluginHooksFile struct {
	Hooks map[string][]codexPluginHookGroup `json:"hooks"`
}

type codexPluginHookGroup struct {
	Matcher string            `json:"matcher"`
	Hooks   []codexPluginHook `json:"hooks"`
}

type codexPluginHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// TestCodexPluginHooksRunLiveCommands guards the Codex plugin manifest: every
// command hook it ships must invoke a bd subcommand this binary answers, so a
// deleted verb cannot survive in hooks.json after the CLI removes it.
func TestCodexPluginHooksRunLiveCommands(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "plugins", "beads", ".codex-plugin", "hooks", "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var manifest codexPluginHooksFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(manifest.Hooks) == 0 {
		t.Fatalf("%s declares no hooks", path)
	}

	for event, groups := range manifest.Hooks {
		for i, group := range groups {
			for j, hook := range group.Hooks {
				if hook.Type != "command" {
					continue
				}
				fields := strings.Fields(hook.Command)
				if len(fields) < 2 || fields[0] != "bd" {
					t.Errorf("%s[%d].hooks[%d] command %q is not a bd invocation", event, i, j, hook.Command)
					continue
				}
				cmd, _, err := rootCmd.Find([]string{fields[1]})
				if err != nil || cmd == nil || cmd.Name() != fields[1] {
					t.Errorf("%s[%d].hooks[%d] command %q runs bd %q, which this binary does not answer", event, i, j, hook.Command, fields[1])
				}
			}
		}
	}
}
