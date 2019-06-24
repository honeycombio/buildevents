package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	circleci "github.com/jszwedko/go-circleci"
)

// pollCircleAPI will continue to poll the Circle API for this workflow's status
// until it succeeds or fails. It waits until this is the only job left running
// then declares the workflow finished and records success or failure same as
// the `handleBuild` function
func pollCircleAPI(traceID, teamName, apiHost, dataset string, timeoutMin int) error {
	workflowID, _ := os.LookupEnv("CIRCLE_WORKFLOW_ID")
	thisJobName, _ := os.LookupEnv("CIRCLE_JOB")
	client := &circleci.Client{}

	wfJobs, err := getJobs(client, workflowID)
	if err != nil {
		return err
	}

	// default timeout is 10 minutes
	if timeoutMin <= 0 {
		timeoutMin = 10
	}
	timeoutDur := time.Duration(timeoutMin) * time.Minute

	// ok, we have our jobs. Find ourself, watch the rest, see if any fail, wait
	// till they're all done
	done := make(chan struct{})
	tic := time.NewTicker(5 * time.Second)
	var failed bool
	go func() {
		defer func() { done <- struct{}{} }()
		for t := range tic.C {
			fmt.Println("polling for finished jobs: ", t.Format(time.StampMilli))
			wfJobs, err = getJobs(client, workflowID)
			if err != nil {
				return
			}

			groupedJobs := map[string][]string{}

			numJobs := len(wfJobs)
			var numFinished int
			for _, job := range wfJobs {
				// always count ourself as finished so we don't wait for ourself
				if job.Name == thisJobName {
					numFinished++
					continue
				}
				// group jobs by status
				group, _ := groupedJobs[job.Status]
				group = append(group, job.Name)
				groupedJobs[job.Status] = group

				switch job.Status {
				case "success":
					numFinished++
					continue
				case "failed":
					numFinished++
					failed = true
					continue
				case "blocked":
					numFinished++
					continue
				case "running":
					// keep waiting
					continue
				}
			}
			fmt.Printf("    found jobs: ")
			short := []string{}
			for status, names := range groupedJobs {
				short = append(short, fmt.Sprintf("%d %s", len(names), status))
			}
			fmt.Printf("%s\n", strings.Join(short, ","))

			if numFinished == numJobs {
				// if all jobs in the run have gotten to one of our finished states, then
				// the whole workflow is finished
				return
			}
		}
	}()

	// wait until everything's finished or we timeout
	select {
	case <-done:
		// yay we're done
	case <-time.After(timeoutDur):
		// uh oh we timed out
		// TODO add reason = timeout or something.
		failed = true
	}

	// if one of our jobs has failed, mark the build as failed. Otherwise success!
	wf, err := client.GetWorkflowV2(workflowID)
	startTime := wf.CreatedAt
	buildStatus := "success"
	if failed {
		buildStatus = "failed"
	}
	name := "build " + traceID
	sendTraceRoot(name, traceID, buildStatus, startTime, time.Since(startTime))
	printTraceURL(traceID, teamName, apiHost, dataset, startTime.Unix())

	return nil
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
