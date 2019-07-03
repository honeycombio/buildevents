package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
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
		Run: func(cmd *cobra.Command, args []string) {
			traceID := args[0]
			stepID := args[1]
			name := args[2]

			var quoted []string
			for _, s := range args[3:] {
				quoted = append(quoted, fmt.Sprintf("\"%s\"", strings.Replace(s, "\"", "\\\"", -1)))
			}
			subcmd := strings.Join(quoted, " ")

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)
			arbitraryFields(*filename, ev)

			spanBytes := make([]byte, 16)
			rand.Read(spanBytes)

			start := time.Now()
			err := runCommand(subcmd)
			dur := time.Since(start)

			ev.Add(map[string]interface{}{
				"trace.parent_id": stepID,
				"trace.span_id":   fmt.Sprintf("%x", spanBytes),
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
		},
	}
	return execCmd
}

func runCommand(subcmd string) error {
	fmt.Println("running /bin/bash -c", subcmd)
	cmd := exec.Command("/bin/bash", "-c", subcmd)

	outReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		io.Copy(os.Stdout, outReader)
	}()
	go func() {
		io.Copy(os.Stderr, errReader)
	}()

	return cmd.Wait()
}
