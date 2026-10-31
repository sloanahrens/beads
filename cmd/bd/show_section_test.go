package main

import (
	"strings"
	"testing"
)

// bd show prints body text as stored. Markdown emphasis keeps its asterisks.
func TestFormatShowSection_PrintsMarkdownAsStored(t *testing.T) {
	body := "Ship the **bold** fix.\n- item one\n- item two"

	got := formatShowSection("DESCRIPTION", body)

	if !strings.Contains(got, body) {
		t.Errorf("section must contain the stored text verbatim\nwant: %q\ngot:  %q", body, got)
	}
}
