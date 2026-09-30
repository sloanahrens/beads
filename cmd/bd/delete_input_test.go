package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadIssueIDsFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("read valid IDs from file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "ids.txt")
		content := "bd-1\nbd-2\nbd-3\n"
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		ids, err := readIssueIDsFromFile(testFile)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if !reflect.DeepEqual(ids, []string{"bd-1", "bd-2", "bd-3"}) {
			t.Errorf("IDs: got %v, want [bd-1 bd-2 bd-3]", ids)
		}
	})

	t.Run("skip empty lines and comments", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "ids_with_comments.txt")
		content := "bd-1\n\n# This is a comment\nbd-2\n  \nbd-3\n"
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		ids, err := readIssueIDsFromFile(testFile)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if !reflect.DeepEqual(ids, []string{"bd-1", "bd-2", "bd-3"}) {
			t.Errorf("IDs: got %v, want [bd-1 bd-2 bd-3]", ids)
		}
	})

	t.Run("handle non-existent file", func(t *testing.T) {
		_, err := readIssueIDsFromFile(filepath.Join(tmpDir, "nonexistent.txt"))
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestUniqueStrings(t *testing.T) {
	t.Run("remove duplicates while preserving the first occurrence order", func(t *testing.T) {
		got := uniqueStrings([]string{"a", "b", "a", "c", "b", "d"})
		want := []string{"a", "b", "c", "d"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("uniqueStrings(): got %v, want %v", got, want)
		}
	})

	t.Run("handle empty input", func(t *testing.T) {
		if got := uniqueStrings([]string{}); len(got) != 0 {
			t.Errorf("Expected empty result, got %d items", len(got))
		}
	})

	t.Run("handle all unique", func(t *testing.T) {
		got := uniqueStrings([]string{"a", "b", "c"})
		want := []string{"a", "b", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("uniqueStrings(): got %v, want %v", got, want)
		}
	})
}
