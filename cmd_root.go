package main

import (
	"os"

	"github.com/spf13/cobra"

	libhoney "github.com/honeycombio/libhoney-go"
)

func commandRoot(cfg *libhoney.Config, filename *string, ciProvider *string) *cobra.Command {
	root := &cobra.Command{
		Version: Version,
		Use:     "buildevents",
		Short:   "buildevents creates events for your CI builds",
		Long: `
The buildevents executable creates Honeycomb events and tracing information
about your Continuous Integration builds.`,
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

	root.PersistentFlags().StringVarP(&cfg.APIHost, "apihost", "a", "https://api.honeycomb.io", "[env.BUILDEVENT_APIHOST] the hostname for the Honeycomb API server to which to send this event")
	if apihost, ok := os.LookupEnv("BUILDEVENT_APIHOST"); ok {
		root.PersistentFlags().Lookup("apihost").Value.Set(apihost)
	}

	root.PersistentFlags().StringVarP(filename, "filename", "f", "", "[env.BUILDEVENT_FILE] the path of a text file holding arbitrary key=val pairs (multi-line-capable, logfmt style) to be added to the Honeycomb event")
	if fname, ok := os.LookupEnv("BUILDEVENT_FILE"); ok {
		root.PersistentFlags().Lookup("filename").Value.Set(fname)
	}

	root.PersistentFlags().StringVarP(ciProvider, "provider", "p", "", "[env.BUILDEVENT_CIPROVIDER] if unset, will inspect the environment to try and detect Travis-CI, CircleCI, or GitLab-CI.")
	prov := os.Getenv("BUILDEVENT_CIPROVIDER")
	if prov == "" {
		if _, present := os.LookupEnv("TRAVIS"); present {
			prov = providerTravis
		} else if _, present := os.LookupEnv("CIRCLECI"); present {
			prov = providerCircle
		} else if _, present := os.LookupEnv("GITLAB_CI"); present {
			prov = providerGitLab
		} else if _, present := os.LookupEnv("JENKINS-X"); present {
			prov = providerJenkinsX
		}
	}
	if prov != "" {
		root.PersistentFlags().Lookup("provider").Value.Set(prov)
	}

	return root
}
