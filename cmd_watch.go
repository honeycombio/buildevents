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
		Run: func(cmd *cobra.Command, args []string) {
			traceID := strings.TrimSpace(args[0])

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)
			arbitraryFields(*filename, ev) // TODO: consider - move this until after the watch timeout??

			failed, startTime, err := waitCircle(context.Background(), *wcfg)
			if err != nil {
				fmt.Printf("buildevents - Error detected: %s\n", err.Error())
				os.Exit(1)
			}

			status := "success"
			if failed {
				status = "failed"
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
				return
			}
			fmt.Println(url)
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

func waitCircle(parent context.Context, cfg watchConfig) (bool, time.Time, error) {
	client := &circleci.Client{Token: cfg.circleKey}
	wf, err := client.GetWorkflowV2(cfg.workflowID)
	if err != nil {
		return false, time.Now(), err
	}
	start := wf.CreatedAt
	var failed bool

	done := make(chan struct{})
	ctx, cxl := context.WithTimeout(parent, time.Duration(cfg.timeoutMin)*time.Minute)
	defer cxl()

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				failed = true
				return
			case <-time.After(5 * time.Second):
				// move on thru
			}

			var finished bool
			finished, failed = evalWorkflow(client, cfg.workflowID, cfg.jobName)
			if finished {
				return
			}
		}
	}()

	<-done
	return failed, start, nil
}

func evalWorkflow(client *circleci.Client, wfID string, jobName string) (finished bool, failed bool) {
	fmt.Println("polling for finished jobs: ", time.Now().Format(time.StampMilli))
	wfJobs, err := getJobs(client, wfID)
	if err != nil {
		return true, true
	}

	for _, job := range wfJobs {
		// always count ourself as finished so we don't wait for ourself
		if job.Name == jobName {
			continue
		}

		switch job.Status {
		case "success", "blocked":
			continue
		case "failed":
			failed = true
			continue
		case "running":
			// We can stop short as soon as we find an unfinished job
			return false, failed
		}
	}
	return true, failed
}

func getJobs(client *circleci.Client, wfID string) ([]*circleci.WorkflowJob, error) {
	// get the list of jobs, paging if necessary
	wfJobs, more, err := client.ListWorkflowV2Jobs(wfID, nil)
	if err != nil {
		return nil, err
	}
	for more != nil {
		fmt.Printf("getting more jobs! next page is %s\n", *more)
		var moreJobs []*circleci.WorkflowJob
		moreJobs, more, err = client.ListWorkflowV2Jobs(wfID, nil)
		if err != nil {
			return nil, err
		}
		wfJobs = append(wfJobs, moreJobs...)
	}
	return wfJobs, nil
}
