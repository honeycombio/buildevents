module github.com/honeycombio/buildevents

go 1.23.0
toolchain go1.23.2

require (
	github.com/honeycombio/beeline-go v1.17.0
	github.com/honeycombio/libhoney-go v1.23.1
	github.com/jszwedko/go-circleci v0.3.0
	github.com/kr/logfmt v0.0.0-20140226030751-b84e30acd515
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.9.0
)

replace github.com/jszwedko/go-circleci => github.com/maplebed/go-circleci v0.0.0-20191121000249-089ef54587e5
