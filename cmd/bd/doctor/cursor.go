package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckCursorSettingsHealth reports malformed .cursor/hooks.json files. A broken
// hooks.json silently disables every Cursor hook, so this is an error (mirrors
// CheckClaudeSettingsHealth). repoPath is the project root directory.
func CheckCursorSettingsHealth(repoPath string) DoctorCheck {
	files := []struct{ path, label string }{
		{filepath.Join(repoPath, ".cursor", "hooks.json"), ".cursor/hooks.json"},
	}
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, struct{ path, label string }{
			filepath.Join(home, ".cursor", "hooks.json"), "~/.cursor/hooks.json",
		})
	}

	var malformed []string
	var checked int
	for _, f := range files {
		data, err := os.ReadFile(f.path) // #nosec G304 -- known safe .cursor locations, not user input
		if err != nil {
			continue
		}
		checked++
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			malformed = append(malformed, fmt.Sprintf("%s: %v", f.label, err))
		}
	}

	if checked == 0 {
		return DoctorCheck{
			Name:    "Cursor Settings Health",
			Status:  StatusOK,
			Message: "No Cursor hooks files found",
		}
	}
	if len(malformed) > 0 {
		return DoctorCheck{
			Name:    "Cursor Settings Health",
			Status:  StatusError,
			Message: fmt.Sprintf("%d malformed hooks file(s)", len(malformed)),
			Detail:  strings.Join(malformed, "\n"),
			Fix:     "Fix the JSON syntax in the listed file(s). A malformed hooks.json silently disables all Cursor hooks.",
		}
	}
	return DoctorCheck{
		Name:    "Cursor Settings Health",
		Status:  StatusOK,
		Message: fmt.Sprintf("%d hooks file(s) valid", checked),
	}
}
