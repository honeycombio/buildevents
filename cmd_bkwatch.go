package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/spf13/cobra"
)

type bkWatchConfig struct {
	timeoutMin   int
	apiToken     string
	orgSlug      string
	pipelineSlug string
	buildNumber  string
	jobID        string
}

func commandBKWatch(cfg *libhoney.Config, filename *string, ciProvider *string) *cobra.Command {
	// WATCH eg: buildevents bk_watch $BUILDKITE_BUILD_ID
	var wcfg bkWatchConfig
	bkWatchCmd := &cobra.Command{
		Use:   "bk_watch BUILD_ID",
		Short: "Polls the Buildkite API and waits until all jobs have finished.",
		Long: `
Polls the CircleCI API and waits until all jobs have finished (either
succeeded, failed, or are blocked). It then reports the final status of the
build with the appropriate timers.`,
		Args: cobra.MatchAll(
			cobra.ExactArgs(1),
			func(cmd *cobra.Command, args []string) error {
				if *ciProvider != providerBuildkite {
					return fmt.Errorf("bk_watch command only valid for %s", providerBuildkite)
				}
				return nil
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := strings.TrimSpace(args[0])

			ev := createEvent(cfg, *ciProvider, traceID)
			defer ev.Send()

			providerInfo(*ciProvider, ev)

			ok, startTime, endTime, err := waitBuildkite(context.Background(), wcfg)
			if err != nil {
				fmt.Printf("buildevents - Error detected: %s\n", err.Error())
				return err
			}

			status := "failed"
			if ok {
				status = "success"
			}

			ev.Add(map[string]interface{}{
				"service_name":  ifClassic(cfg.APIKey, "watch", cfg.Dataset),
				"service.name":  ifClassic(cfg.APIKey, "watch", cfg.Dataset),
				"command_name":  "watch",
				"trace.span_id": traceID,
				"name":          ifClassic(cfg.APIKey, "watch "+traceID, "watch"),
				"status":        status,
				"duration_ms":   endTime.Sub(startTime) / time.Millisecond,
				"source":        "buildevents",
			})
			ev.Timestamp = startTime

			arbitraryFields(*filename, ev) // TODO: consider - move this until after the watch timeout??

			url, err := buildURL(cfg, traceID, startTime.Unix())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to create trace URL: %v\n", err)
			} else {
				fmt.Println(url)
			}
			return nil
		},
	}

	bkWatchCmd.Flags().IntVarP(&wcfg.timeoutMin, "timeout", "t", 10, "[env.BUILDEVENT_TIMEOUT] maximum time (in minutes) that watch should wait before timing out")
	if timeout, ok := os.LookupEnv("BUILDEVENT_TIMEOUT"); ok {
		// This will end up ignoring non-integer values in the envvar
		if _, err := strconv.Atoi(timeout); err == nil {
			bkWatchCmd.Flags().Lookup("timeout").Value.Set(timeout)
		}
	}

	bkWatchCmd.Flags().StringVarP(&wcfg.apiToken, "apitoken", "", "", "[env.BUILDEVENT_BUILDKITE_API_TOKEN] Buildkite API token used for watching builds")
	if tok, ok := os.LookupEnv("BUILDEVENT_BUILDKITE_API_TOKEN"); ok {
		bkWatchCmd.Flags().Lookup("apitoken").Value.Set(tok)
	}

	bkWatchCmd.Flags().StringVarP(&wcfg.orgSlug, "org", "", "", "[env.BUILDKITE_ORGANIZATION_SLUG] Buildkite organization slug")
	if org, ok := os.LookupEnv("BUILDKITE_ORGANIZATION_SLUG"); ok {
		bkWatchCmd.Flags().Lookup("org").Value.Set(org)
	}

	bkWatchCmd.Flags().StringVarP(&wcfg.pipelineSlug, "pipeline", "", "", "[env.BUILDKITE_PIPELINE_SLUG] Buildkite pipeline slug")
	if pipeline, ok := os.LookupEnv("BUILDKITE_PIPELINE_SLUG"); ok {
		bkWatchCmd.Flags().Lookup("pipeline").Value.Set(pipeline)
	}

	bkWatchCmd.Flags().StringVarP(&wcfg.buildNumber, "buildnum", "", "", "[env.BUILDKITE_BUILD_NUMBER] CircleCI identifier for the current workflow")
	if buildNum, ok := os.LookupEnv("BUILDKITE_BUILD_NUMBER"); ok {
		bkWatchCmd.Flags().Lookup("buildnum").Value.Set(buildNum)
	}

	bkWatchCmd.Flags().StringVarP(&wcfg.jobID, "jobid", "", "", "[env.BUILDKITE_JOB_ID] Buildkite identifier for the current job")
	if jobid, ok := os.LookupEnv("BUILDKITE_JOB_ID"); ok {
		bkWatchCmd.Flags().Lookup("jobid").Value.Set(jobid)
	}

	return bkWatchCmd
}

// waitBuildkite polls the Buildkite API checking for the status of this build
// and the jobs it contains. It returns whether the build succeeded,
// the time it started, and the time it ended (which will be either nowish or
// sometime in the past if we timed out). The err returned is for errors polling
// the Buildkite API, not errors in the build itself.
func waitBuildkite(parent context.Context, cfg bkWatchConfig) (passed bool, started, ended time.Time, err error) {

	// we need a token to query anything; give a helpful error if we have no token
	if cfg.apiToken == "" {
		return false, time.Now(), time.Now().Add(time.Second), fmt.Errorf("buildkite token required to poll the API")
	}

	config, err := buildkite.NewTokenConfig(cfg.apiToken, false)

	if err != nil {
		log.Fatalf("client config failed: %s", err)
	}

	client := buildkite.NewClient(config.Client())

	build, err := bkGetBuild(client, cfg)
	if err != nil {
		return false, time.Now(), time.Now().Add(time.Second), err
	}
	started = build.StartedAt.Time
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
				fmt.Fprintf(os.Stderr, "Timeout reached waiting for the build to finish\n")
				ended = time.Now()
				return
			default:
			}

			anyRunning, anyFailed, err := bkCheckJobs(client, cfg)
			if !anyRunning {
				// if this is the first time we think we're finished store the timestamp
				if checksLeft >= numChecks {
					ended = time.Now()
				}

				// ok, carry on
				checksLeft--
				if checksLeft <= 0 {
					// we're done checking.
					passed = !anyFailed
					if passed {
						fmt.Println("Build passed!")
					} else {
						fmt.Println("Build failed!")
					}
					return
				}
				if err != nil {
					// we previously successfully queried for the build; this is likely a
					// transient error
					fmt.Printf("Querying the Buildkite API failed with %s; trying %d more times before giving up.\n", err.Error(), checksLeft)
					continue
				}
				if anyFailed {
					// don't bother rechecking if a job has failed
					fmt.Printf("Build failed!\n")
					ended = time.Now()
					return
				}

				// yay looks like maybe we're done?
				fmt.Printf("Build appears finished; checking %d more times to make sure.\n", checksLeft)
				continue
			}
			// if we previously thought we were finished but now realize we weren't,
			// reset the check counter so we try again next time we think we're
			// finished.
			passed = false
			checksLeft = numChecks
		}
	}()

	<-done
	return passed, started, ended, nil
}

// bkCheckJobs looks at the Builtkite API for the list of jobs in this build
// and decides whether the build has finished and if finished, whether it
// failed. If an error is returned, it represents an error talking to the
// Buildkite API, not an error with the build
func bkCheckJobs(client *buildkite.Client, cfg bkWatchConfig) (anyRunning bool, anyFailed bool, err error) {
	fmt.Printf("%s: polling for jobs: ", time.Now().Format(time.StampMilli))
	build, err := bkGetBuild(client, cfg)
	if err != nil {
		fmt.Printf("error polling: %s\n", err.Error())
		return true, true, err
	}
	fmt.Println(bkSummarizeJobs(build.Jobs))

	anyRunning = false
	for _, job := range build.Jobs {
		// skip ourself so we don't wait if we're the only job running
		if *job.ID == cfg.jobID {
			continue
		}

		// see https://buildkite.com/docs/pipelines/defining-steps#job-states
		// for a graph of all the states a job can be in.

		if job.State == nil {
			// not sure how this can happen..  maybe a BK wait step in the pipeline?
			b, _ := json.Marshal(job)
			fmt.Printf("nil job state for %s\n", string(b))
			continue
		}

		switch *job.State {
		case "pending":
			// pending means a job will either start or be skipped soon, so we
			// consider it running already.
			anyRunning = true
		case "waiting":
			// waiting looks to be a wait job in the pipeline.  we'll consider this
			// job running even though it's not _really_ a job.
			anyRunning = true
		case "waiting_failed":
			// The wait job failed?  I guess the only way this can happen is if
			// one of the jobs before it in the pipeline also failed?  we'll
			// consider this a failure.
			anyFailed = true
		case "blocked":
			anyRunning = true
		case "blocked_failed":
			anyFailed = true
		case "unblocked":
			anyRunning = true
		case "unblocked_failed":
			anyFailed = true
		case "scheduled":
			// the api also returns this state for limiting/limited jobs
			anyRunning = true
		case "assigned":
			anyRunning = true
		case "accepted":
			anyRunning = true
		case "running":
			anyRunning = true
		case "passed":
			continue
		case "failed":
			anyFailed = true
		case "canceling":
			// cancelation is intentional cancelling of the job, via the UI or the API.
			anyRunning = true
		case "canceled":
			// canceled means the job was canceled by a user.  don't count it.
		case "timing_out":
			anyRunning = true
		case "timed_out":
			anyFailed = true
		case "skipped":
			// skipped jobs are jobs that aren't run because a newer build was started
			// and "build skipping" is enabled.  Don't count them.
			continue
		case "broken":
			// broken jobs are jobs that aren't run because of the build
			// configuration keeps them from running.  Don't count them either.
			continue
		}
	}

	return anyRunning, anyFailed, nil
}

// bkGetBuild queries the Buildkite API for a build
func bkGetBuild(client *buildkite.Client, cfg bkWatchConfig) (*buildkite.Build, error) {
	build, _, err := client.Builds.Get(cfg.orgSlug, cfg.pipelineSlug, cfg.buildNumber, nil)
	if err != nil {
		return nil, err
	}
	return build, nil
}

// bkSummarizeJobs takes a list of jobs and returns a string summary
func bkSummarizeJobs(jobs []*buildkite.Job) string {
	if len(jobs) == 0 {
		return "no jobs found"
	}

	// look at all the jobs and count how many are in each state
	countByState := map[string]int{}
	for _, job := range jobs {
		if job.State == nil {
			countByState["nil-job-state"]++
			continue
		}
		countByState[*job.State]++
	}

	// sort the states present to print them in a consistent order
	sortedStateList := make([]string, 0, len(countByState))
	for key := range countByState {
		sortedStateList = append(sortedStateList, key)
	}
	sort.Strings(sortedStateList)

	// create a list of printable state counts
	stateStrings := make([]string, 0, len(countByState))
	for i := 0; i < len(countByState); i++ {
		state := sortedStateList[i]
		count := countByState[state]
		stateStrings = append(stateStrings, fmt.Sprintf("%d %s", count, state))
	}

	// join the list of printable states to make one nice line
	return strings.Join(stateStrings, ", ")
}
