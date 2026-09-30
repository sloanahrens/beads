package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// bd capabilities --json describes this binary's command surface so a caller
// can check for a command or flag instead of probing with trial invocations.

type capabilityFlag struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Type       string `json:"type"`
	Default    string `json:"default"`
	Persistent bool   `json:"persistent"`
	Hidden     bool   `json:"hidden,omitempty"`
}

type capabilityCommand struct {
	// Path is the command path without the binary name ("mol wisp list").
	Path    string           `json:"path"`
	Aliases []string         `json:"aliases"`
	Hidden  bool             `json:"hidden,omitempty"`
	Flags   []capabilityFlag `json:"flags"`
}

type capabilitiesReport struct {
	Version         string              `json:"version"`
	Commit          string              `json:"commit"`
	ContractVersion int                 `json:"contract_version"`
	Commands        []capabilityCommand `json:"commands"`
	ErrorKinds      map[errorKind]int   `json:"error_kinds"`
	// Notes are one-line contract notices a caller should know about.
	Notes []string `json:"notes"`
}

var capabilitiesCmd = &cobra.Command{
	Use:     "capabilities",
	GroupID: "advanced",
	Short:   "Describe the commands, flags and error kinds this bd supports (JSON)",
	Long: `Print this binary's command tree, every command's flags (its own and the
persistent ones it inherits), the JSON contract version, and the machine-mode
error kinds with their exit codes. Programs check this instead of probing for
flags with trial invocations. Output is always JSON.`,
	Args:          cobra.NoArgs,
	Annotations:   map[string]string{skipStoreAnnotation: "1"},
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return outputJSONRaw(buildCapabilities(cmd.Root()))
	},
}

func init() {
	rootCmd.AddCommand(capabilitiesCmd)
}

func buildCapabilities(root *cobra.Command) capabilitiesReport {
	report := capabilitiesReport{
		Version:         Version,
		Commit:          resolveCommitHash(),
		ContractVersion: JSONContractVersion,
		ErrorKinds:      errorKindExitCodes,
		Notes: []string{
			"formula strict decode: on for bd cook under machine mode, bd formula lint, --strict and config formula.strict=true; legacy cook/pour/wisp/mol warn per dropped key; the default flips to strict everywhere once gastown's formulas are clean (gt-fd2cu.3)",
		},
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c != root && c.Name() != "help" && !strings.HasPrefix(c.Name(), "__") {
			report.Commands = append(report.Commands, describeCommand(root, c))
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(report.Commands, func(i, j int) bool { return report.Commands[i].Path < report.Commands[j].Path })
	return report
}

func describeCommand(root, c *cobra.Command) capabilityCommand {
	path := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
	out := capabilityCommand{Path: path, Aliases: c.Aliases, Hidden: c.Hidden, Flags: []capabilityFlag{}}
	if out.Aliases == nil {
		out.Aliases = []string{}
	}
	seen := map[string]bool{}
	add := func(fs *pflag.FlagSet, persistent bool) {
		fs.VisitAll(func(f *pflag.Flag) {
			if seen[f.Name] {
				return
			}
			seen[f.Name] = true
			out.Flags = append(out.Flags, capabilityFlag{
				Name:       f.Name,
				Shorthand:  f.Shorthand,
				Type:       f.Value.Type(),
				Default:    f.DefValue,
				Persistent: persistent,
				Hidden:     f.Hidden,
			})
		})
	}
	add(c.LocalNonPersistentFlags(), false)
	add(c.PersistentFlags(), true)
	add(c.InheritedFlags(), true)
	sort.Slice(out.Flags, func(i, j int) bool { return out.Flags[i].Name < out.Flags[j].Name })
	return out
}
