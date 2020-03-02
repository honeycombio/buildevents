package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	circleci "github.com/jszwedko/go-circleci"
	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

// numChecks is the number of times to verify that we're finished before
// declaring success in case we enter a transient state with blocked jobs that
// really will start soon. This can be long - wait for up to 2 minutes (5sec *
// 24 = 120sec). It's ok for this to be long because it only covers the time
// when there are existing jobs that are not going to run. Most builds finish
// with all jobs finishing, so this timer will not caused delayed builds in
// those cases.
const numChecks = 24

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

			ok, startTime, endTime, err := waitCircle(context.Background(), *wcfg)
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
				"duration_ms":   endTime.Sub(startTime) / time.Millisecond,
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
// the time it started, and the time it ended (which will be either nowish or
// sometime in the past if we timed out). The err returned is for errors polling
// the CircleCI API, not errors in the build itself.
func waitCircle(parent context.Context, cfg watchConfig) (passed bool, started, ended time.Time, err error) {
	// we need a token to query anything; give a helpful error if we have no token
	if cfg.circleKey == "" {
		return false, time.Now(), time.Now().Add(time.Second), fmt.Errorf("circle token required to poll the API")
	}
	client := &circleci.Client{Token: cfg.circleKey}
	wf, err := client.GetWorkflowV2(cfg.workflowID)
	if err != nil {
		return false, time.Now(), time.Now().Add(time.Second), err
	}
	started = wf.CreatedAt
	ended = time.Now() // set a default in case we early exit

	// set up cancellation timeout based on the configured timout duration
	done := make(chan struct{})
	ctx, cxl := context.WithTimeout(parent, time.Duration(cfg.timeoutMin)*time.Minute)
	defer cxl()

	// sometimes there's a gap between when a job finishes and the next one starts.
	// In that case there are no jobs running and some jobs blocked that could
	// still run. If we think the build has passed and finished, let's give it a
	// buffer to spin up new jobs before really considering it done. This buffer
	// will check for up to 2 minutes
	checksLeft := numChecks + 1 // +1 because we decrement at the beginning of the loop

	go func() {
		defer close(done)
		tk := time.NewTicker(5 * time.Second).C
		for range tk {
			// check for timeout or pause before the next iteration
			select {
			case <-ctx.Done():
				// TODO add the fact that it timed out to the trace to say why it failed
				fmt.Fprintf(os.Stderr, "Timeout reached waiting for the workflow to finish\n")
				ended = time.Now()
				return
			default:
			}

			finished, failed, blocked, err := evalWorkflow(client, cfg.workflowID, cfg.jobName)
			if finished {
				if !failed && !blocked && err == nil {
					// we are legit done.
					passed = true
					fmt.Println("Build passed!")
					return
				}

				// if this is the first time we think we're finished store the timestamp
				if checksLeft >= numChecks {
					ended = time.Now()
				}
				// ok, carry on
				checksLeft--
				if checksLeft <= 0 {
					// we're done checking.
					passed = !failed
					if passed {
						fmt.Println("Build passed!")
					} else {
						fmt.Println("Build failed!")
					}
					return
				}
				if err != nil {
					// we previously successfully queried for the workflow; this is likely a
					// transient error
					fmt.Printf("Querying the CirlceCI API failed with %s; trying %d more times before giving up.\n", err.Error(), checksLeft)
					continue
				}
				if failed {
					// don't bother rechecking if the job has failed but didn't error
					fmt.Printf("Build failed!\n")
					ended = time.Now()
					return
				}
				// yay looks like maybe we're done?
				fmt.Printf("Build appears finished; checking %d more times to make sure.\n", checksLeft)
				continue
			}
			// if we previously thought we were finished but now realize we weren't,
			// reset the check counter so we try 3 times again next time we think we're
			// finished.
			passed = false
			checksLeft = numChecks

		}
	}()

	<-done
	return passed, started, ended, nil
}

// evalWorkflow looks at the CircleCI API for the list of jobs in this workflow
// and decides whether the build has finished and if finished, whether it
// failed. If an error is returned, it represents an error talking to the
// CircleCI API, not an error with the workflow.
func evalWorkflow(client *circleci.Client, wfID string, jobName string) (finished bool, failed bool, blocked bool, err error) {
	fmt.Printf("%s: polling for jobs: ", time.Now().Format(time.StampMilli))
	wfJobs, err := getJobs(client, wfID)
	if err != nil {
		fmt.Printf("error polling: %s\n", err.Error())
		return true, true, false, err
	}
	fmt.Println(summarizeJobList(wfJobs))

	blocked = false
	for _, job := range wfJobs {
		// always count ourself as finished so we don't wait for ourself
		if job.Name == jobName {
			continue
		}

		switch job.Status {
		case "success":
			// success means it passed, don't keep track of it
			continue
		case "blocked":
			// blocked means it can't yet run, but that could be because either
			// it's waiting on a running job, depends on a failed job, or
			// it's not configured to run this build (because of a tag or something)
			blocked = true
			continue
		case "queued":
			return false, failed, blocked, nil
		case "failed":
			failed = true
			continue
		case "running":
			// We can stop short as soon as we find an unfinished job
			return false, failed, blocked, nil
		}
	}

	return true, failed, blocked, nil
}

// getJobs queries the CircleCI API for a list of all jobs in the current workflow
func getJobs(client *circleci.Client, wfID string) ([]*circleci.WorkflowJob, error) {
	// get the list of jobs, paging if necessary
	wfJobs, more, err := client.ListWorkflowV2Jobs(wfID, nil)
	if err != nil {
		return nil, err
	}
	for more != nil {
		// TODO only print this in debug mode
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

// summarizeJobList takes a list of jobs and returns a string summary
func summarizeJobList(wfJobs []*circleci.WorkflowJob) string {
	if len(wfJobs) == 0 {
		return "no jobs found"
	}

	// look at all the jobs and count how many are in each status state
	countByStatus := map[string]int{}
	for _, job := range wfJobs {
		countByStatus[job.Status]++
	}

	// sort the statuses present to print them in a consistent order
	sortedStatusList := make([]string, 0, len(countByStatus))
	for key := range countByStatus {
		sortedStatusList = append(sortedStatusList, key)
	}
	sort.Strings(sortedStatusList)

	// create a list of printable status counts
	statusStrings := make([]string, 0, len(countByStatus))
	for i := 0; i < len(countByStatus); i++ {
		status := sortedStatusList[i]
		count := countByStatus[status]
		statusStrings = append(statusStrings, fmt.Sprintf("%d %s", count, status))
	}

	// join the list of printable statuses to make one nice line
	return strings.Join(statusStrings, ", ")
}
