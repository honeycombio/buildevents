MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables

GOTESTCMD = $(if $(shell command -v gotestsum),gotestsum --junitfile ./test_results/$(1).xml --format testname --,go test)

.PHONY: test
#: run all tests
test: test_with_race test_all

.PHONY: test_with_race
#: run only tests tagged with potential race conditions
test_with_race: test_results
	@echo
	@echo "+++ testing - race conditions?"
	@echo
	$(call GOTESTCMD,$@) -tags race --race --timeout 60s -v ./...

.PHONY: test_all
#: run all tests, but with no race condition detection
test_all: test_results
	@echo
	@echo "+++ testing - all the tests"
	@echo
	$(call GOTESTCMD,$@) -tags all --timeout 60s -v ./...

test_results:
	@mkdir -p test_results

.PHONY: install-tools
install-tools:
	go install github.com/google/go-licenses/v2@v2.0.0-alpha.1

.PHONY: update-licenses
update-licenses: install-tools
	rm -rf LICENSES; \
	#: We ignore the standard library (go list std) as a workaround for \
	"https://github.com/google/go-licenses/issues/244." The awk script converts the output \
  of `go list std` (line separated modules) to the input that `--ignore` expects (comma separated modules).
	go-licenses save --save_path LICENSES --ignore "github.com/honeycombio/buildevents" \
		--ignore $(shell go list std | awk 'NR > 1 { printf(",") } { printf("%s",$$0) } END { print "" }') ./;


.PHONY: verify-licenses
verify-licenses: install-tools
	go-licenses save --save_path temp --ignore "github.com/honeycombio/buildevents" \
		--ignore $(shell go list std | awk 'NR > 1 { printf(",") } { printf("%s",$$0) } END { print "" }') ./; \
	chmod +r temp; \
    if diff temp LICENSES; then \
      echo "Passed"; \
      rm -rf temp; \
    else \
      echo "LICENSES directory must be updated. Run make update-licenses"; \
      rm -rf temp; \
      exit 1; \
    fi; \
