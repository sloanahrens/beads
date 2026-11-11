package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/samplesops"
)

// openSamples hands back the generic-samples role for this invocation.
//
// directRequirement is the message ensureDirectMode reports when a workspace is
// reachable by no route. It is per-verb because the shipped text names the
// verb.
func openSamples(directRequirement string) (samplesops.Samples, error) {
	if err := ensureDirectMode(directRequirement); err != nil {
		return nil, err
	}
	return store.Samples()
}

// noteDirectSampleWrite marks the invocation as having written, which is what
// the auto-commit epilogue in main.go keys on. A direct sample write lands in
// the Dolt working set and nothing else commits it, so a verb that forgets to
// call this leaves the row uncommitted.
func noteDirectSampleWrite() {
	commandDidWrite.Store(true)
}

// sampleCmd is the parent `bd sample` verb.
var sampleCmd = &cobra.Command{
	Use:     "sample",
	GroupID: "sync",
	Short:   "Store and read generic timestamped samples",
	Long: `Store and read generic timestamped samples — JSON observations such as a
provider balance or a usage record — in the beads database.

A sample has a kind, an event time and a JSON-object payload. Kinds are opaque
strings chosen by the producer; bd validates their shape, never their
vocabulary. The payload is stored verbatim and listed back byte-for-byte, so a
consumer can compare samples without a normalization step.

The plane is append-only: there is no delete or retention verb.`,
}

// sampleAddCmd appends one sample.
var sampleAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Append one sample",
	Long: `Append one sample row.

  bd sample add --kind <kind> --at <rfc3339> --json <file|->

--at is the event time as an RFC3339 timestamp and is stored in UTC. --json is
the payload: a path to a file holding a JSON object, or "-" to read standard
input. The payload is size-capped at 65536 bytes, validated as a JSON object,
and stored verbatim.

Examples:
  bd sample add --kind deepseek_balance --at 2026-11-11T09:30:00Z --json balance.json
  printf '{"used":1200}' | bd sample add --kind usage_record --at 2026-11-11T09:31:00Z --json -`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		CheckReadonly("sample add")

		kind, _ := cmd.Flags().GetString("kind")
		atRaw, _ := cmd.Flags().GetString("at")
		payloadSource, _ := cmd.Flags().GetString("json")

		// Everything is parsed and validated BEFORE this verb opens the samples
		// role, so a caller's mistake is refused without a write and cannot be
		// mistaken for a write that rolled back.
		at, err := samplesops.ParseAt(atRaw)
		if err != nil {
			return sampleValidationError(err, false)
		}
		if payloadSource == "" {
			return sampleValidationError(fmt.Errorf("%w: --json is required (<file|->)", samplesops.ErrValidation), false)
		}
		payload, err := readSamplePayload(payloadSource)
		if err != nil {
			return HandleErrorRespectJSON("%v", err)
		}
		request := samplesops.AddRequest{Kind: kind, At: at, Payload: payload}
		if err := samplesops.ValidateAddRequest(request); err != nil {
			return sampleValidationError(err, false)
		}

		samples, err := openSamples("sample add requires direct database access")
		if err != nil {
			return HandleError("%v", err)
		}
		result, err := samples.Add(rootCtx, request)
		if err != nil {
			if errors.Is(err, samplesops.ErrValidation) {
				return sampleValidationError(err, false)
			}
			return HandleErrorRespectJSON("storing sample: %v", err)
		}
		noteDirectSampleWrite()

		fmt.Printf("Added sample %s\n", result.Sample.ID)
		return nil
	},
}

// sampleListCmd reads one kind's samples, newest first.
var sampleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a kind's samples, newest first",
	Long: `List one kind's samples, newest first by event time.

  bd sample list --kind <kind> [--since <rfc3339>] [--limit N]

--since keeps rows whose event time is at or after it (inclusive); --limit 0,
the default, is uncapped. With --json the output is the usual envelope:

  {"schema_version": 1,
   "data": [{"id": "...", "kind": "...", "at": "2026-11-11T09:30:00Z",
             "payload": {"used": 1200}, "created_at": "..."}]}

at and created_at are RFC3339 timestamps in UTC; payload is the stored JSON
object, returned byte-for-byte as it was added.

Examples:
  bd sample list --kind deepseek_balance --limit 10 --json
  bd sample list --kind usage_record --since 2026-11-11T00:00:00Z`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		kind, _ := cmd.Flags().GetString("kind")
		sinceRaw, _ := cmd.Flags().GetString("since")
		limit, _ := cmd.Flags().GetInt("limit")

		since, err := samplesops.ParseSince(sinceRaw)
		if err != nil {
			return sampleValidationError(err, jsonOutput)
		}
		request := samplesops.ListRequest{Kind: kind, Since: since, Limit: limit}
		if err := samplesops.ValidateListRequest(request); err != nil {
			return sampleValidationError(err, jsonOutput)
		}

		samples, err := openSamples("sample list requires direct database access")
		if err != nil {
			return HandleError("%v", err)
		}
		result, err := samples.List(rootCtx, request)
		if err != nil {
			if errors.Is(err, samplesops.ErrValidation) {
				return sampleValidationError(err, jsonOutput)
			}
			return HandleErrorRespectJSON("listing samples: %v", err)
		}
		return printSampleList(result.Samples)
	},
}

// sampleValidationError prints a role refusal without the sentinel prefix.
// The role's sentences ARE this command's output, so wrapping one would reword
// text a caller may be matching on into "validation failed: ..." to say the
// same thing.
//
// machineOutput is passed rather than read from the global: `bd sample add`
// shadows the inherited --json bool with its payload flag, so a JSON error on
// that verb would answer a question the caller did not ask.
func sampleValidationError(err error, machineOutput bool) error {
	message := err.Error()
	if errors.Is(err, samplesops.ErrValidation) {
		message = trimValidationPrefix(message)
	}
	if machineOutput {
		jsonStdoutError(message, "")
		return &exitError{Code: 1}
	}
	return HandleError("%s", message)
}

func trimValidationPrefix(message string) string {
	return strings.TrimPrefix(message, samplesops.ErrValidation.Error()+": ")
}

// readSamplePayload reads --json's source, capped one byte past the payload
// limit so validation can refuse an oversize payload without the process
// reading an arbitrarily large file into memory.
func readSamplePayload(source string) ([]byte, error) {
	if source == "-" {
		payload, err := io.ReadAll(io.LimitReader(os.Stdin, samplesops.MaxPayloadBytes+1))
		if err != nil {
			return nil, fmt.Errorf("reading payload from standard input: %w", err)
		}
		return payload, nil
	}
	file, err := os.Open(source) // #nosec G304 -- source is the caller's --json path
	if err != nil {
		return nil, fmt.Errorf("opening payload %s: %w", source, err)
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, samplesops.MaxPayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading payload %s: %w", source, err)
	}
	return payload, nil
}

// sampleJSON is one row of `bd sample list --json`. Payload is embedded as raw
// JSON, not as a string, so a consumer reads the object it stored.
type sampleJSON struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	At        string          `json:"at"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

func printSampleList(samples []samplesops.Sample) error {
	if jsonOutput {
		rows := make([]sampleJSON, len(samples))
		for i, sample := range samples {
			rows[i] = sampleJSON{
				ID:        sample.ID,
				Kind:      sample.Kind,
				At:        sample.At.Format(time.RFC3339Nano),
				Payload:   json.RawMessage(sample.Payload),
				CreatedAt: sample.CreatedAt.Format(time.RFC3339Nano),
			}
		}
		return outputJSON(rows)
	}
	if len(samples) == 0 {
		fmt.Println("No samples")
		return nil
	}
	for _, sample := range samples {
		fmt.Printf("%s  %s  %s\n", sample.At.Format(time.RFC3339Nano), sample.Kind, sample.ID)
	}
	return nil
}

func init() {
	sampleAddCmd.Flags().String("kind", "", "Sample kind (required)")
	sampleAddCmd.Flags().String("at", "", "Event time as an RFC3339 timestamp (required; stored in UTC)")
	sampleAddCmd.Flags().String("json", "", "Payload source: a file holding a JSON object, or - for standard input (required)")
	// Only fails if the flag is missing (caught in tests).
	_ = sampleAddCmd.MarkFlagRequired("kind")
	_ = sampleAddCmd.MarkFlagRequired("at")
	_ = sampleAddCmd.MarkFlagRequired("json")

	sampleListCmd.Flags().String("kind", "", "Sample kind to list (required)")
	sampleListCmd.Flags().String("since", "", "Keep rows whose event time is at or after this RFC3339 timestamp")
	sampleListCmd.Flags().Int("limit", 0, "Maximum rows to return (0 = uncapped)")
	_ = sampleListCmd.MarkFlagRequired("kind") // Only fails if the flag is missing (caught in tests).

	sampleCmd.AddCommand(sampleAddCmd)
	sampleCmd.AddCommand(sampleListCmd)
	rootCmd.AddCommand(sampleCmd)
}
