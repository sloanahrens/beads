package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deadMetricsWording lists phrases that only made sense while the
// usage-metrics subsystem existed (deleted in be-xu2.21). Comments and config
// key lists under cmd/bd kept describing it afterwards, sending readers after
// a subsystem that is not there (be-0l02).
var deadMetricsWording = []string{
	"usage-metrics",
	"cli_command",
	"send_metrics",
	"metrics event",
	"metrics bootstrap",
}

// TestNoStaleUsageMetricsReferences fails when a non-test file under cmd/bd
// still mentions the deleted usage-metrics subsystem. Whitespace is collapsed
// before matching because the stale wording was often split across lines
// ("emitting a metrics" / "event").
func TestNoStaleUsageMetricsReferences(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Drop comment markers so a phrase wrapped across two // lines
		// ("emitting a metrics" / "// event") still reads as one sentence.
		flat := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(string(data), "//", "")), " "))
		for _, phrase := range deadMetricsWording {
			if strings.Contains(flat, phrase) {
				t.Errorf("%s still mentions the deleted usage-metrics subsystem (%q)", path, phrase)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan cmd/bd sources: %v", err)
	}
}

// TestMachineModeDocNamesNoMetricsEffect fails when machine.go's description of
// machine mode still lists metrics among the effects it suppresses: machine
// mode toggles nothing metrics-related now, so the doc sends debuggers after a
// switch that does not exist (be-0l02).
func TestMachineModeDocNamesNoMetricsEffect(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("machine.go")
	if err != nil {
		t.Fatalf("read machine.go: %v", err)
	}
	doc := machineModeDoc(string(data))
	if doc == "" {
		t.Fatal("machine.go: machine-mode doc comment not found; update this guard if the comment was reworded")
	}
	if strings.Contains(strings.ToLower(doc), "metric") {
		t.Errorf("machine.go machine-mode doc comment still names metrics: %q", doc)
	}
}

// machineModeDoc returns the // comment paragraph that describes machine mode,
// or "" when there is none.
func machineModeDoc(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if !strings.Contains(strings.ToLower(line), "machine mode is the non-interactive surface") {
			continue
		}
		start := i
		for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "//") {
			start--
		}
		end := i
		for end+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end+1]), "//") {
			end++
		}
		return strings.Join(lines[start:end+1], "\n")
	}
	return ""
}
