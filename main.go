package main

import (
	"errors"
	"os"

	"os/exec"

	libhoney "github.com/honeycombio/libhoney-go"
)

// Version will be set by CircleCI based on a git tag and the commit hash
var Version = "dev"

const (
	providerTravis             = "Travis-CI"
	providerCircle             = "CircleCI"
	providerGitLab             = "GitLab-CI"
	providerBuildkite          = "Buildkite"
	providerJenkinsX           = "Jenkins-X"
	providerGoogleCloudBuild   = "Google-Cloud-Build"
	providerAzurePipelines     = "Azure-Pipelines"
	providerGitHubActions      = "GitHub-Actions"
	providerBitbucketPipelines = "Bitbucket-Pipelines"
)

func main() {
	defer libhoney.Close()
	var config libhoney.Config
	var filename string
	var ciProvider string
	var wcfg watchConfig
	var serviceName string

	root := commandRoot(&config, &filename, &ciProvider, &serviceName)

	// Put 'em all together
	root.AddCommand(
		commandBuild(&config, &filename, &ciProvider),
		commandStep(&config, &filename, &ciProvider),
		commandCmd(&config, &filename, &ciProvider),
		commandWatch(&config, &filename, &ciProvider, &wcfg),
	)

	// Do the work
	if err := root.Execute(); err != nil {
		libhoney.Close()

		// If the underlying command returned a specific exit code, we need
		// to exit it with it as well to act transparently.
		var cmdErr *exec.ExitError
		if errors.As(err, &cmdErr) {
			os.Exit(cmdErr.ExitCode())
		}
		os.Exit(1)
	}
}
