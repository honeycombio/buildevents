package main

import (
	"os"

	libhoney "github.com/honeycombio/libhoney-go"
)

// Version will be set by CircleCI based on a git tag and the commit hash
var Version = "dev"

const (
	providerTravis           = "Travis-CI"
	providerCircle           = "CircleCI"
	providerGitLab           = "GitLab-CI"
	providerJenkinsX         = "Jenkins-X"
	providerGoogleCloudBuild = "Google-Cloud-Build"
	providerAzurePipelines   = "Azure-Pipelines"
	providerGitHubActions    = "GitHub-Actions"
	providerBitbucketPipelines = "Bitbucket-Pipelines"
)

func main() {
	defer libhoney.Close()
	var config libhoney.Config
	var filename string
	var ciProvider string
	var wcfg watchConfig

	root := commandRoot(&config, &filename, &ciProvider)

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
		os.Exit(1)
	}
}
