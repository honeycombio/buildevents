module github.com/honeycombio/buildevents

go 1.15

require (
	github.com/honeycombio/beeline-go v1.7.0
	github.com/honeycombio/libhoney-go v1.15.8
	github.com/jszwedko/go-circleci v0.3.0
	github.com/kr/logfmt v0.0.0-20140226030751-b84e30acd515
	github.com/spf13/cobra v1.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/jszwedko/go-circleci => github.com/maplebed/go-circleci v0.0.0-20191121000249-089ef54587e5
