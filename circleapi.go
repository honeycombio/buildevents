package main

import (
	"context"
	"fmt"
	"time"

	circleci "github.com/jszwedko/go-circleci"
)

type watchConfig struct {
	timeoutMin int
	circleKey  string
	workflowID string
	jobName    string
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
