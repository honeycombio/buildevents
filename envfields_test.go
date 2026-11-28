package main

import (
	"os"
	"testing"

	libhoney "github.com/honeycombio/libhoney-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvFields(t *testing.T) {
	testCases := []struct {
		name     string
		specs    []string
		expected []envFieldMapping
		badSpecs int
	}{
		{name: "none"},
		{
			name:     "single",
			specs:    []string{"CI_PIPELINE_ID=ci.pipeline.id"},
			expected: []envFieldMapping{{"CI_PIPELINE_ID", "ci.pipeline.id"}},
		},
		{
			name:     "whitespace is trimmed and blanks are skipped",
			specs:    []string{" CI_JOB_ID = ci.job.id ", "", "  "},
			expected: []envFieldMapping{{"CI_JOB_ID", "ci.job.id"}},
		},
		{
			name:     "only the first equals sign splits",
			specs:    []string{"A=b=c"},
			expected: []envFieldMapping{{"A", "b=c"}},
		},
		{
			name:     "malformed entries are reported and skipped",
			specs:    []string{"NOEQUALS", "=field", "VAR=", "GOOD=good"},
			expected: []envFieldMapping{{"GOOD", "good"}},
			badSpecs: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, errs := parseEnvFields(tc.specs)
			assert.Equal(t, tc.expected, got)
			assert.Len(t, errs, tc.badSpecs)
		})
	}
}

func TestEnvFields(t *testing.T) {
	t.Setenv("CI_PIPELINE_ID", "4242")
	t.Setenv("CI_JOB_NAME", "")
	t.Setenv("BUILDEVENTS_TEST_UNSET", "x")
	// make sure the "unset" case really is unset for this process
	require.NoError(t, os.Unsetenv("BUILDEVENTS_TEST_UNSET"))

	ev := libhoney.NewEvent()
	ev.AddField("build_num", "from-provider")

	envFields([]string{
		"CI_PIPELINE_ID=ci.pipeline.id",
		"CI_PIPELINE_ID=build_num",
		"CI_JOB_NAME=ci.job.name",
		"BUILDEVENTS_TEST_UNSET=never.sent",
		"garbage",
	}, ev)

	fields := ev.Fields()
	assert.Equal(t, "4242", fields["ci.pipeline.id"])
	assert.Equal(t, "4242", fields["build_num"], "a mapping replaces the provider value for the same field")
	assert.Equal(t, "", fields["ci.job.name"], "a variable set to the empty string is still reported")
	assert.NotContains(t, fields, "never.sent")
	assert.NotContains(t, fields, "garbage")
}

func TestEnvFieldFlag(t *testing.T) {
	testCases := []struct {
		name     string
		env      string
		args     []string
		expected []string
	}{
		{name: "unset", args: []string{"build"}},
		{
			name:     "repeated flag",
			args:     []string{"-e", "A=a", "--env_field", "B=b", "build"},
			expected: []string{"A=a", "B=b"},
		},
		{
			name:     "comma separated flag",
			args:     []string{"--env_field", "A=a,B=b", "build"},
			expected: []string{"A=a", "B=b"},
		},
		{
			name:     "from the environment",
			env:      "A=a,B=b",
			args:     []string{"build"},
			expected: []string{"A=a", "B=b"},
		},
		{
			name:     "flags add to the environment",
			env:      "A=a",
			args:     []string{"--env_field", "B=b", "build"},
			expected: []string{"A=a", "B=b"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("BUILDEVENT_ENV_FIELDS", tc.env)
			} else {
				t.Setenv("BUILDEVENT_ENV_FIELDS", "") // registers the restore
				require.NoError(t, os.Unsetenv("BUILDEVENT_ENV_FIELDS"))
			}

			var cfg libhoney.Config
			var filename, provider, serviceName string
			var specs []string
			root := commandRoot(&cfg, &filename, &provider, &serviceName, &specs)
			// a stand in for the real subcommands, which would send events
			root.AddCommand(&cobra.Command{Use: "build", Run: func(*cobra.Command, []string) {}})
			root.SetArgs(tc.args)
			require.NoError(t, root.Execute())

			assert.Equal(t, tc.expected, specs)
		})
	}
}
