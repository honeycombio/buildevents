# buildevents [![Build Status](https://travis-ci.org/honeycombio/buildevents.svg?branch=master)](https://travis-ci.org/honeycombio/buildevents)

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

There is also a built binary (for linux) hosted on Github and available under the [releases](https://github.com/honeycombio/buildevents/releases) tab. The following two commands will down load and make executable the github-hosted binary.
```
curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents
chmod 755 buildevents
```

If this doesn't work for you, please [let us know](mailto:support@honeycomb.io) - we'd love to hear what would work.

<!-- TODO provide a compiled binary at some evergreen 'latest' URL  -->

## Environment Variables

There is one required environment variable; it will hold your Honeycomb API key (available at https://ui.honeycomb.io/account). If it is absent, events will not be sent to Honeycomb. Set `BUILDEVENT_APIKEY` to hold your API key.

There are several other optional enviornment variables that will adjust the behavior of `buildevents`:

* `BUILDEVENT_DATASET` sets the Honeycomb dataset to use. The default is `buildevents`
* `BUILDEVENT_APIHOST` sets the API target for sending Honeycomb traces.  Default is `https://api.honeycomb.io/`
* `BUILDEVENT_CIPROVIDER` if set, a field in all spans named `ci_provider` will contain this value. If unset, `buildevents` will inspect the environment to try and detect Travis-CI and CircleCI (by looking for the environment variables `TRAVIS` and `CIRCLECI` respectively). If either Travis-CI or CircleCI are detected, `buildevents` will add a number of additional fields from the environment, such as the branch name, the repository, the build number, and so on. If detection fails and you are on Travis-CI or CircleCI, setting this to `Travis-CI` or `CircleCI` precisely will also trigger the automatic field additions.

## Trace Identifier

The `buildevents` script needs a unique ID to join together all of the steps and commands with the build.  This is the Trace ID. It must be unique within the Honeycomb dataset holding traces. An excellent choice is the Build ID, since it is both unique (even when re-running builds, you will often get a new Build ID) and is also a primary value that the build system uses to identify the build.

The Build ID may already be available in the environment for your build:
* Travis-CI: `TRAVIS_BUILD_ID`
* CircleCI: `CIRCLE_WORKFLOW_ID` (if you're using workflows)
* CircleCI: `CIRCLE_BUILD_NUM` (the build number for this job if you're not using workflows)

# Use

Now that `buildevents` is installed and configured, actually generating spans to send to Honeycomb involves invoking `buildevents` in various places throughout your build config.

`buildevents` is invoked with one of three modes, `build`, `step`, and `cmd`.
* The `build` mode sends the root span for the entire build. It should be called when the build finishes and records the duration of the entire build.
* The `step` mode represents a block of related commands. In Travis-CI, this is one of `install`, `before_script`, `script`, and so on. In CircleCI, this most closely maps to a single job. It should be run at the end of the step.
* The `cmd` mode invokes an individual command that is part of the build, such as running DB migrations or running a specific test suite. It must be able to be expressed as a single shell command - either a process like `go test` or a shell script. The command to run is the final argument to `buildevents` and will be launched via `bash -c` using `exec`.

## build

Though listed first, running `buildevents` in `build` mode should actually be the last command that your build runs so that it can record the total running duration for the build. It does this by having the time the build started as one of the arguments passed in.

For the `build` step, you must first record the time the build started.
* Travis-CI: the `env` section of the config file establishes some global variables in the environment. This is run before anything else, so gets a good start time.
* CircleCI: make a `setup` job that is `require`d by what would otherwise be the beginning of your build. Record the start time during that job. You will have to persist this value to a workspace for it to be available to other jobs in the workflow.

The actual invocation of `buildevents build` should be as close to the last thing that the build does as possible.
* Travis-CI: the end of the `after_failure` and `after_success` steps
* CircleCI: the last job in the workflow

Travis-CI example:
```yaml
env:
  global:
    - BUILD_START=$(date +%s)

...

after_failure:
  - buildevents build $TRAVIS_BUILD_ID $BUILD_START failure
after_success:
  - buildevents build $TRAVIS_BUILD_ID $BUILD_START success
```

CircleCI example:
```yaml
jobs:
  setup:
    steps:
      - run: |
          mkdir -p ~/be
          date +%s > ~/be/build_start
      - run: |
          curl -L -o ~/be/buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents
          chmod 755 ~/be/buildevents
      - persist_to_workspace:
          root: ~/be
          paths:
            - build_start
            - buildevents
  final:
    steps:
      - attach_workspace:
          at: ~/be
      - run |
          BUILD_START=$(cat buildevents/build_start)
          ~/be/buildevents build $CIRCLE_WORKFLOW_ID $BUILD_START success
```
## step

The `step` mode is the outer wrapper that joins a collection of individual `cmd`s together in to a block. Like the `build` command, it should be run at the end of the collection of `cmd`s and needs a start time collected at the beginning. In addition to the trace identifier, it needs a step identifier that will also be passed to all the `cmd`s that are part of this step in order to tie them together in to a block. Because the step identifier must be available to all commands, both it and the start time should be generated at the beginning of the step and recorded. The step identifier must be unique within the trace (but does not need to be globally unique). To avoid being distracting, we use a hash of the step name as the identifier.

Travis-CI exmaple:
```yaml
before_script:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo before_script | sum | cut -f 1 -d \ )
  - ... do stuff
  - buildevents travis-ci step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START before_script
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

# Putting it all together

We've covered each of the three modes in which `buildevents` is invoked and shown abbreviated examples for each one. Now it's time to look at an entire config to see how they interact: installation, running a build, and finally reporting the whole thing.

In both of these examples, the `BUILDEVENTS_APIKEY` should be set in the protected environment variable section of the CI config so that your API key is not checked in to your source.

Travis-CI example:
```yaml
env:
  global:
    - BUILD_START=$(date +%s)

install:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo install | sum | cut -f 1 -d \ )
  - curl -L -o buildevents https://github.com/honeycombio/buildevents/releases/latest/download/buildevents
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
  - ./buildevents travis-ci build $TRAVIS_BUILD_ID $BUILD_START failure

after_success:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo after_success | sum | cut -f 1 -d \ )
  - ./buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID build -- go install ./...
  - # ... tar up artifacts, upload them, etc.
  - ./buildevents  step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START after_success
  - ./buildevents  build $TRAVIS_BUILD_ID $BUILD_START success
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
  final:
    steps:
      - attach_workspace:
          at: buildevents
      - run |
          BUILD_START=$(cat buildevents/build_start)
          buildevents build $CIRCLE_WORKFLOW_ID $BUILD_START success

workflows:
  test-and-build:
    jobs:
      - setup
      - test:
          requires:
            - setup
      - build:
          requires:
            - test
      - final
          requires:
            - test
            - build
```

# Positional argument reference

All the arguments to the various `buildevents` modes are listed above, but for
convenience, here is a summary of the three modes and the arguments that each
requires.

The first argument is the running mode for this invocation of buildevents:
`build`, `step`, or `cmd` The remaining arguments differ depending on the
mode.

arguments for the `build` mode:
1. `build_id` this is used as both the trace ID and to generate a URL to link back to the build
1. `start_time` used to calculate the total duration of the build
1. `status` should be `success` or `failure` and indicates whether the overall build succeeeded or failed

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


