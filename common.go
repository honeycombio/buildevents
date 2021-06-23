package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kr/logfmt"
	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

func createEvent(cfg *libhoney.Config, provider string, traceID string) *libhoney.Event {
	libhoney.UserAgentAddition = fmt.Sprintf("buildevents/%s", Version)
	if provider != "" {
		libhoney.UserAgentAddition += fmt.Sprintf(" (%s)", provider)
	}

	if cfg.APIKey == "" {
		cfg.Transmission = &transmission.WriterSender{}
	}
	libhoney.Init(*cfg)

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

	case "buildkite", "buildkiteci", "build-kite":
		envVars = map[string]string{
			"BUILDKITE_BRANCH":            "branch",
			"BUILDKITE_BUILD_NUMBER":      "build_num",
			"BUILDKITE_BUILD_URL":         "build_url",
			"BUILDKITE_PULL_REQUEST":      "pr_number",
			"BUILDKITE_PULL_REQUEST_REPO": "pr_repo",
			"BUILDKITE_REPO":              "repo",
		}
	case "jenkinsx", "jenkins-x":
		envVars = map[string]string{
			"BRANCH_NAME":  "branch",
			"BUILD_NUMBER": "build_num",
			"PULL_NUMBER":  "pr_number",
			"REPO_NAME":    "repo",
		}

	case "google-cloud-build", "cloud-build", "gcb":
		envVars = map[string]string{
			"BRANCH_NAME": "branch",
			"BUILD_ID":    "build_num",
			"HEAD_BRANCH": "pr_branch",
			"REPO_OWNER":  "pr_user",
			"REPO_NAME":   "repo",
		}

	case "azure-pipelines", "azure-devops", "vsts", "tfs":
		envVars = map[string]string{
			"BUILD_SOURCEBRANCHNAME":               "branch",
			"BUILD_BUILDID":                        "build_id",
			"BUILD_BUILDNUMBER":                    "build_number",
			"SYSTEM_JOBDISPLAYNAME":                "job_name",
			"SYSTEM_STAGEDISPLAYNAME":              "stage_name",
			"SYSTEM_PULLREQUEST_PULLREQUESTID":     "pr_id",
			"SYSTEM_PULLREQUEST_PULLREQUESTNUMBER": "pr_number",
			"SYSTEM_PULLREQUEST_SOURCEBRANCH":      "pr_branch",
			"BUILD_REQUESTEDFOR":                   "build_user",
			"BUILD_REPOSITORY_URI":                 "repo",
		}

	case "github-actions", "githubactions", "github":
		envVars = map[string]string{
			"GITHUB_REF":        "branch",
			"GITHUB_RUN_ID":     "build_num",
			"GITHUB_WORKFLOW":   "workflow_name",
			"GITHUB_HEAD_REF":   "pr_branch",
			"GITHUB_ACTOR":      "pr_user",
			"GITHUB_REPOSITORY": "repo",
		}

	case "bitbucket-pipelines", "bitbucketpipelines", "bitbucket":
		envVars = map[string]string{
			"BITBUCKET_BRANCH":              "branch",
			"BITBUCKET_PIPELINE_UUID":       "pipeline_id",
			"BITBUCKET_BUILD_NUMBER":        "build_num",
			"BITBUCKET_REPO_FULL_NAME":      "repo",
			"BITBUCKET_PR_ID":               "pr_id",
			"BITBUCKET_STEP_TRIGGERER_UUID": "build_user",
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

// parseUnix reads the input text as a Unix timestamp (to the second)
func parseUnix(ts string) time.Time {
	secondsSinceEpoch, _ := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	unix := time.Unix(secondsSinceEpoch, 0)
	if unix == time.Unix(0, 0) {
		fmt.Fprintf(os.Stderr, "couldn't parse startTime of %q\n", ts)
		unix = time.Now()
	}
	return unix
}

func buildURL(cfg *libhoney.Config, traceID string, ts int64) (string, error) {
	teamName, err := libhoney.VerifyAPIKey(*cfg)
	if err != nil {
		return "", fmt.Errorf("unable to verify API key")
	}
	uiHost := strings.Replace(cfg.APIHost, "api", "ui", 1)
	u, err := url.Parse(uiHost)
	if err != nil {
		return "", fmt.Errorf("unable to infer UI host: %s", uiHost)
	}
	u.Path = path.Join(teamName, "datasets", strings.Replace(cfg.Dataset, " ", "-", -1), "trace")
	endTime := time.Now().Add(10 * time.Minute).Unix()
	return fmt.Sprintf(
		"%s?trace_id=%s&trace_start_ts=%d&trace_end_ts=%d",
		u.String(), traceID, ts, endTime,
	), nil
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
