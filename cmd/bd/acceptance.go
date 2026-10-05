package main

import (
	"fmt"
	"strings"

	"github.com/steveyegge/beads/issueops"
)

// A bead's acceptance criteria have two spellings. The first-class
// acceptance_criteria column is what bd show --json reports and what the
// round-trip formats carry. The description's '## Acceptance' section is what
// bd show prints under the same heading and what the spec dispatcher writes:
// a bead minted from the spec template ends with its '- [ ]' lines in the
// description and nothing in the column.
//
// `bd update --acceptance` used to write only the column. On a bead whose
// criteria live in the description it printed success while every '- [ ]'
// line a reader could see stayed exactly where it was, so copying the block
// back with the boxes ticked - the round trip the command is documented for -
// never converged: the next read showed the old block again. The write looked
// applied and nothing a caller could see had moved.
//
// syncAcceptanceSection folds the write into both spellings. The section the
// description already has is the one edited, byte for byte around it, so the
// value reported by `bd show` and by `bd show --json` is the value written.

// syncAcceptanceSection returns the patch to run for one issue, with the
// issue's description carried along when the patch writes acceptance criteria
// and the description has a '## Acceptance' section to keep in step.
//
// It leaves the patch alone in the two cases where the description is not the
// acceptance write's to edit: a patch that does not write acceptance criteria,
// and one that writes the description too (an explicit --description or
// --body-file already says what the description is, and second-guessing it
// would discard what the caller wrote).
func syncAcceptanceSection(patch issueops.IssuePatch, description string) (issueops.IssuePatch, error) {
	if !patch.AcceptanceCriteria.Set || patch.Description.Set {
		return patch, nil
	}
	rewritten, err := rewriteAcceptanceSection(description, patch.AcceptanceCriteria.Value)
	if err != nil {
		return patch, err
	}
	if rewritten != description {
		patch.Description = setField(rewritten)
	}
	return patch, nil
}

// rewriteAcceptanceSection returns description with the body of its
// '## Acceptance' section replaced by criteria.
//
// A description with no such section comes back unchanged: the column is then
// the only spelling of the criteria, so there is nothing to keep in step. A
// description with several such sections names no single place to write, and
// the write is refused rather than aimed at a guess. The body itself is
// refused when it could not survive the trip - see checkAcceptanceBody.
func rewriteAcceptanceSection(description, criteria string) (string, error) {
	sections := findAcceptanceSections(description)
	switch len(sections) {
	case 0:
		return description, nil
	case 1:
	default:
		return "", fmt.Errorf("description has %d '## Acceptance' sections, so --acceptance cannot tell which one the criteria belong to; rewrite the description with --description instead", len(sections))
	}
	if err := checkAcceptanceBody(criteria); err != nil {
		return "", err
	}

	section := sections[0]
	body := strings.TrimSpace(criteria)
	replacement := section.heading + "\n"
	if body != "" {
		replacement += "\n" + body + "\n"
	}
	// What followed the section keeps its text. Trimming the newlines at the
	// seam and adding one back leaves a single blank line between the new body
	// and the next heading, which is the spacing the section arrived with.
	if rest := strings.TrimLeft(description[section.bodyEnd:], "\n"); rest != "" {
		replacement += "\n" + rest
	}
	return description[:section.headingStart] + replacement, nil
}

// checkAcceptanceBody rejects criteria that cannot be stored as the body of
// the description's section: a line that opens a level 1 or 2 heading ends the
// section right there, so the text a reader (or the next parse) finds inside
// the section is not the text the caller asked to write. A deeper heading is a
// subsection of acceptance and stays inside it.
func checkAcceptanceBody(criteria string) error {
	for _, line := range strings.Split(criteria, "\n") {
		if level, _, ok := headingLevel(line); ok && level <= 2 {
			return fmt.Errorf("acceptance criteria cannot be stored in the description's '## Acceptance' section: line %q opens a level %d heading, which would end the section there", strings.TrimSpace(line), level)
		}
	}
	return nil
}

// findAcceptanceSections returns every '## Acceptance' or '## Acceptance
// Criteria' heading in description, with the offsets that delimit the section
// each opens.
//
// Both spellings name the same thing - the spec template writes
// '## Acceptance' and bd lint asks a bug for '## Acceptance Criteria' - so a
// bead carrying either one keeps its criteria there.
func findAcceptanceSections(description string) []acceptanceSection {
	headings := markdownHeadings(description)
	var sections []acceptanceSection
	for i, heading := range headings {
		if !isAcceptanceHeading(heading.title) {
			continue
		}
		// markdownHeadings carries every level 1-2 heading, so the next one is
		// where this section's body ends; the last section runs to the end.
		bodyEnd := len(description)
		if i+1 < len(headings) {
			bodyEnd = headings[i+1].start
		}
		sections = append(sections, acceptanceSection{
			heading:      heading.line,
			headingStart: heading.start,
			bodyEnd:      bodyEnd,
		})
	}
	return sections
}

// acceptanceSection is one acceptance heading in a description, with the
// offsets of the section it opens.
type acceptanceSection struct {
	heading      string // the heading line verbatim, without its newline
	headingStart int    // offset of the heading line's first character
	bodyEnd      int    // offset just past the section: the next level 1-2 heading, or end of description
}

// markdownHeading is one ATX heading line of a description, with the offsets
// of the line it sits on.
type markdownHeading struct {
	title string // heading text, closing '#'s stripped
	line  string // the line verbatim, without its newline
	start int    // offset of the line's first character
}

// markdownHeadings returns every level 1 or 2 ATX heading in description.
// Deeper headings are left out because they do not close a '## ' section: a
// '### ' inside '## Acceptance' belongs to that section's body.
func markdownHeadings(description string) []markdownHeading {
	var headings []markdownHeading
	for offset := 0; offset < len(description); {
		line := description[offset:]
		next := len(description)
		if newline := strings.IndexByte(line, '\n'); newline >= 0 {
			line = line[:newline]
			next = offset + newline + 1
		}
		// A CRLF line is parsed without its carriage return so a description
		// written on Windows still matches the heading; the section is
		// re-emitted with plain newlines, like the body is.
		text := strings.TrimSuffix(line, "\r")
		if level, title, ok := headingLevel(text); ok && level <= 2 {
			headings = append(headings, markdownHeading{
				title: title,
				line:  text,
				start: offset,
			})
		}
		offset = next
	}
	return headings
}

// headingLevel reports the level and text of an ATX heading line, or ok=false
// when the line is not one. Up to three leading spaces are allowed, as
// CommonMark allows them, and a marker not followed by a space or the end of
// the line is not a heading ('#hashtag' is text).
func headingLevel(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	rest := trimmed[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	// A closing run of '#'s is decoration; the title is what remains.
	return level, strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest), "#")), true
}

// isAcceptanceHeading reports whether a heading's text names the acceptance
// criteria section. Case does not matter, and the trailing colon some
// documents write is not part of the name.
func isAcceptanceHeading(title string) bool {
	switch strings.ToLower(strings.TrimSuffix(title, ":")) {
	case "acceptance", "acceptance criteria":
		return true
	default:
		return false
	}
}
