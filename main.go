package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

// Version will be set by CircleCI based on a git tag and the commit hash
var Version string

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

func handleBuild(traceID, teamName, apiHost, dataset string) {
	// command line eg: buildevents build $TRAVIS_BUILD_ID $BUILD_START success

	name := "build " + traceID
	startTime := strings.TrimSpace(os.Args[3])
	buildStatus := strings.TrimSpace(os.Args[4])

	secondsSinceEpoch, _ := strconv.ParseInt(startTime, 10, 64)

	startUnix := time.Unix(secondsSinceEpoch, 0)
	sendTraceRoot(name, traceID, buildStatus, startUnix, time.Since(startUnix))

	// spit out the URL to the trace
	if teamName == "" {
		// no team name means the API key didn't resolve, so we have no trace
		fmt.Println("skipping printing out the trace because the Honeycomb API key did not resolve to a team name")
		return
	}
	printTraceURL(traceID, teamName, apiHost, dataset, startUnix.Unix())

}

func printTraceURL(traceID, teamName, apiHost, dataset string, startUnix int64) {
	// spit out the URL to the trace
	if teamName == "" {
		// no team name means the API key didn't resolve, so we have no trace
		return
	}
	uiHost := strings.Replace(apiHost, "api", "ui", 1)
	u, err := url.Parse(uiHost)
	if err != nil {
		return
	}
	u.Path = path.Join(teamName, "datasets", dataset, "trace")
	endTime := time.Now().Add(10 * time.Minute).Unix()
	traceURL := fmt.Sprintf("%s?trace_id=%s&trace_start_ts=%d&trace_end_ts=%d",
		u.String(), traceID, startUnix, endTime)
	fmt.Println(traceURL)
}

func handleStep() error {
	// command line eg: buildevents step $TRAVIS_BUILD_ID $STAGE_SPAN_ID $STAGE_START script

	parentSpanID := strings.TrimSpace(os.Args[2])
	stepSpanID := strings.TrimSpace(os.Args[3])
	startTime := strings.TrimSpace(os.Args[4])
	name := strings.TrimSpace(os.Args[5])

	secondsSinceEpoch, _ := strconv.ParseInt(strings.TrimSpace(startTime), 10, 64)

	startUnix := time.Unix(secondsSinceEpoch, 0)

	if startUnix == time.Unix(0, 0) {
		fmt.Printf("couldn't parse startTime of %s\n", startTime)
		startUnix = time.Now()
	}

	ev := getTraceSpanEvent(parentSpanID, stepSpanID, "step", name, startUnix, time.Since(startUnix))
	return ev.Send()
}

func handleCmd() error {
	// command line eg: buildevents cmd $TRAVIS_BUILD_ID $STAGE_SPAN_ID go-test -- go test github.com/honeycombio/hound/...

	// TODO include in readme warning about really needing positional argumenst to be correct
	parentSpanID := strings.TrimSpace(os.Args[3])
	name := strings.TrimSpace(os.Args[4])
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

// addEnvVars adds a bunch of fields to every span with useful information
// about the build
func addEnvVars(ciProvider string) {
	// envVars is a map of environment variable to event field name
	var envVars map[string]string
	switch strings.ToLower(ciProvider) {
	case "circleci", "circle-ci", "circle":
		envVars = map[string]string{
			"CIRCLE_BRANCH":         "branch",
			"CIRCLE_BUILD_NUM":      "build_num",
			"CIRCLE_BUILD_URL":      "build_url", // overwrites buildevent_url+traceID
			"CIRCLE_JOB":            "job_name",
			"CIRCLE_PR_NUMBER":      "pr_number",
			"CIRCLE_PR_REPONAME":    "pr_repo",
			"CIRCLE_PR_USER":        "pr_user",
			"CIRCLE_REPOSITORY_URL": "repo",
		}
	case "travis-ci", "travisci", "travis":
		envVars = map[string]string{
			"TRAVIS_BRANCH":              "branch",
			"TRAVIS_BUILD_NUMBER":        "build_num",
			"TRAVIS_BUILD_WEB_URL":       "build_url",
			"TRAVIS_PULL_REQUEST":        "pr_number",
			"TRAVIS_PULL_REQUEST_BRANCH": "pr_branch",
			"TRAVIS_PULL_REQUEST_SLUG":   "pr_repo",
			"TRAVIS_REPO_SLUG":           "repo",
		}
	}
	for envVar, fieldName := range envVars {
		if val, ok := os.LookupEnv(envVar); ok {
			libhoney.AddField(fieldName, val)
		}
	}
}

func usage() {
	fmt.Printf(`Usage: buildevents [build,step,cmd] ... args

	For documentation, see https://github.com/honeycombio/buildevents

`)
}

func main() {
	apikey, _ := os.LookupEnv("BUILDEVENT_APIKEY")
	dataset, _ := os.LookupEnv("BUILDEVENT_DATASET")
	apihost, _ := os.LookupEnv("BUILDEVENT_APIHOST")
	// timeout is only for polling the circleci api
	timeoutStr, _ := os.LookupEnv("BUILDEVENT_TIMEOUT")
	ciProvider, _ := os.LookupEnv("BUILDEVENT_CIPROVIDER")
	if ciProvider == "" {
		if _, present := os.LookupEnv("TRAVIS"); present {
			ciProvider = "Travis-CI"
		} else if _, present := os.LookupEnv("CIRCLECI"); present {
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
	// use timeout default of 10min
	if timeoutStr == "" {
		timeoutStr = "10"
	}
	timeoutMin, err := strconv.Atoi(timeoutStr)
	if err != nil {
		timeoutMin = 10
	}

	if Version == "" {
		Version = "dev"
	}

	// respond to ./buildevents --version
	if strings.TrimSpace(os.Args[1]) == "--version" {
		fmt.Println(Version)
		os.Exit(0)
	}

	// every command needs at least two parameters - the command to run and the trace ID
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	// initialize libhoney
	libhoney.UserAgentAddition = fmt.Sprintf("buildevents/%s", Version)
	var teamName string
	if apikey != "" {
		teamName, _ = libhoney.VerifyAPIKey(libhoney.Config{
			APIHost: apihost,
			APIKey:  apikey,
		})
		libhoney.Init(libhoney.Config{
			WriteKey: apikey,
			Dataset:  dataset,
			APIHost:  apihost,
		})
	} else {
		// no API key set, initialize libhoney to drop all events
		libhoney.Init(libhoney.Config{
			Transmission: &transmission.DiscardSender{},
		})
	}
	libhoney.AddField("meta.version", Version)

	spanType := strings.TrimSpace(os.Args[1])
	traceID := strings.TrimSpace(os.Args[2])

	if ciProvider != "" {
		libhoney.AddField("ci_provider", ciProvider)
	}
	libhoney.AddField("trace.trace_id", traceID)

	addEnvVars(ciProvider)

	if spanType == "cmd" {
		err = handleCmd()
	} else if spanType == "step" {
		// there can be no error here
		handleStep()
	} else if spanType == "watch" {
		if ciProvider == "CircleCI" {
			err = pollCircleAPI(traceID, teamName, apihost, dataset, timeoutMin)
		} else {
			err = fmt.Errorf("watch command only valid on CircleCI")
		}
	} else {
		// there can be no error here
		handleBuild(traceID, teamName, apihost, dataset)
	}

	libhoney.Close()

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
