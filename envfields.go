package main

import (
	"fmt"
	"os"
	"strings"

	libhoney "github.com/honeycombio/libhoney-go"
)

// envFieldMapping ties an environment variable to the name of the event field
// that should carry its value.
type envFieldMapping struct {
	envVar string
	field  string
}

// parseEnvFields turns specs of the form ENV_VAR=field.name into mappings.
// Entries that are malformed are reported in the returned error list and
// skipped, so one typo does not stop a build from being traced.
func parseEnvFields(specs []string) ([]envFieldMapping, []error) {
	var mappings []envFieldMapping
	var errs []error
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		envVar, field, ok := strings.Cut(spec, "=")
		envVar, field = strings.TrimSpace(envVar), strings.TrimSpace(field)
		if !ok || envVar == "" || field == "" {
			errs = append(errs, fmt.Errorf("ignoring env field %q: expected ENV_VAR=field_name", spec))
			continue
		}
		mappings = append(mappings, envFieldMapping{envVar: envVar, field: field})
	}
	return mappings, errs
}

// envFields adds a field for each requested environment variable that is set.
// It runs after providerInfo, so a mapping can reuse a built in field name to
// replace its value, and before arbitraryFields, so the extra fields file
// still has the last word.
func envFields(specs []string, ev *libhoney.Event) {
	mappings, errs := parseEnvFields(specs)
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err)
	}
	for _, m := range mappings {
		if val, ok := os.LookupEnv(m.envVar); ok {
			ev.AddField(m.field, val)
		}
	}
}
