package main

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

func commandStep(cfg *libhoney.Config, filename *string, ciProvider *string) *cobra.Command {
	// STEP - eg: buildevents step $TRAVIS_BUILD_ID $STAGE_SPAN_ID $STAGE_START script
	stepCmd := &cobra.Command{
		Use:   "step [flags] BUILD_ID STEP_ID START_TIME NAME",
		Short: "Joins a collection of individual commands",
		Long: `
The step mode represents a block of related commands. In Travis-CI, this is
one of "install", "before_script", "script", and so on. In CircleCI, this
most closely maps to a single job. It should be run at the end of the step.`,
		Args:                  cobra.ExactArgs(4),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			startTime := parseUnix(strings.TrimSpace(args[2]))
			name := strings.TrimSpace(args[3])

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)

			ev.Add(map[string]interface{}{
				"trace.parent_id": traceID,
				"trace.span_id":   stepID,
				"service_name":    "step",
				"name":            name,
				"duration_ms":     time.Since(startTime) / time.Millisecond,
			})
			ev.Timestamp = startTime

			arbitraryFields(*filename, ev)

			return nil
		},
	}
	return stepCmd
}
