package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

func commandBuild(cfg *libhoney.Config, filename *string, ciProvider *string) *cobra.Command {
	// BUILD - eg: buildevents build $TRAVIS_BUILD_ID $BUILD_START success
	buildCmd := &cobra.Command{
		Use:   "build [flags] BUILD_ID BUILD_START OUTCOME",
		Short: "Sends the root span for the entire build",
		Long: `
The build mode sends the root span for the entire build. It should be called
when the build finishes and records the duration of the entire build. It emits
a URL pointing to the generated trace in Honeycomb to STDOUT.`,
		Args:                  argOptions(2, "success", "failure"),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			traceID := strings.TrimSpace(args[0])
			startTime := parseUnix(strings.TrimSpace(args[1]))
			outcome := strings.TrimSpace(args[2])

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)
			arbitraryFields(*filename, ev)

			ev.Add(map[string]interface{}{
				"service_name":  "build",
				"trace.span_id": traceID,
				"name":          "build " + traceID,
				"status":        outcome,
				"duration_ms":   time.Since(startTime) / time.Millisecond,
			})
			ev.Timestamp = startTime

			url, err := buildURL(cfg, traceID, startTime.Unix())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to create trace URL: %v\n", err)
			} else {
				fmt.Println(url)
			}
		},
	}
	return buildCmd
}

// composer allows combining several PositionalArgs to work in concert.
func argOptions(pos int, opts ...string) cobra.PositionalArgs {
	return composer(
		cobra.MinimumNArgs(pos+1),
		func(cmd *cobra.Command, args []string) error {
			for _, opt := range opts {
				if strings.TrimSpace(args[pos]) == opt {
					return nil
				}
			}
			if len(opts) == 1 {
				return fmt.Errorf("argument at index %d (%q) must be %q", pos, strings.TrimSpace(args[pos]), opts[0])
			}
			return fmt.Errorf("argument at index %d (%q) must be one of %v", pos, strings.TrimSpace(args[pos]), opts)
		},
	)
}
