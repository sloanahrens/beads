package formula

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ProblemSyntax is a file that is not valid TOML.
const ProblemSyntax = "syntax"

// LintFileReport is one formula file's problems (empty when clean).
type LintFileReport struct {
	File     string    `json:"file"`
	Problems []Problem `json:"problems"`
}

// LintReport is the result of linting a file or a directory.
type LintReport struct {
	Checked int              `json:"checked"`
	Failed  int              `json:"failed"`
	Files   []LintFileReport `json:"files"`
}

// LintPath checks a .formula.toml file, or every .formula.toml directly in a
// directory, for everything strict decode rejects: keys bd would drop, var
// fields VarDef ignores, gate types nothing resolves, TOML syntax errors and
// structural validation (including required:true with a default). A formula
// that extends another is validated after resolving it against the file's
// own directory followed by searchPaths.
func LintPath(path string, searchPaths []string) (*LintReport, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), FormulaExtTOML) {
				files = append(files, filepath.Join(path, e.Name()))
			}
		}
	} else {
		files = []string{path}
	}
	sort.Strings(files)

	rep := &LintReport{Files: []LintFileReport{}}
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		problems := lintFile(abs, searchPaths)
		rep.Checked++
		if len(problems) > 0 {
			rep.Failed++
		}
		if problems == nil {
			problems = []Problem{}
		}
		rep.Files = append(rep.Files, LintFileReport{File: abs, Problems: problems})
	}
	return rep, nil
}

func lintFile(path string, searchPaths []string) []Problem {
	// #nosec G304 -- path is the file or directory entry the caller named
	data, err := os.ReadFile(path)
	if err != nil {
		// One unreadable file must not hide the rest of the directory.
		return []Problem{{Kind: ProblemSyntax, Message: fmt.Sprintf("cannot read: %v", err)}}
	}
	f, problems, err := DecodeTOMLStrict(data)
	if err != nil {
		p := Problem{Kind: ProblemSyntax, Message: err.Error()}
		var pe toml.ParseError
		if errors.As(err, &pe) {
			p.Line = pe.Position.Line
			p.Message = pe.Message
		}
		return []Problem{p}
	}

	if len(f.Extends) == 0 {
		f.Source = path
		if verr := validationError(f); verr != nil {
			problems = append(problems, problemsOf(verr, path)...)
		}
		return problems
	}
	if len(problems) > 0 {
		// Resolving would stop at this file's own strict error.
		return problems
	}
	parser := NewParser(append([]string{filepath.Dir(path)}, searchPaths...)...)
	parsed, err := parser.ParseFile(path)
	if err == nil {
		_, err = parser.Resolve(parsed)
	}
	if err != nil {
		problems = append(problems, problemsOf(err, path)...)
	}
	return problems
}

// problemsOf turns a parse/resolve error into problems: a *FormulaError
// contributes its own (naming the parent file when the parent is at fault),
// anything else becomes one validation problem.
func problemsOf(err error, path string) []Problem {
	var fe *FormulaError
	if errors.As(err, &fe) {
		if fe.File == "" || fe.File == path {
			return fe.Problems
		}
		out := make([]Problem, 0, len(fe.Problems))
		for _, p := range fe.Problems {
			p.Message = fmt.Sprintf("in extended formula %s: %s", fe.File, p.Message)
			out = append(out, p)
		}
		return out
	}
	return []Problem{{Kind: ProblemValidation, Message: fmt.Sprint(err)}}
}
