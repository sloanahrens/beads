package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// validateUpdateDescription refuses an empty or whitespace-only description on
// `bd update` unless --allow-empty-description says a blank one is meant.
//
// A description is what the landing reviewer and the dispatcher read, so an
// empty one silently un-shapes a bead. The value arrives from a flag a caller
// cannot always see — `--description "$(cat notes.md)"` expands to "" when the
// file failed to write — and the write succeeds, so the loss is silent.
// Refusing the value is the only place the caller learns it. Unlike the
// create-side validateDescriptionUpdate, this covers the explicit inline
// spelling too: on update, `--description ""` is the common way a substitution
// that expanded to nothing gets past the shell, and an update that clears a
// description on purpose has --allow-empty-description to say so.
func validateUpdateDescription(cmd *cobra.Command, ids []string, description string, descChanged bool) error {
	if !descChanged || strings.TrimSpace(description) != "" {
		return nil
	}

	if allowEmptyDescription, _ := cmd.Flags().GetBool("allow-empty-description"); allowEmptyDescription {
		return nil
	}

	return fmt.Errorf("refusing an empty --description for %s: the value is empty or whitespace-only (pass --allow-empty-description if a blank description is intended)", strings.Join(ids, ", "))
}

func descriptionUsesExternalInput(cmd *cobra.Command) bool {
	if stdinFlag, _ := cmd.Flags().GetBool("stdin"); stdinFlag {
		return true
	}
	if cmd.Flags().Changed("body-file") || cmd.Flags().Changed("description-file") {
		return true
	}
	if cmd.Flags().Changed("description") {
		desc, _ := cmd.Flags().GetString("description")
		if desc == "-" {
			return true
		}
	}
	if cmd.Flags().Changed("body") {
		body, _ := cmd.Flags().GetString("body")
		if body == "-" {
			return true
		}
	}
	if cmd.Flags().Changed("message") {
		message, _ := cmd.Flags().GetString("message")
		if message == "-" {
			return true
		}
	}
	return false
}

// validateDescriptionUpdate is the create-side guard: it refuses an empty body
// that came from stdin or a file, but keeps an explicit inline empty
// --description working. `bd update` uses validateUpdateDescription instead
// (be-neiv), because on an update the inline spelling is itself the footgun.
func validateDescriptionUpdate(cmd *cobra.Command, description string, descChanged bool) error {
	if !descChanged || description != "" || !descriptionUsesExternalInput(cmd) {
		return nil
	}

	allowEmptyDescription, _ := cmd.Flags().GetBool("allow-empty-description")
	if allowEmptyDescription {
		return nil
	}

	return fmt.Errorf("empty description from stdin/file requires --allow-empty-description (or use an explicit inline empty value like --description \"\")")
}
