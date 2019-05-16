package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	libhoney "github.com/honeycombio/libhoney-go"
)

// buildevents expects to get some unchanging values from the environment and
// the rest as positional arguments on the command line.
//
// see README.md for detailed usage information

func sendTraceRoot(name, traceID, buildStatus string, timestamp time.Time, duration time.Duration) {
	ev := libhoney.NewEvent()
	ev.Add(map[string]interface{}{
		"service_name":  "build",
		"trace.span_id": traceID,
		"name":          name,
		"status":        buildStatus,
		"duration_ms":   duration / time.Millisecond,
	})
	ev.Timestamp = timestamp
	ev.Send()
}

func getTraceSpanEvent(parentSpanID, spanID, serviceName, name string, timestamp time.Time, duration time.Duration) *libhoney.Event {
	ev := libhoney.NewEvent()
	ev.Add(map[string]interface{}{
		"trace.parent_id": parentSpanID,
		"trace.span_id":   spanID,
		"service_name":    serviceName,
		"name":            name,
		"duration_ms":     duration / time.Millisecond,
	})
	ev.Timestamp = timestamp

	return ev
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

func handleBuild(traceID string) {
	// command line eg: buildevents build $TRAVIS_BUILD_ID $BUILD_START success

	name := "build " + traceID
	startTime := os.Args[3]
	buildStatus := os.Args[4]

	secondsSinceEpoch, _ := strconv.ParseInt(startTime, 10, 64)

	startUnix := time.Unix(secondsSinceEpoch, 0)
	sendTraceRoot(name, traceID, buildStatus, startUnix, time.Since(startUnix))
}

func handleStep() error {
	// command line eg: buildevents step $TRAVIS_BUILD_ID $STAGE_SPAN_ID $STAGE_START script

	parentSpanID := os.Args[2]
	stepSpanID := os.Args[3]
	startTime := os.Args[4]
	name := os.Args[5]

	secondsSinceEpoch, _ := strconv.ParseInt(startTime, 10, 64)

	startUnix := time.Unix(secondsSinceEpoch, 0)

	ev := getTraceSpanEvent(parentSpanID, stepSpanID, "step", name, startUnix, time.Since(startUnix))
	return ev.Send()
}

func handleCmd() error {
	// command line eg: buildevents cmd $TRAVIS_BUILD_ID $STAGE_SPAN_ID go-test -- go test github.com/honeycombio/hound/...

	// TODO include in readme warning about really needing positional argumenst to be correct
	parentSpanID := os.Args[3]
	name := os.Args[4]
	// arg[5] is the "--"

	spanBytes := make([]byte, 16)
	rand.Read(spanBytes)
	spanID := base64.StdEncoding.EncodeToString(spanBytes)

	var quoted []string
	for _, s := range os.Args[6:] {
		quoted = append(quoted, fmt.Sprintf("\"%s\"", strings.Replace(s, "\"", "\\\"", -1)))
	}
	subcmd := strings.Join(quoted, " ")

	before := time.Now()
	err := runCommand(subcmd)

	ev := getTraceSpanEvent(parentSpanID, spanID, "cmd", name, before, time.Since(before))
	ev.AddField("cmd", subcmd)
	if err != nil {
		ev.Add(map[string]interface{}{
			"cmd":            subcmd,
			"status":         "failure",
			"failure_reason": err.Error(),
		})
	} else {
		ev.Add(map[string]interface{}{
			"cmd":    subcmd,
			"status": "success",
		})
	}
	ev.Send()
	return err
}

func main() {
	apikey, _ := os.LookupEnv("BUILDEVENT_APIKEY")
	dataset, _ := os.LookupEnv("BUILDEVENT_DATASET")
	apihost, _ := os.LookupEnv("BUILDEVENT_APIHOST")
	buildurl, _ := os.LookupEnv("BUILDEVENT_URL")
	ciProvider, _ := os.LookupEnv("BUILDEVENT_CIPROVIDER")
	if ciProvider == "" {
		if _, present := os.LookupEnv("TRAVIS_BUILD_NUMBER"); present {
			ciProvider = "Travis-CI"
		} else if _, present := os.LookupEnv("CIRCLE_WORKFLOW_ID"); present {
			ciProvider = "CircleCI"
		}
	}

	// use defaults for dataset and apihost if they're unset.
	if dataset == "" {
		dataset = "buildevents"
	}
	if apihost == "" {
		apihost = "https://api.honeycomb.io"
	}

	libhoney.Init(libhoney.Config{
		WriteKey: apikey,
		Dataset:  dataset,
		APIHost:  apihost,
	})

	spanType := os.Args[1]
	traceID := os.Args[2]

	// add the build URL to all spans
	if buildurl != "" {
		libhoney.AddField("build_url", buildurl+traceID)
	}
	if ciProvider != "" {
		libhoney.AddField("ci_provider", ciProvider)
	}
	libhoney.AddField("trace.trace_id", traceID)

	responses := libhoney.Responses()
	var err error
	if spanType == "cmd" {
		err = handleCmd()
	} else if spanType == "step" {
		// there can be no error here
		handleStep()
	} else {
		// there can be no error here
		handleBuild(traceID)
	}

	// actually wait for the response
	_ = <-responses

	// if the command we ran exitted with an error, let's exit with the same error
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
			os.Exit(1)
		}
		os.Exit(1)
	}
}
