module github.com/honeycombio/buildevents

go 1.15

require (
	github.com/honeycombio/beeline-go v1.11.1
	github.com/honeycombio/libhoney-go v1.18.0
	github.com/jszwedko/go-circleci v0.3.0
	github.com/kr/logfmt v0.0.0-20140226030751-b84e30acd515
	github.com/spf13/cobra v1.6.1
	github.com/stretchr/testify v1.8.4
)

replace github.com/jszwedko/go-circleci => github.com/maplebed/go-circleci v0.0.0-20191121000249-089ef54587e5
