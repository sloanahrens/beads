package main

import (
	"encoding/csv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetCommandTreeFlags puts every flag in cmd's tree back to its default and
// clears its Changed bit, so a value one in-process run set cannot reach the
// next run. resetCommandFlag does the same for one flag.
//
// A slice flag is restored with Replace, since Set appends once a slice flag
// has been set. Replace does not clear that state, so the next Set appends to
// the default. Every slice flag in bd's commands defaults to empty, so there
// the next Set matches a fresh flag. A slice flag with a non-empty default
// would keep the default in front of later values.
func resetCommandTreeFlags(cmd *cobra.Command) error {
	var firstErr error
	reset := func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			def, err := parseSliceDefault(flag.DefValue)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			_ = slice.Replace(def)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cmd)
	return firstErr
}

// parseSliceDefault reads a slice flag's DefValue, which pflag writes as
// "[a,b]", back into the values it stands for.
func parseSliceDefault(defValue string) ([]string, error) {
	inner := strings.TrimSuffix(strings.TrimPrefix(defValue, "["), "]")
	if inner == "" {
		return nil, nil
	}
	return csv.NewReader(strings.NewReader(inner)).Read()
}
