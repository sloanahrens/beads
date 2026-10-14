package sqlbuild

import "testing"

// TestGlobToLikePattern exercises globToLikePattern directly against the
// package that actually calls it from BuildIssueFilterClauses (be-ucslk4).
// This used to be pinned only against a byte-identical but unreachable copy
// in internal/storage/issueops, so an escaping regression in this — the
// live — implementation could ship unseen.
func TestGlobToLikePattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "trailing star", in: "tech-*", want: "tech-%"},
		{name: "surrounding stars", in: "*foo*", want: "%foo%"},
		{name: "question mark", in: "v?", want: "v_"},
		{name: "literal percent", in: "5%", want: "5|%"},
		{name: "literal underscore", in: "snake_case", want: "snake|_case"},
		{name: "literal pipe", in: "a|b", want: "a||b"},
		{name: "no metachars", in: "needs-pm", want: "needs-pm"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := globToLikePattern(tc.in)
			if got != tc.want {
				t.Errorf("globToLikePattern(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeLikeWord(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"100%", "100|%"},
		{"snake_case", "snake|_case"},
		{"a|b", "a||b"},
		{`back\slash`, `back\slash`},
		{"", ""},
	}
	for _, tc := range cases {
		if got := escapeLikeWord(tc.in); got != tc.want {
			t.Errorf("escapeLikeWord(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildQueryClause(t *testing.T) {
	t.Parallel()

	const textWord = "(LOWER(title) LIKE ? ESCAPE '|' OR LOWER(description) LIKE ? ESCAPE '|' OR LOWER(notes) LIKE ? ESCAPE '|' OR id LIKE ? ESCAPE '|')"

	t.Run("single word keeps one clause", func(t *testing.T) {
		t.Parallel()
		clause, args := buildQueryClause("Restart", false)
		if clause != textWord {
			t.Errorf("clause = %q, want %q", clause, textWord)
		}
		if len(args) != 4 || args[0] != "%restart%" || args[3] != "%restart%" {
			t.Errorf("args = %v, want four %%restart%%", args)
		}
	})

	t.Run("words are AND-ed in any order, split on any whitespace", func(t *testing.T) {
		t.Parallel()
		clause, args := buildQueryClause("done  intent\trestart", false)
		if want := "(" + textWord + " AND " + textWord + " AND " + textWord + ")"; clause != want {
			t.Errorf("clause = %q, want %q", clause, want)
		}
		if len(args) != 12 || args[0] != "%done%" || args[4] != "%intent%" || args[8] != "%restart%" {
			t.Errorf("args = %v", args)
		}
	})

	t.Run("phrase is one adjacent title match", func(t *testing.T) {
		t.Parallel()
		clause, args := buildQueryClause("overrun guard", true)
		if want := "(LOWER(title) LIKE ? ESCAPE '|' OR id LIKE ? ESCAPE '|')"; clause != want {
			t.Errorf("clause = %q, want %q", clause, want)
		}
		if len(args) != 2 || args[0] != "%overrun guard%" {
			t.Errorf("args = %v", args)
		}
	})

	t.Run("wildcards are escaped in the bound value", func(t *testing.T) {
		t.Parallel()
		_, args := buildQueryClause(`50%_a\b`, false)
		if args[0] != `%50|%|_a\b%` {
			t.Errorf("args[0] = %q, want %q", args[0], `%50|%|_a\b%`)
		}
	})

	t.Run("id-shaped word keeps exact-id, prefix and external_ref", func(t *testing.T) {
		t.Parallel()
		clause, args := buildQueryClause("be-xwu", false)
		want := "(id = ? OR id LIKE ? ESCAPE '|' OR LOWER(title) LIKE ? ESCAPE '|' OR LOWER(description) LIKE ? ESCAPE '|' OR LOWER(notes) LIKE ? ESCAPE '|' OR LOWER(external_ref) LIKE ? ESCAPE '|')"
		if clause != want {
			t.Errorf("clause = %q, want %q", clause, want)
		}
		if len(args) != 6 || args[0] != "be-xwu" || args[1] != "be-xwu%" || args[5] != "%be-xwu%" {
			t.Errorf("args = %v", args)
		}
	})
}
