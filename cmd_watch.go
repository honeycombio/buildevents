package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	circleci "github.com/jszwedko/go-circleci"
	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

// numChecks is the number of times we attempt to query workflow information
// before giving up.
const numChecks = 3

type watchConfig struct {
	timeoutMin int
	circleKey  string
	workflowID string
	jobName    string
}

func commandWatch(cfg *libhoney.Config, filename *string, ciProvider *string, wcfg *watchConfig) *cobra.Command {
	// WATCH eg: buildevents watch $TRAVIS_BUILD_ID
	watchCmd := &cobra.Command{
		Use:   "watch BUILD_ID",
		Short: "Polls the CircleCI API and waits until all jobs have finished.",
		Long: `
Polls the CircleCI API and waits until all jobs have finished (either
succeeded, failed, or are blocked). It then reports the final status of the
build with the appropriate timers.`,
		Args: composer(
			cobra.ExactArgs(1),
			func(cmd *cobra.Command, args []string) error {
				if *ciProvider != providerCircle {
					return fmt.Errorf("watch command only valid for %s", providerCircle)
				}
				return nil
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := strings.TrimSpace(args[0])

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)
			arbitraryFields(*filename, ev) // TODO: consider - move this until after the watch timeout??

			ok, startTime, err := waitCircle(context.Background(), *wcfg)
			if err != nil {
				fmt.Printf("buildevents - Error detected: %s\n", err.Error())
				return err
			}

			status := "failed"
			if ok {
				status = "success"
			}

			ev.Add(map[string]interface{}{
				"service_name":  "watch",
				"trace.span_id": traceID,
				"name":          "watch " + traceID,
				"status":        status,
				"duration_ms":   time.Since(startTime) / time.Millisecond,
			})
			ev.Timestamp = startTime

			url, err := buildURL(cfg, traceID, startTime.Unix())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to create trace URL: %v\n", err)
				return err
			}
			fmt.Println(url)

			return nil
		},
	}

	watchCmd.Flags().IntVarP(&wcfg.timeoutMin, "timeout", "t", 10, "[env.BUILDEVENT_TIMEOUT] maximum time (in minutes) that watch should wait before timing out")
	if ts, ok := os.LookupEnv("BUILDEVENT_TIMEOUT"); ok {
		// This will end up ignoring non-integer values in the envvar
		if _, err := strconv.Atoi(ts); err == nil {
			watchCmd.Flags().Lookup("timeout").Value.Set(ts)
		}
	}

	watchCmd.Flags().StringVarP(&wcfg.circleKey, "circlekey", "c", "", "[env.BUILDEVENT_CIRCLE_API_TOKEN] CircleCI API token used for watching builds for private repositories")
	if tok, ok := os.LookupEnv("BUILDEVENT_CIRCLE_API_TOKEN"); ok {
		watchCmd.Flags().Lookup("circlekey").Value.Set(tok)
	}

	watchCmd.Flags().StringVarP(&wcfg.workflowID, "workflowid", "w", "", "[env.CIRCLE_WORKFLOW_ID] CircleCI identifier for the current workflow")
	if wfid, ok := os.LookupEnv("CIRCLE_WORKFLOW_ID"); ok {
		watchCmd.Flags().Lookup("workflowid").Value.Set(wfid)
	}

	watchCmd.Flags().StringVarP(&wcfg.jobName, "jobname", "j", "", "[env.CIRCLE_JOB] CircleCI identifier for the current job")
	if jnm, ok := os.LookupEnv("CIRCLE_JOB"); ok {
		watchCmd.Flags().Lookup("jobname").Value.Set(jnm)
	}

	return watchCmd
}

// waitCircle polls the CircleCI API checking for the status of this workflow
// and the jobs it contains. It returns whether the workflow build succeeded,
// the time it started. The err returned is for errors polling the CircleCI API,
// not errors in the build itself.
func waitCircle(parent context.Context, cfg watchConfig) (bool, time.Time, error) {
	// we need a token to query anything; give a helpful error if we have no token
	if cfg.circleKey == "" {
		return false, time.Now(), fmt.Errorf("circle token required to poll the API")
	}
	client := &circleci.Client{Token: cfg.circleKey}
	wf, err := client.GetWorkflowV2(cfg.workflowID)
	if err != nil {
		return false, time.Now(), err
	}
	start := wf.CreatedAt

	ctx, cxl := context.WithTimeout(parent, time.Duration(cfg.timeoutMin)*time.Minute)
	defer cxl()

	checksLeft := numChecks

	tk := time.NewTicker(5 * time.Second)
	for {
		// check for timeout or pause before the next iteration
		select {
		case <-ctx.Done():
			// TODO add the fact that it timed out to the trace to say why it failed
			fmt.Fprintf(os.Stderr, "\nTimeout reached waiting for the workflow to finish\n")
			return false, start, nil
		case <-tk.C:
		}

		wf, err = client.GetWorkflowV2(cfg.workflowID)
		if err != nil {
			// we previously successfully queried for the workflow; this is likely a transient error
			fmt.Printf("\nQuerying the CirlceCI API failed with %s;", err.Error())
			checksLeft--
			if checksLeft > 0 {
				fmt.Printf(" trying %d more times before giving up.\n", checksLeft)
				continue
			}
			fmt.Println()
			return false, start, nil
		}
		checksLeft = numChecks
		status := wf.Status

		switch status {
		case "running", "failing", "on_hold":
			fmt.Print(".")
			continue
		case "success":
			fmt.Println("\nBuild passed!")
			return true, start, nil
		default:
			fmt.Printf("\nBuild failed with status: %s\n", status)
			return false, start, nil
		}
	}
}
