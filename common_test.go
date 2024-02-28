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

func TestBuildUrl(t *testing.T) {
	testCases := []struct {
		Name        string
		APIKey      string
		Classic     bool
		expectedUrl string
	}{
		{Name: "classic", Classic: true, APIKey: "25f7d47575430a8fafea5b7d70a6af09", expectedUrl: "test_team/datasets/test_dataset/trace"},
		{Name: "classic ingest key", Classic: true, APIKey: "hcaic_1234567890123456789012345678901234567890123456789012345678", expectedUrl: "test_team/datasets/test_dataset/trace"},
		{Name: "non classic v2 configuration key", Classic: false, APIKey: "lcYrFflRUR6rHbIifwqhfG", expectedUrl: "test_team/environments/test_env/datasets/test_dataset/trace"},
		{Name: "non classic ingest key", Classic: false, APIKey: "hcxik_01hqk4k20cjeh63wca8vva5stw70nft6m5n8wr8f5mjx3762s8269j50wc", expectedUrl: "test_team/environments/test_env/datasets/test_dataset/trace"},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/1/auth", r.URL.Path)
					assert.Equal(t, []string{tc.APIKey}, r.Header["X-Honeycomb-Team"])

					if tc.Classic {
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
