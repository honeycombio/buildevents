package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kr/logfmt"
	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

// Version will be set by CircleCI based on a git tag and the commit hash
var Version = "dev"

const (
	providerTravis = "Travis-CI"
	providerCircle = "CircleCI"
	providerGitLab = "GitLab-CI"
)

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

func createEvent(cfg libhoney.Config, provider string, traceID string) *libhoney.Event {
	libhoney.UserAgentAddition = fmt.Sprintf("buildevents/%s", Version)
	if provider != "" {
		libhoney.UserAgentAddition += fmt.Sprintf(" (%s)", provider)
	}

	if cfg.APIKey == "" {
		cfg.Transmission = &transmission.WriterSender{}
	}
	libhoney.Init(cfg)

	ev := libhoney.NewEvent()
	if provider != "" {
		ev.AddField("ci_provider", provider)
	}
	ev.AddField("trace.trace_id", traceID)
	ev.AddField("meta.version", Version)

	return ev
}

// providerInfo adds a bunch of fields to every span with useful information
// about the build, gleaned from known providers
func providerInfo(provider string, ev *libhoney.Event) {
	// envVars is a map of environment variable to event field name
	var envVars map[string]string
	switch strings.ToLower(provider) {
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
	case "gitlab-ci", "gitlabci", "gitlab":
		envVars = map[string]string{
			"CI_COMMIT_REF_NAME":                   "branch",
			"CI_PIPELINE_ID":                       "build_num",
			"CI_PIPELINE_URL":                      "build_url",
			"CI_MERGE_REQUEST_ID":                  "pr_number",
			"CI_MERGE_REQUEST_SOURCE_BRANCH_NAME":  "pr_branch",
			"CI_MERGE_REQUEST_SOURCE_PROJECT_PATH": "pr_repo",
			"CI_PROJECT_URL":                       "repo",
		}
	}
	for envVar, fieldName := range envVars {
		if val, ok := os.LookupEnv(envVar); ok {
			ev.AddField(fieldName, val)
		}
	}
}

// arbitraryFields adds an arbitrary set of fields provided by the end user
func arbitraryFields(loc string, ev *libhoney.Event) {
	if loc == "" {
		return
	}

	data, err := ioutil.ReadFile(loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read %q: %v\n", loc, err)
		return
	}

	err = logfmt.Unmarshal(
		data,
		logfmt.HandlerFunc(func(key, val []byte) error {
			if f, err := strconv.ParseFloat(string(val), 64); err == nil {
				ev.AddField(string(key), f)
				return nil
			}
			if b, err := strconv.ParseBool(string(val)); err == nil {
				ev.AddField(string(key), b)
				return nil
			}
			ev.AddField(string(key), string(val))
			return nil
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "problems loading from %q: %v\n", loc, err)
	}
}

func parseSince(start string) (time.Time, time.Duration) {
	secondsSinceEpoch, _ := strconv.ParseInt(strings.TrimSpace(start), 10, 64)
	startUnix := time.Unix(secondsSinceEpoch, 0)
	if startUnix == time.Unix(0, 0) {
		fmt.Fprintf(os.Stderr, "couldn't parse startTime of %q\n", start)
		startUnix = time.Now()
	}
	return startUnix, time.Since(startUnix)
}

func buildURL(cfg libhoney.Config, traceID string, ts int64) (string, error) {
	teamName, err := libhoney.VerifyAPIKey(cfg)
	if err != nil {
		return "", fmt.Errorf("unable to verify API key")
	}
	uiHost := strings.Replace(cfg.APIHost, "api", "ui", 1)
	u, err := url.Parse(uiHost)
	if err != nil {
		return "", fmt.Errorf("unable to infer UI host: %s", uiHost)
	}
	u.Path = path.Join(teamName, "datasets", cfg.Dataset, "trace")
	endTime := time.Now().Add(10 * time.Minute).Unix()
	return fmt.Sprintf(
		"%s?trace_id=%s&trace_start_ts=%d&trace_end_ts=%d",
		u.String(), traceID, ts, endTime,
	), nil
}

func main() {
	defer libhoney.Close()
	config := libhoney.Config{}
	filename := ""
	ciProvider := ""

	root := &cobra.Command{
		Version: Version,
		Use:     "buildevents",
		Short:   "buildevents creates events for your CI builds",
		Long: `
The buildevents executable creates Honeycomb events and tracing information
about your Continuous Integration builds.`,
	}

	root.PersistentFlags().StringVarP(&config.APIKey, "apikey", "k", "", "[env.BUILDEVENT_APIKEY] the Honeycomb authentication token")
	if apikey, ok := os.LookupEnv("BUILDEVENT_APIKEY"); ok {
		// https://github.com/spf13/viper/issues/461#issuecomment-366831834
		root.PersistentFlags().Lookup("apikey").Value.Set(apikey)
	}

	root.PersistentFlags().StringVarP(&config.Dataset, "dataset", "d", "buildevents", "[env.BUILDEVENT_DATASET] the name of the Honeycomb dataset to which to send these events")
	if dataset, ok := os.LookupEnv("BUILDEVENT_DATASET"); ok {
		root.PersistentFlags().Lookup("dataset").Value.Set(dataset)
	}

	root.PersistentFlags().StringVarP(&config.APIHost, "apihost", "a", "https://api.honeycomb.io", "[env.BUILDEVENT_APIHOST] the hostname for the Honeycomb API server to which to send this event")
	if apihost, ok := os.LookupEnv("BUILDEVENT_APIHOST"); ok {
		root.PersistentFlags().Lookup("apihost").Value.Set(apihost)
	}

	root.PersistentFlags().StringVarP(&filename, "filename", "f", "", "[env.BUILDEVENT_FILE] the path of a text file holding arbitrary key=val pairs (multi-line-capable, logfmt style) to be added to the Honeycomb event")
	if fname, ok := os.LookupEnv("BUILDEVENT_FILE"); ok {
		root.PersistentFlags().Lookup("filename").Value.Set(fname)
	}

	root.PersistentFlags().StringVarP(&ciProvider, "provider", "p", "", "[env.BUILDEVENT_CIPROVIDER] if unset, will inspect the environment to try and detect Travis-CI, CircleCI, or GitLab-CI.")
	prov := os.Getenv("BUILDEVENT_CIPROVIDER")
	if prov == "" {
		if _, present := os.LookupEnv("TRAVIS"); present {
			prov = providerTravis
		} else if _, present := os.LookupEnv("CIRCLECI"); present {
			prov = providerCircle
		} else if _, present := os.LookupEnv("GITLAB_CI"); present {
			prov = providerGitLab
		}
	}
	if prov != "" {
		root.PersistentFlags().Lookup("provider").Value.Set(prov)
	}

	// BUILD - eg: buildevents build $TRAVIS_BUILD_ID $BUILD_START success
	buildCmd := &cobra.Command{
		Use:   "build [flags] BUILD_ID BUILD_START OUTCOME",
		Short: "Sends the root span for the entire build",
		Long: `
The build mode sends the root span for the entire build. It should be called
when the build finishes and records the duration of the entire build. It emits
a URL pointing to the generated trace in Honeycomb to STDOUT.`,
		Args:                  argOptions(2, "success", "failure"),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			ev := createEvent(config, ciProvider, args[0])
			defer ev.Send()

			providerInfo(ciProvider, ev)
			arbitraryFields(filename, ev)

			ts, dur := parseSince(args[1])
			ev.Add(map[string]interface{}{
				"service_name":  "build",
				"trace.span_id": args[0],
				"name":          "build " + args[0],
				"status":        args[2],
				"duration_ms":   dur / time.Millisecond,
			})
			ev.Timestamp = ts

			url, err := buildURL(config, args[0], ts.Unix())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to create trace URL: %v\n", err)
			} else {
				fmt.Println(url)
			}
		},
	}

	// STEP - eg: buildevents step $TRAVIS_BUILD_ID $STAGE_SPAN_ID $STAGE_START script
	stepCmd := &cobra.Command{
		Use:   "step [flags] BUILD_ID STEP_ID START_TIME NAME",
		Short: "Joins a collection of individual commands",
		Long: `
The step mode represents a block of related commands. In Travis-CI, this is
one of "install", "before_script", "script", and so on. In CircleCI, this
most closely maps to a single job. It should be run at the end of the step.`,
		Args:                  cobra.ExactArgs(4),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			ev := createEvent(config, ciProvider, args[0])
			defer ev.Send()

			providerInfo(ciProvider, ev)
			arbitraryFields(filename, ev)

			ts, dur := parseSince(args[2])
			ev.Add(map[string]interface{}{
				"trace.parent_id": args[0],
				"trace.span_id":   args[1],
				"service_name":    "step",
				"name":            args[3],
				"duration_ms":     dur / time.Millisecond,
			})
			ev.Timestamp = ts
		},
	}

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
		Args:                  cobra.MinimumNArgs(4),
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if cmd.ArgsLenAtDash() != 3 {
				fmt.Fprintf(os.Stderr, "use `--` to signify shell command\n")
				return
			}
			// fmt.Printf("DASH ARGS: %#v ; DASH: %d\n", args, cmd.ArgsLenAtDash())
			ev := createEvent(config, ciProvider, args[0])
			defer ev.Send()

			providerInfo(ciProvider, ev)
			arbitraryFields(filename, ev)

			spanBytes := make([]byte, 16)
			rand.Read(spanBytes)

			var quoted []string
			for _, s := range args[3:] {
				quoted = append(quoted, fmt.Sprintf("\"%s\"", strings.Replace(s, "\"", "\\\"", -1)))
			}
			subcmd := strings.Join(quoted, " ")

			start := time.Now()
			err := runCommand(subcmd)
			dur := time.Since(start)

			ev.Add(map[string]interface{}{
				"trace.parent_id": args[1],
				"trace.span_id":   fmt.Sprintf("%x", spanBytes),
				"service_name":    "cmd",
				"name":            args[2],
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

	// WATCH eg: buildevents watch $TRAVIS_BUILD_ID
	var wcfg watchConfig
	watchCmd := &cobra.Command{
		Use:   "watch BUILD_ID",
		Short: "Polls the CircleCI API and waits until all jobs have finished.",
		Long: `
Polls the CircleCI API and waits until all jobs have finished (either
succeeded, failed, or are blocked). It then reports the final status of the
build with the appropriate timers.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if ciProvider != providerCircle {
				fmt.Fprintf(os.Stderr, "watch command only valid for %s\n", providerCircle)
				return
			}

			ev := createEvent(config, ciProvider, args[0])
			defer ev.Send()

			providerInfo(ciProvider, ev)

			// TODO: consider - move this until after the watch timeout??
			arbitraryFields(filename, ev)

			failed, start, err := waitCircle(context.Background(), wcfg)
			if err != nil {
				fmt.Printf("buildevents - Error detected: %s\n", err.Error())
				os.Exit(1)
			}

			dur := time.Since(start)
			status := "success"
			if failed {
				status = "failed"
			}

			ev.Add(map[string]interface{}{
				"service_name":  "watch",
				"trace.span_id": args[0],
				"name":          "watch " + args[0],
				"status":        status,
				"duration_ms":   dur / time.Millisecond,
			})
			ev.Timestamp = start

			url, err := buildURL(config, args[0], start.Unix())
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

	// Put 'em all together, and do the work
	root.AddCommand(buildCmd, stepCmd, execCmd, watchCmd)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// composer allows combining several PositionalArgs to work in concert.
func composer(pargs ...cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		for _, parg := range pargs {
			if err := parg(cmd, args); err != nil {
				return err
			}
		}
		return nil
	}
}

// composer allows combining several PositionalArgs to work in concert.
func argOptions(pos int, opts ...string) cobra.PositionalArgs {
	return composer(
		cobra.MinimumNArgs(pos+1),
		func(cmd *cobra.Command, args []string) error {
			for _, opt := range opts {
				if args[pos] == opt {
					return nil
				}
			}
			if len(opts) == 1 {
				return fmt.Errorf("argument at index %d (%q) must be %q", pos, args[pos], opts[0])
			}
			return fmt.Errorf("argument at index %d (%q) must be one of %v", pos, args[pos], opts)
		},
	)
}
