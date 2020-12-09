package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
	propagation "github.com/honeycombio/beeline-go/propagation"
)

func commandCmd(cfg *libhoney.Config, filename *string, ciProvider *string) *cobra.Command {
	// CMD eg: buildevents cmd $TRAVIS_BUILD_ID $STAGE_SPAN_ID go-test -- go test github.com/honeycombio/hound/...
	execCmd := &cobra.Command{
		Use:   "cmd [flags] BUILD_ID STEP_ID NAME -- [shell command to execute]",
		Short: "Invoke an individual command that is part of the build.",
		Long: `
The cmd mode invokes an individual command that is part of the build, such as
running DB migrations or running a specific test suite. It must be able to be
expressed as a single shell command - either a process like "go test" or a
shell script. The command to run is the final argument to buildevents and
will be launched via "bash -c" using "exec".`,
		Args: composer(
			cobra.MinimumNArgs(4),
			func(cmd *cobra.Command, args []string) error {
				if cmd.ArgsLenAtDash() != 3 {
					return fmt.Errorf("use `--` to signify shell command")
				}
				return nil
			},
		),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage if RunE returns an error. This set in RunE
			// instead of when we instantiate the cmd so we don't suppress usage
			// for errors from Args.
			cmd.SilenceUsage = true

			traceID := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			name := strings.TrimSpace(args[2])

			var quoted []string
			for _, s := range args[3:] {
				quoted = append(quoted, fmt.Sprintf("\"%s\"", strings.Replace(s, "\"", "\\\"", -1)))
			}
			subcmd := strings.Join(quoted, " ")

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)

			spanBytes := make([]byte, 16)
			rand.Read(spanBytes)

			start := time.Now()

			// copy out the current set of fields to avoid later modification
			localFields := map[string]interface{}{}
			for k, v := range ev.Fields() {
				localFields[k] = v
			}
			var spanID = fmt.Sprintf("%x", spanBytes)
			prop := &propagation.PropagationContext{
				TraceID:      traceID,
				ParentID:     spanID,
				TraceContext: localFields,
			}
			err := runCommand(subcmd, prop)
			dur := time.Since(start)

			// Annotate with arbitrary fields after the command runs
			// this way we can consume a file if the command itself generated one
			arbitraryFields(*filename, ev)

			ev.Add(map[string]interface{}{
				"trace.parent_id": stepID,
				"trace.span_id":   spanID,
				"service_name":    "cmd",
				"name":            name,
				"duration_ms":     dur / time.Millisecond,
				"cmd":             subcmd,
			})
			ev.Timestamp = start

			if err == nil {
				ev.AddField("status", "success")
			} else {
				ev.Add(map[string]interface{}{
					"status":         "failure",
					"failure_reason": err.Error(),
				})
			}

			return err
		},
	}
	return execCmd
}

func runCommand(subcmd string, prop *propagation.PropagationContext) error {
	fmt.Println("running /bin/bash -c", subcmd)
	cmd := exec.Command("/bin/bash", "-c", subcmd)

	cmd.Env = append(os.Environ(),
		"HONEYCOMB_TRACE=" + propagation.MarshalHoneycombTraceContext(prop),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
