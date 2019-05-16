# buildevents [![Build Status](https://travis-ci.org/honeycombio/buildevents.svg?branch=master)](https://travis-ci.org/honeycombio/buildevents)

buildevents is a small binary used to help instrument Travis-CI builds. It is installed during the setup phase and then invoked as part of each step in order to visualize the build as a trace in Honeycomb

The trace that you get at the end has a span for every step of the travis build and a span for every instrumented command within each step. Here's an example showing a build that goes through the `install`, `before_script`, `script`, and `after_success` steps:

![buildevents_trace](https://user-images.githubusercontent.com/361454/53279652-23e34700-36c7-11e9-876c-4dc716416393.png)

# Setup

You need to add an environment variable to your Travis build (done through the Travis UI) to set the API key to use to send the trace to Honeycomb. Set `BUILDEVENT_APIKEY` to your Honeycomb API key (available at https://ui.honeycomb.io/account)

Other optional environment variables available:

* `BUILDEVENT_URL` the base URL back to the builds. This is used to construct a link back to the build log from within the trace. It should look something like `https://travis-ci.org/honeycombio/buildevents/builds/`
* `BUILDEVENT_DATASET` overrides the default Honeycomb dataset to use. Default is `travis-ci builds`
* `BUILDEVENT_APIHOST` overrides the API target for sending Honeycomb traces.  Default is `https://api.honeycomb.io/`

# Use

In the `install` section of your `.travis.yml`, add a line to install buildevents.

```yaml
install:
  - go get https://github.com/honeycombio/buildevents
```

In every subsequent step, you must set a start and span ID as environment variables, and then invoke `buildevents` once at the end of that step. This sends the span representing the entire step. It's important to use the name of the step as the last argument to the buildevents command

```yaml
script:
  - STEP_START=$(date +%s)
  - STEP_SPAN_ID=$(echo $RANDOM | sha256sum | awk '{print $1}')
  - ...
  - ...   regular content of the 'script' step
  - ...
  - buildevents step $TRAVIS_BUILD_ID $STEP_SPAN_ID $STEP_START script
```

For every command within a step that you want to create an additional span, add another `bulidevents` call of the `cmd` type. Do this for any commands within the step that you think will take significant time or you want to measure. Put the actual command to run after the double hyphen.

```yaml
  - ... previous things in the step
  - buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID poodle-test -- yarn test
  - buildevents cmd $TRAVIS_BUILD_ID $STEP_SPAN_ID poodle-lint -- yarn lint
  - ...
```

# Positional argument reference

buildevents requires all its arguments.

* `step` or `cmd` - main travis section or command within that section
* `build_id` this is used as both the trace ID and to generate a URL to link back to the build
* `step_id` buildevents expects a build to contain steps, and each step to have commands. The step ID is used to help construct this tree
* `start_time` _only in `step` type spans_ used to calculate the total duration of running this section of the build
* `name` the last argument is the name for this step or command, used in the Honeycomb UI


