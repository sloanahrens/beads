package main

import (
	"encoding/json"
	"testing"
)

// stagedMachineData runs fn in machine mode and returns the data it staged,
// re-marshaled through JSON so the test sees what a caller would decode.
func stagedMachineData(t *testing.T, fn func() error) map[string]any {
	t.Helper()
	env, _, stderr, code := runMachine(t, fn)
	if code != 0 || env.Error != nil {
		t.Fatalf("exit %d error %+v stderr %s", code, env.Error, stderr)
	}
	var m map[string]any
	if err := json.Unmarshal(env.Data, &m); err != nil {
		t.Fatalf("data is not an object: %v\n%s", err, env.Data)
	}
	return m
}

func TestVersionJSONReportsHandshakeFields(t *testing.T) {
	oldCommit := Commit
	Commit = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() { Commit = oldCommit })

	m := stagedMachineData(t, func() error { return versionCmd.RunE(versionCmd, nil) })
	if m["commit"] != Commit || m["build_id"] != Commit {
		t.Fatalf("commit/build_id = %v/%v, want %s", m["commit"], m["build_id"], Commit)
	}
	if m["contract_version"] != float64(JSONContractVersion) {
		t.Fatalf("contract_version = %v", m["contract_version"])
	}
	if _, ok := m["db_schema_version"].(float64); !ok {
		t.Fatalf("db_schema_version missing: %v", m)
	}
	ceiling, ok := m["schema_ceiling"].(map[string]any)
	if !ok {
		t.Fatalf("schema_ceiling missing: %v", m)
	}
	if ceiling["main"] != m["db_schema_version"] {
		t.Fatalf("schema_ceiling.main %v != db_schema_version %v", ceiling["main"], m["db_schema_version"])
	}
	if _, ok := ceiling["ignored"].(float64); !ok {
		t.Fatalf("schema_ceiling.ignored missing: %v", ceiling)
	}
}

func TestVersionJSONCommitAlwaysPresent(t *testing.T) {
	oldCommit := Commit
	Commit = ""
	t.Cleanup(func() { Commit = oldCommit })
	m := stagedMachineData(t, func() error { return versionCmd.RunE(versionCmd, nil) })
	if _, ok := m["commit"]; !ok {
		t.Fatal("commit key omitted when unknown; the handshake needs it present (possibly empty)")
	}
}

func TestCapabilitiesListsCommandTree(t *testing.T) {
	m := stagedMachineData(t, func() error { return capabilitiesCmd.RunE(capabilitiesCmd, nil) })
	if m["contract_version"] != float64(JSONContractVersion) {
		t.Fatalf("contract_version = %v", m["contract_version"])
	}
	cmds, ok := m["commands"].([]any)
	if !ok || len(cmds) == 0 {
		t.Fatalf("commands missing: %v", m["commands"])
	}
	byPath := map[string]map[string]any{}
	for _, c := range cmds {
		cm := c.(map[string]any)
		byPath[cm["path"].(string)] = cm
	}
	for _, want := range []string{"show", "list", "ready", "close", "mol wisp list", "events tail", "config get", "capabilities"} {
		if byPath[want] == nil {
			t.Errorf("command %q missing from capabilities", want)
		}
	}
	hasFlag := func(path, flag string) bool {
		for _, f := range byPath[path]["flags"].([]any) {
			if f.(map[string]any)["name"] == flag {
				return true
			}
		}
		return false
	}
	if !hasFlag("ready", "limit") || !hasFlag("events tail", "since") {
		t.Error("command-local flags missing")
	}
	if !hasFlag("show", "machine") || !hasFlag("show", "json") {
		t.Error("inherited persistent flags missing from a leaf command")
	}
	if byPath["stats"] != nil || !listHas(byPath["status"]["aliases"], "stats") {
		t.Error("aliases must be listed on the command, not as separate paths")
	}
	kinds, ok := m["error_kinds"].(map[string]any)
	if !ok || kinds["partial"] != float64(22) || kinds["guard_not_held"] != float64(13) {
		t.Fatalf("error_kinds = %v", m["error_kinds"])
	}
}

func listHas(list any, s string) bool {
	items, _ := list.([]any)
	for _, it := range items {
		if it == s {
			return true
		}
	}
	return false
}
