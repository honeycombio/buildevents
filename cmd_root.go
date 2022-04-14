package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

func commandRoot(cfg *libhoney.Config, filename *string, ciProvider *string, serviceName *string) *cobra.Command {
	root := &cobra.Command{
		Version: Version,
		Use:     "buildevents",
		Short:   "buildevents creates events for your CI builds",
		Long: `
The buildevents executable creates Honeycomb events and tracing information
about your Continuous Integration builds.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			quiet, _ := cmd.Flags().GetBool("quiet")
			if isClassic(cfg.APIKey) {
				// if we're in classic mode, we want to behave the same as we always have
				if cfg.Dataset == "" {
					cfg.Dataset = "buildevents"
				}
				if *serviceName != "" && !quiet {
					fmt.Fprintf(os.Stderr, "WARN: classic mode ignores the service name parameter.\n")
				}
			} else {
				// This is the non-classic behavior
				if *serviceName != "" {
					// service name was specified, so use it as the dataset
					if cfg.Dataset != "" && !quiet {
						// warn if we're going to ignore a specified dataset
						fmt.Fprintf(os.Stderr, "WARN: service name was specified, dataset is ignored.\n")
					}
					trimmed := strings.TrimSpace(*serviceName)
					if trimmed != *serviceName && !quiet {
						fmt.Fprintf(os.Stderr, "WARN: service name contained leading or trailing whitespace, sending to '%s'.\n", trimmed)
					}
					cfg.Dataset = trimmed
				} else {
					// service_name was not specified
					if cfg.Dataset != "" {
						// dataset was, so use it but warn
						if !quiet {
							fmt.Fprintf(os.Stderr, "WARN: dataset is deprecated, please use service_name.\n")
						}
					} else {
						// neither was specified, so just use the default
						cfg.Dataset = "buildevents"
					}
				}
			}
		},
	}

	root.PersistentFlags().StringVarP(&cfg.APIKey, "apikey", "k", "", "[env.BUILDEVENT_APIKEY] the Honeycomb authentication token")
	if apikey, ok := os.LookupEnv("BUILDEVENT_APIKEY"); ok {
		// https://github.com/spf13/viper/issues/461#issuecomment-366831834
		root.PersistentFlags().Lookup("apikey").Value.Set(apikey)
	}

	root.PersistentFlags().StringVarP(&cfg.Dataset, "dataset", "d", "buildevents", "[env.BUILDEVENT_DATASET] the name of the Honeycomb dataset to which to send these events")
	if dataset, ok := os.LookupEnv("BUILDEVENT_DATASET"); ok {
		root.PersistentFlags().Lookup("dataset").Value.Set(dataset)
	}

	root.PersistentFlags().StringVarP(serviceName, "service_name", "n", "", "[env.BUILDEVENT_SERVICE_NAME] the name of the service to which to send these events; overrides dataset")
	if service_name, ok := os.LookupEnv("BUILDEVENT_SERVICE_NAME"); ok {
		root.PersistentFlags().Lookup("service_name").Value.Set(service_name)
	}

	root.PersistentFlags().StringVarP(&cfg.APIHost, "apihost", "a", "https://api.honeycomb.io", "[env.BUILDEVENT_APIHOST] the hostname for the Honeycomb API server to which to send this event")
	if apihost, ok := os.LookupEnv("BUILDEVENT_APIHOST"); ok {
		root.PersistentFlags().Lookup("apihost").Value.Set(apihost)
	}

	root.PersistentFlags().StringVarP(filename, "filename", "f", "", "[env.BUILDEVENT_FILE] the path of a text file holding arbitrary key=val pairs (multi-line-capable, logfmt style) to be added to the Honeycomb event")
	if fname, ok := os.LookupEnv("BUILDEVENT_FILE"); ok {
		root.PersistentFlags().Lookup("filename").Value.Set(fname)
	}

	root.PersistentFlags().StringVarP(ciProvider, "provider", "p", "", "[env.BUILDEVENT_CIPROVIDER] if unset, will inspect the environment to try to detect common CI providers.")
	prov := os.Getenv("BUILDEVENT_CIPROVIDER")
	if prov == "" {
		if _, present := os.LookupEnv("TRAVIS"); present {
			prov = providerTravis
		} else if _, present := os.LookupEnv("CIRCLECI"); present {
			prov = providerCircle
		} else if _, present := os.LookupEnv("GITLAB_CI"); present {
			prov = providerGitLab
		} else if _, present := os.LookupEnv("BUILDKITE"); present {
			prov = providerBuildkite
		} else if _, present := os.LookupEnv("JENKINS-X"); present {
			prov = providerJenkinsX
		} else if _, present := os.LookupEnv("GOOGLE-CLOUD-BUILD"); present {
			prov = providerGoogleCloudBuild
		} else if _, present := os.LookupEnv("TF_BUILD"); present {
			prov = providerAzurePipelines
		} else if _, present := os.LookupEnv("GITHUB_ACTIONS"); present {
			prov = providerGitHubActions
		} else if _, present := os.LookupEnv("BITBUCKET_BUILD_NUMBER"); present {
			prov = providerBitbucketPipelines
		}
	}
	if prov != "" {
		root.PersistentFlags().Lookup("provider").Value.Set(prov)
	}

	return root
}
