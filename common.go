package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kr/logfmt"

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

	ev.AddField("meta.os", runtime.GOOS)
	ev.AddField("meta.arch", runtime.GOARCH)

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

	case "github-actions", "githubactions", "github", "gha-buildevents":
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

// slugify turns a name into a slug. It is idempotent to things that are already slugs.
func slugify(name string) string {
	slugReplaceRegex := regexp.MustCompile(`[^a-z0-9_~\.-]`)
	return slugReplaceRegex.ReplaceAllString(strings.ToLower(name), "-")
}

func buildURL(cfg *libhoney.Config, traceID string, ts int64) (string, error) {
	team, environment, err := libhoney.GetTeamAndEnvironment(*cfg)
	if err != nil {
		return "", fmt.Errorf("unable to verify API key: %w", err)
	}
	uiHost := strings.Replace(cfg.APIHost, "api", "ui", 1)
	u, err := url.Parse(uiHost)
	if err != nil {
		return "", fmt.Errorf("unable to infer UI host: %s", uiHost)
	}
	pathSegments := []string{team}
	if !isClassic(cfg.APIKey) {
		pathSegments = append(pathSegments, "environments", environment)
	}
	pathSegments = append(pathSegments, "datasets", slugify(cfg.Dataset), "trace")
	u.Path = path.Join(pathSegments...)
	endTime := time.Now().Add(10 * time.Minute).Unix()

	v := url.Values{}
	v.Set("trace_id", traceID)
	v.Set("trace_start_ts", strconv.FormatInt(ts, 10))
	v.Set("trace_end_ts", strconv.FormatInt(endTime, 10))
	u.RawQuery = v.Encode()

	return u.String(), nil
}

// classic keys are always 32 bytes, non-classic are less than that
// classic keys are also hex but that doesn't matter here
func isClassic(key string) bool {
	return len(key) == 32
}

func ifClassic(key, classicVal, elseVal string) string {
	if isClassic(key) {
		return classicVal
	}
	return elseVal
}
