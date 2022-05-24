# buildevents

[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/buildevents?color=success)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)
[![CircleCI](https://circleci.com/gh/honeycombio/buildevents.svg?style=shield)](https://circleci.com/gh/honeycombio/buildevents)

buildevents is a small binary used to help instrument builds in a build system such as Travis-CI, CircleCI, Jenkins, and so on. It is installed during the setup phase and then invoked as part of each step in order to visualize the build as a trace in Honeycomb

The trace that you get at the end represents the entire build. It has spans for each section and subsection of the build, representing groups of actual commands that are run. The duration of each span is how long that stage or specific command took to run, and includes whether or not the command succeeded.

Here's an example showing a build that ran on CircleCI. It goes through running go tests, setting up javascript dependencies, triggers js_build and poodle_test in parallel after dependencies are configured, and then continues off below the captured portion of the waterfall.

![CircleCI_Build_Trace](https://user-images.githubusercontent.com/361454/57872910-ac9eea00-77c1-11e9-8bdd-db7a870dcd61.png)

# Setup

Getting your build ready to use `buildevents` involves:
* installing the `buildevents` binary in your build environment
* setting a number of environment variables for configuring the tool
* choosing a unique trace identifier

## Installation

If you have a working go environment in your build, the easiest way to install `buildevents` is via `go get`.

```
go get github.com/honeycombio/buildevents/
```

There are also built binaries for linux and macOS hosted on Github and available under the [releases](https://github.com/honeycombio/buildevents/releases) tab. The following commands will download and make executable the github-hosted binary.

**linux, 32-bit x86:**

```
curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-linux-386
chmod 755 buildevents
```

**linux, 64-bit x86:**

```
curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-linux-amd64
chmod 755 buildevents
```

**linux, arm64:**

```
curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-linux-arm64
chmod 755 buildevents
```

**macOS:**

```
curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-darwin-amd64
chmod 755 buildevents
```

If this doesn't work for you, please [let us know](mailto:support@honeycomb.io) - we'd love to hear what would work.

<!-- TODO provide a compiled binary at some evergreen 'latest' URL  -->

## Environment Variables

There is one required environment variable; it will hold your Honeycomb API key (available at https://ui.honeycomb.io/account). If it is absent, events will not be sent to Honeycomb. Set `BUILDEVENT_APIKEY` to hold your API key.

There are several other optional enviornment variables that will adjust the behavior of `buildevents`:

* `BUILDEVENT_DATASET` sets the Honeycomb dataset to use. The default is `buildevents`
* `BUILDEVENT_APIHOST` sets the API target for sending Honeycomb traces.  Default is `https://api.honeycomb.io/`
* `BUILDEVENT_CIPROVIDER` if set, a field in all spans named `ci_provider` will contain this value. If unset, `buildevents` will inspect the environment to try and detect Travis-CI, CircleCI, GitLab-CI, Buildkite, Jenkins-X, Google-Cloud-Build and Bitbucket-Pipelines (by looking for the environment variables `TRAVIS`, `CIRCLECI`, `BUILDKITE`, `GITLAB_CI`, `JENKINS-X`, `GOOGLE-CLOUD-BUILD` and `BITBUCKET_BUILD_NUMBER` respectively). If either Travis-CI, CircleCI, GitLab-CI, Buildkite, Jenkins-X, Google-Cloud-Build or Bitbucket-Pipelines are detected, `buildevents` will add a number of additional fields from the environment, such as the branch name, the repository, the build number, and so on. If detection fails and you are on Travis-CI, CircleCI, GitLab-CI, Jenkins-X, Google-Cloud-Build or Bitbucket-Pipelines setting this to `Travis-CI`, `CircleCI`, `Buildkite`, `GitLab-CI`, `Jenkins-X`, `Google-Cloud-Build`, or `Bitbucket-Pipelines` precisely will also trigger the automatic field additions.
* `BUILDEVENT_FILE` if set, is used as the path of a text file holding arbitrary key=val pairs (multi-line-capable, logfmt style) that will be added to the Honeycomb event.

## Trace Identifier

The `buildevents` script needs a unique ID to join together all of the steps and commands with the build.  This is the Trace ID. It must be unique within the Honeycomb dataset holding traces. An excellent choice is the Build ID, since it is both unique (even when re-running builds, you will often get a new Build ID) and is also a primary value that the build system uses to identify the build.

The Build ID may already be available in the environment for your build:
* Travis-CI: `TRAVIS_BUILD_ID`
* CircleCI: `CIRCLE_WORKFLOW_ID` (if you're using workflows)
* CircleCI: `CIRCLE_BUILD_NUM` (the build number for this job if you're not using workflows)
* GitLab-CI: `CI_PIPELINE_ID`
* Buildkite: `BUILDKITE_BUILD_ID`
* JenkinsX: `JENKINSX_BUILD_NUMBER`
* Google-Cloud-Build: `BUILD_ID`
* GitHub Actions: `GITHUB_RUN_ID`
* Bitbucket Pipelines: `BITBUCKET_BUILD_NUMBER`

# Use

Now that `buildevents` is installed and configured, actually generating spans to send to Honeycomb involves invoking `buildevents` in various places throughout your build config.

`buildevents` is invoked with one of three modes, `build`, `step`, and `cmd`.
* The `build` mode sends the root span for the entire build. It should be called when the build finishes and records the duration of the entire build. It emits a URL pointing to the generated trace in Honeycomb to STDOUT.
* The `step` mode represents a block of related commands. In Travis-CI, this is one of `install`, `before_script`, `script`, and so on. In CircleCI, this most closely maps to a single job. It should be run at the end of the step.
* The `cmd` mode invokes an individual command that is part of the build, such as running DB migrations or running a specific test suite. It must be able to be expressed as a single shell command - either a process like `go test` or a shell script. The command to run is the final argument to `buildevents` and will be launched via `bash -c` using `exec`. You can specify an alternate shell using the `-s/--shell` flag but it must support the the `-c` flag.

## build

Though listed first, running `buildevents` in `build` mode should actually be the last command that your build runs so that it can record the total running duration for the build. It does this by having the time the build started as one of the arguments passed in.

The output of buildevents in `build` will be a link to the trace within Honeycomb. Take this URL and use it in the notifications your CI system emits to easily jump to the Honeycomb trace for a build. If the API Key used in this run is not valid, no output will be emitted.

Note that CircleCI uses an alternate method of creating the root span, so the `build` command should not be used. Use the `watch` command instead.

For the `build` step, you must first record the time the build started.
* Travis-CI: the `env` section of the config file establishes some global variables in the environment. This is run before anything else, so gets a good start time.

The actual invocation of `buildevents build` should be as close to the last thing that the build does as possible.
* Travis-CI: the end of the `after_failure` and `after_success` steps

Travis-CI example:
```yaml
env:
  global:
    - BUILD_START=$(date +%s)

...

after_failure:
  - traceURL=$(buildevents build $TRAVIS_BUILD_ID $BUILD_START failure)
  - echo "Honeycomb Trace: $traceURL"
after_success:
  - traceURL=$(buildevents build $TRAVIS_BUILD_ID $BUILD_START success)
  - echo "Honeycomb Trace: $traceURL"
```

### what it generates

Given this command:

```bash
buildevents $HOST -k $API_KEY build htjebmye $BUILD_STARTTIME success
```

The event that arrives at Honeycomb (which has no trace.parent_id since it is the root of a trace) might look like:

```json
{
    "Timestamp": "2022-05-24T01:49:13Z",
    "command_name": "build",
    "duration_ms": 4981,
    "meta.version": "dev",
    "name": "build htjebmye",
    "service.name": "build",
    "service_name": "build",
    "source": "buildevents",
    "status": "success",
    "trace.span_id": "htjebmye",
    "trace.trace_id": "htjebmye"
}
```

## watch

CirclecI requires use of the CircleCI API to detect when workflows start and stop. There is no facility to always run a job after all others, so what works using the Travis-CI `after_failure` will not work on CircleCI. However, the CircleCI API exposes when the current workflow has started, and can be used intsead.

The `watch` command polls the CircleCI API and waits until all jobs have finished (either succeeded, failed, or are blocked). It then reports the final status of the build with the appropriate timers.  `watch` should be invoked in a job all on its own, dependent on only the `setup` job, with only the Trace ID to use. After some time, `watch` will timeout waiting for the build to finish and fail. The timeout default is 10 minutes and can be overridden by setting `BUILDEVENT_TIMEOUT`

Using the `watch` command requires a personal (not project) CircleCI API token. You can provide this token to `buildevents` via the `BUILDEVENT_CIRCLE_API_TOKEN` environment variable. You can get a personal API token from https://circleci.com/account/api. For more detail on tokens, please see the [CircleCI API Tokens documentation](https://circleci.com/docs/2.0/managing-api-tokens/)

The `watch` command will emit a link to the finished trace to the job output in Honeycomb when the build is complete.

```yaml
jobs:
  send_trace:
    steps:
      - run: buildevents watch $CIRCLE_WORKFLOW_ID
```

## step

The `step` mode is the outer wrapper that joins a collection of individual `cmd`s together in to a block. Like the `build` command, it should be run at the end of the collection of `cmd`s and needs a start time collected at the beginning. In addition to the trace identifier, it needs a step identifier that will also be passed to all the `cmd`s that are part of this step in order to tie them together in to a block. Because the step identifier must be available to all commands, both it and the start time should be generated at the beginning of the step and recorded. The step identifier must be unique within the trace (but does not need to be globally unique). To avoid being distracting, we use a hash of the step name as the identifier.

Travis-CI exmaple:
```yaml
before_script:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo before_script | sum | cut -f 1 -d \ )
  - ... do stuff
  - buildevents step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START before_script
```

CircleCI example:
```yaml
jobs:
  go_test:
    steps:
      - run: echo "STEP_START=$(date +%s)" >> $BASH_ENV
      - run: echo "STEP_SPAN_ID=$(echo go_test | sum | cut -f 1 -d \ )" >> $BASH_ENV
      - run: ... do stuff
      - run:
          name: finishing span for the job
          command: $GOPATH/bin/buildevents step $CIRCLE_WORKFLOW_ID $STEP_SPAN_ID $STEP_START go_test
          when: always   # ensures the span is always sent, even when something in the job fails
```

### what it generates

Given this command:

```bash
buildevents $HOST -k $API_KEY step htjebmye building_htjebmye $STEP_STARTTIME building
```

The event that arrives at Honeycomb might look like:

```json
{
    "Timestamp": "2022-05-24T01:49:14Z",
    "command_name": "step",
    "duration_ms": 3064,
    "meta.version": "dev",
    "name": "building",
    "service.name": "step",
    "service_name": "step",
    "source": "buildevents",
    "trace.parent_id": "htjebmye",
    "trace.span_id": "building_htjebmye",
    "trace.trace_id": "htjebmye"
}
```

## cmd

Running `buildevents cmd` will run the given command, time it, and include the `status` of the command (`success` or `failure`). `buildevents` passes through both STDOUT and STDERR from the process it wraps, and exits with the same exit code as the wrapped process. The actual command to run is separated from the `buildevents` arguments by a double hyphen `--`.

This is the most frequent line you'll see in your config file; anything of consequence should generate a span.

Travis-CI example:
```yaml
script:
  - buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID go-test -- go test -timeout 2m -mod vendor ./...
```

CircleCI example:
```yaml
jobs:
  go_test:
    steps:
      - run: $GOPATH/bin/buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID go-test -- go test -timeout 2m -mod vendor ./...
```

### what it generates

Given this command:

```bash
buildevents $HOST -k $API_KEY cmd htjebmye building_htjebmye compile -- sleep 1
```

The event that arrives at Honeycomb might look like:

```json
{
    "Timestamp": "2022-05-24T01:49:14.653182Z",
    "cmd": "\"sleep\" \"1\"",
    "command_name": "cmd",
    "duration_ms": 1008,
    "meta.version": "dev",
    "name": "compile",
    "service.name": "cmd",
    "service_name": "cmd",
    "source": "buildevents",
    "status": "success",
    "trace.parent_id": "building_htjebmye",
    "trace.span_id": "6facde6ac6a95e704b9ec1c837270578",
    "trace.trace_id": "htjebmye"
}
```

## Attaching more traces from your build and test process

Every command running through `buildevents cmd` will receive a `HONEYCOMB_TRACE` environment variable that contains a marshalled trace propagation context. This can be used to connect more spans to this trace.

Ruby Beeline example:
```ruby
# at the very start of the command
# establish a command-level span, linking to the buildevent
process_span = Honeycomb.start_span(name: File.basename($PROGRAM_NAME), serialized_trace: ENV['HONEYCOMB_TRACE'])
Honeycomb.add_field_to_trace('process.full_name', $PROGRAM_NAME)

# if you're not passing sensitive information through CLI args, enable this for more insights.
#Honeycomb.add_field_to_trace('process.args', ARGV)

# override the HONEYCOMB_TRACE for sub-processes
ENV['HONEYCOMB_TRACE'] = process_span.to_trace_header

# ensure that the process_span is sent before the process terminates
at_exit do
  if $ERROR_INFO&.is_a?(SystemExit)
    process_span.add_field('process.exit_code', $ERROR_INFO.status)
  elsif $ERROR_INFO
    process_span.add_field('process.exit_code', $ERROR_INFO.class.name)
  else
    process_span.add_field('process.exit_code', 'unknown')
  end
  process_span.send
end
```

# Putting it all together

We've covered each of the three modes in which `buildevents` is invoked and shown abbreviated examples for each one. Now it's time to look at an entire config to see how they interact: installation, running a build, and finally reporting the whole thing.

In both of these examples, the `BUILDEVENT_APIKEY` should be set in the protected environment variable section of the CI config so that your API key is not checked in to your source.

Travis-CI example:
```yaml
env:
  global:
    - BUILD_START=$(date +%s)

install:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo install | sum | cut -f 1 -d \ )
  - curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-linux-amd64
  - chmod 755 buildevents
  - # ... any other setup necessary for your build
  - ./buildevents step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START install

script:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo script | sum | cut -f 1 -d \ )
  - ./buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID go-tests -- go test ./...
  - ./buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID js-tests -- yarn test
  - ./buildevents step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START script

after_failure:
  - ./buildevents build $TRAVIS_BUILD_ID $BUILD_START failure

after_success:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo after_success | sum | cut -f 1 -d \ )
  - ./buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID build -- go install ./...
  - # ... tar up artifacts, upload them, etc.
  - ./buildevents step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START after_success
  - ./buildevents build $TRAVIS_BUILD_ID $BUILD_START success
```

CircleCI example:
```yaml
version: 2.1

# factored out start/finish_job_span commands here so we don't have every one of our build jobs duplicating them
commands:
  with_job_span:
    parameters:
      steps:
        type: steps
    steps:
      - attach_workspace:
          at: buildevents
      - run:
          name: starting span for job
          command: |
            echo "STEP_START=$(date +%s)" >> $BASH_ENV
            echo "STEP_SPAN_ID=$(echo $CIRCLE_JOB | sum | cut -f 1 -d \ )" >> $BASH_ENV
      - run: echo "PATH=$PATH:buildevents/bin/" >> $BASH_ENV
      - steps: << parameters.steps >>
      - run:
          name: finishing span for job
          command: buildevents step $CIRCLE_WORKFLOW_ID $STEP_SPAN_ID $STEP_START $CIRCLE_JOB
          when: always

jobs:
  setup:
    steps:
      - run: |
          mkdir -p buildevents/bin
          date +%s > buildevents/build_start
      - run: go get github.com/honeycombio/buildevents
      - run: cp $GOPATH/bin/buildevents buildevents/bin/
      - persist_to_workspace:
          root: buildevents
          paths:
            - build_start
            - bin/buildevents
  send_trace:
    steps:
      - attach_workspace:
          at: buildevents
      - run: buildevents watch $CIRCLE_WORKFLOW_ID
  test:
    steps:
      - with_job_span:
          steps:
            - run: buildevents cmd $CIRCLE_WORKFLOW_ID $STEP_SPAN_ID go-tests -- go test ./...
            - run: buildevents cmd $CIRCLE_WORKFLOW_ID $STEP_SPAN_ID js-tests -- yarn test
  build:
    steps:
      - with_job_span:
          steps:
            - run: mkdir artifacts
            - run: buildevents cmd $CIRCLE_WORKFLOW_ID $STEP_SPAN_ID build -- go install ./...
            - run: # publish your build artifacts

workflows:
  test-and-build:
    jobs:
      - setup
      - send_trace:
          requires:
            - setup
      - test:
          requires:
            - setup
      - build:
          requires:
            - test
```

GitLab CI example:
```yaml
# Not a huge fan of YAML anchors, but it's the easiest way to
# extend the scripts in jobs where you need other before_script and after_script
.default_before_script: &default_before_script
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo $CI_JOB_NAME | sum | cut -f 1 -d \ )
  - echo "export STEP_START=$STEP_START" >> buildevents/env
  - echo "export STEP_SPAN_ID=$STEP_SPAN_ID" >> buildevents/env
  - echo "export PATH=\"$PATH:buildevents/bin/\"" >> buildevents/env
  - source buildevents/env
  - cat buildevents/env

.default_after_script: &default_after_script
  - source buildevents/env
  - cat buildevents/env
  - buildevents step $CI_PIPELINE_ID $STEP_SPAN_ID $STEP_START $CI_JOB_NAME

default:
  image: golang:latest
  before_script:
    - *default_before_script
  after_script:
    - *default_after_script

stages:
  # .pre and .post are guaranteed to be first and last run jobs
  # https://docs.gitlab.com/ee/ci/yaml/README.html#pre-and-post
  - .pre
  - build
  - test
  - .post

setup:
  before_script:
    - mkdir -p buildevents/bin/
    - *default_before_script
  script:
    - curl -L -o main https://github.com/honeycombio/buildevents/releases/latest/download/buildevents-linux-amd64
    - chmod 755 main
    - mv main buildevents/bin/buildevents
    - export BUILD_START=$(date +%s)
    - echo "export BUILD_START=$(date +%s)" >> buildevents/env
  artifacts:
    paths:
      - buildevents
  stage: .pre

go_build:
  script:
    - buildevents cmd $CI_PIPELINE_ID $STEP_SPAN_ID build -- go install ./...
  stage: build

go_test:
  script:
    - buildevents cmd $CI_PIPELINE_ID $STEP_SPAN_ID build -- go test ./...
  stage: test

send_success_trace:
  script:
    - "traceURL=$(buildevents build $CI_PIPELINE_ID $BUILD_START success)"
    - "echo \"Honeycomb Trace: $traceURL\""
  stage: .post
  rules:
    - when: on_success

send_failure_trace:
  script:
    - "traceURL=$(buildevents build $CI_PIPELINE_ID $BUILD_START failure)"
    - "echo \"Honeycomb Trace: $traceURL\""
  stage: .post
  rules:
    - when: on_failure
```

# Positional argument reference

All the arguments to the various `buildevents` modes are listed above, but for
convenience, here is a summary of the modes and the arguments that each
requires.

The first argument is the running mode for this invocation of buildevents:
`build`, `watch`, `step`, or `cmd` The remaining arguments differ depending on the
mode.

arguments for the `build` mode:
1. `build_id` this is used as both the trace ID and to generate a URL to link back to the build
2. `start_time` used to calculate the total duration of the build
3. `status` should be `success` or `failure` and indicates whether the overall build succeeeded or failed

arguments for the `watch` mode:
1. `build_id` this is used as the trace ID

arguments for the `step` mode:
1. `build_id` this is used as both the trace ID and to generate a URL to link back to the build
1. `step_id` buildevents expects a build to contain steps, and each step to have commands. The step ID is used to help construct this tree
1. `start_time` used to calculate the total duration of running this step in the build
1. `name` the last argument is the name for this step or command, used in the Honeycomb UI

arguments for the `cmd` mode:
1. `build_id` this is used as both the trace ID and to generate a URL to link back to the build
1. `step_id` buildevents expects a build to contain steps, and each step to have commands. The step ID is used to help construct this tree
1. `name` the name for this command, used in the Honeycomb UI
1. `--` double hyphen indicates the rest of the line will be the command to run

## Note
`name` is most useful if it is a low-cardinality value, usually something like the name of a step in your process. Using a low-cardinality value makes it valuable to do things like `GROUP BY name` in your queries.

## Differences between Classic and non-Classic environments

For "Honeycomb Classic", `buildevents` works almost the same as it always has. It has added service.name in addition to service_name; both fields have the same value.

In a non-Classic environment, there are several differences:
* Service Name, if specified, is used as the dataset as well as both `service_name` and `service.name` fields.
* if dataset is specified and service name is not, it will be used but will generate a warning.
* if both are specified, service name will be used, dataset is ignored, and a warning will be emitted (except in quiet mode)
* the command name is now sent as command_name (in classic it is sent as service_name)
* the watch command now sets the `name` field to merely `watch` rather than a high-cardinality value, making it easier to aggregate queries across different builds

