package main

import (
	"testing"
)

// Write paths must normalize labels the same way read paths already do.
//
// bd applies utils.NormalizeLabels (trim, drop empty, dedupe) to every label
// FILTER path (list, search, ready, orphans, count, workapi) but to no label
// WRITE path. The asymmetry means `--labels 'a, b'` stores " b" with a leading
// space, while `--label ' b'` on a filter is trimmed to "b" — so the stored
// label can never match its own filter and a filtered list is silently short.
func TestGatherCreateInputNormalizesLabels(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			// The comma-space form most people type. pflag's CSV split keeps
			// the leading space, and the split looks like it worked.
			name: "comma space separated",
			args: []string{"--labels", "theme:a, theme:b"},
			want: []string{"theme:a", "theme:b"},
		},
		{
			name: "surrounding whitespace trimmed",
			args: []string{"--labels", "  theme:a  "},
			want: []string{"theme:a"},
		},
		{
			name: "empty elements dropped",
			args: []string{"--labels", "theme:a,,theme:b"},
			want: []string{"theme:a", "theme:b"},
		},
		{
			name: "duplicates collapsed",
			args: []string{"--labels", "theme:a,theme:a"},
			want: []string{"theme:a"},
		},
		{
			// --label is documented as an alias for --labels, so it must
			// normalize identically rather than bypass the cleanup.
			name: "label alias normalized",
			args: []string{"--label", " theme:a , theme:b"},
			want: []string{"theme:a", "theme:b"},
		},
		{
			// The alias appends to --labels; dedupe must span both flags.
			name: "alias and labels deduped together",
			args: []string{"--labels", "theme:a", "--label", " theme:a"},
			want: []string{"theme:a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--title", "Title"}, tt.args...)
			cmd := newCreateFlagsCommand(t, args...)

			in, err := gatherCreateInput(cmd, nil)
			if err != nil {
				t.Fatalf("gatherCreateInput: %v", err)
			}

			assertLabels(t, in.labels, tt.want)
		})
	}
}

func assertLabels(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("labels = %q, want %q", got, want)
		}
	}
}
