package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/honeycombio/libhoney-go"
	"github.com/stretchr/testify/assert"
)

const (
	apiKey        = ""
	classicAPIKey = ""
)

func TestBuildUrl(t *testing.T) {
	testCases := []struct {
		Name        string
		APIKey      string
		expectedUrl string
	}{
		{Name: "classic", APIKey: "lcYrFflRUR6rHbIifwqhfGRUR6rHbIic", expectedUrl: "test_team/datasets/test_dataset/trace"},
		{Name: "non-classic", APIKey: "lcYrFflRUR6rHbIifwqhfG", expectedUrl: "test_team/environments/test_env/datasets/test_dataset/trace"},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/1/auth", r.URL.Path)
					assert.Equal(t, []string{tc.APIKey}, r.Header["X-Honeycomb-Team"])

					if isClassic(tc.APIKey) {
						w.Write([]byte(`{"team":{"slug":"test_team"}}`))
					} else {
						w.Write([]byte(`{"team":{"slug":"test_team"},"environment":{"slug":"test_env"}}`))
					}
				}),
			)
			defer server.Close()

			config := libhoney.Config{
				APIKey:  tc.APIKey,
				APIHost: server.URL,
				Dataset: "test_dataset",
			}

			url, err := buildURL(&config, "trace_id", time.Now().UTC().UnixNano())
			assert.Nil(t, err)
			expectedUrl := server.URL + "/" + tc.expectedUrl
			assert.Equal(t, expectedUrl, url[:strings.IndexByte(url, '?')])
		})
	}
}
