package transmission

// txClient handles the transmission of events to Honeycomb.
//
// Overview
//
// Create a new instance of Client.
// Set any of the public fields for which you want to override the defaults.
// Call Start() to spin up the background goroutines necessary for transmission
// Call Add(Event) to queue an event for transmission
// Ensure Stop() is called to flush all in-flight messages.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/facebookgo/muster"
)

const (
	apiMaxBatchSize    int = 5000000 // 5MB
	apiEventSizeMax    int = 100000  // 100KB
	maxOverflowBatches int = 10
)

// Version is the build version, set by libhoney
var Version string

type Honeycomb struct {
	MaxBatchSize           uint          // how many events to collect into a batch before sending
	BatchTimeout           time.Duration // how often to send off batches
	MaxConcurrentBatches   uint          // how many batches can be inflight simultaneously
	PendingWorkCapacity    uint          // how many events to allow to pile up
	BlockOnSend            bool          // whether to block or drop events when the queue fills
	BlockOnResponse        bool          // whether to block or drop responses when the queue fills
	UserAgentAddition      string
	DisableGzipCompression bool // toggles gzip compression when sending batches of events

	responses chan Response

	Transport http.RoundTripper

	muster muster.Client

	Logger  Logger
	Metrics Metrics
}

func (h *Honeycomb) Start() error {
	if h.Logger == nil {
		h.Logger = &nullLogger{}
	}
	h.Logger.Printf("default transmission starting")
	h.responses = make(chan Response, h.PendingWorkCapacity*2)
	h.muster.MaxBatchSize = h.MaxBatchSize
	h.muster.BatchTimeout = h.BatchTimeout
	h.muster.MaxConcurrentBatches = h.MaxConcurrentBatches
	h.muster.PendingWorkCapacity = h.PendingWorkCapacity
	if h.Metrics == nil {
		h.Metrics = &nullMetrics{}
	}
	h.muster.BatchMaker = func() muster.Batch {
		return &batchAgg{
			userAgentAddition: h.UserAgentAddition,
			batches:           map[string][]*Event{},
			httpClient: &http.Client{
				Transport: h.Transport,
				Timeout:   60 * time.Second,
			},
			blockOnResponse:        h.BlockOnResponse,
			responses:              h.responses,
			metrics:                h.Metrics,
			disableGzipCompression: h.DisableGzipCompression,
		}
	}
	return h.muster.Start()
}

func (h *Honeycomb) Stop() error {
	h.Logger.Printf("Honeycomb transmission stopping")
	err := h.muster.Stop()
	close(h.responses)
	return err
}

func (h *Honeycomb) Add(ev *Event) {
	h.Logger.Printf("adding event to transmission; queue length %d", len(h.muster.Work))
	h.Metrics.Gauge("queue_length", len(h.muster.Work))
	if h.BlockOnSend {
		h.muster.Work <- ev
		h.Metrics.Increment("messages_queued")
	} else {
		select {
		case h.muster.Work <- ev:
			h.Metrics.Increment("messages_queued")
		default:
			h.Metrics.Increment("queue_overflow")
			r := Response{
				Err:      errors.New("queue overflow"),
				Metadata: ev.Metadata,
			}
			h.Logger.Printf("got response code %d, error %s, and body %s",
				r.StatusCode, r.Err, string(r.Body))
			writeToResponse(h.responses, r, h.BlockOnResponse)
		}
	}
}

func (h *Honeycomb) TxResponses() chan Response {
	return h.responses
}

func (h *Honeycomb) SendResponse(r Response) bool {
	if h.BlockOnResponse {
		h.responses <- r
	} else {
		select {
		case h.responses <- r:
		default:
			return true
		}
	}
	return false
}

// batchAgg is a batch aggregator - it's actually collecting what will
// eventually be one or more batches sent to the /1/batch/dataset endpoint.
type batchAgg struct {
	// map of batch key to a list of events destined for that batch
	batches map[string][]*Event
	// Used to reenque events when an initial batch is too large
	overflowBatches        map[string][]*Event
	httpClient             *http.Client
	blockOnResponse        bool
	userAgentAddition      string
	disableGzipCompression bool

	responses chan Response
	// numEncoded       int

	metrics Metrics

	// allows manipulation of the value of "now" for testing
	testNower   nower
	testBlocker *sync.WaitGroup
}

// batch is a collection of events that will all be POSTed as one HTTP call
// type batch []*Event

func (b *batchAgg) Add(ev interface{}) {
	// from muster godoc: "The Batch does not need to be safe for concurrent
	// access; synchronization will be handled by the Client."
	if b.batches == nil {
		b.batches = map[string][]*Event{}
	}
	e := ev.(*Event)
	// collect separate buckets of events to send based on the trio of api/wk/ds
	// if all three of those match it's safe to send all the events in one batch
	key := fmt.Sprintf("%s_%s_%s", e.APIHost, e.APIKey, e.Dataset)
	b.batches[key] = append(b.batches[key], e)
}

func (b *batchAgg) enqueueResponse(resp Response) {
	if writeToResponse(b.responses, resp, b.blockOnResponse) {
		if b.testBlocker != nil {
			b.testBlocker.Done()
		}
	}
}

func (b *batchAgg) reenqueueEvents(events []*Event) {
	if b.overflowBatches == nil {
		b.overflowBatches = make(map[string][]*Event)
	}
	for _, e := range events {
		key := fmt.Sprintf("%s_%s_%s", e.APIHost, e.APIKey, e.Dataset)
		b.overflowBatches[key] = append(b.overflowBatches[key], e)
	}
}

func (b *batchAgg) Fire(notifier muster.Notifier) {
	defer notifier.Done()

	// send each batchKey's collection of event as a POST to /1/batch/<dataset>
	// we don't need the batch key anymore; it's done its sorting job
	for _, events := range b.batches {
		b.fireBatch(events)
	}
	// The initial batches could have had payloads that were greater than 5MB.
	// The remaining events will have overflowed into overflowBatches
	// Process these until complete. Overflow batches can also overflow, so we
	// have to prepare to process it multiple times
	overflowCount := 0
	if b.overflowBatches != nil {
		for len(b.overflowBatches) > 0 {
			// We really shouldn't get here but defensively avoid an endless
			// loop of re-enqueued events
			if overflowCount > maxOverflowBatches {
				break
			}
			overflowCount++
			// fetch the keys in this map - we can't range over the map
			// because it's possible that fireBatch will reenqueue more overflow
			// events
			keys := make([]string, len(b.overflowBatches))
			i := 0
			for k := range b.overflowBatches {
				keys[i] = k
				i++
			}

			for _, k := range keys {
				events := b.overflowBatches[k]
				// fireBatch may append more overflow events
				// so we want to clear this key before firing the batch
				delete(b.overflowBatches, k)
				b.fireBatch(events)
			}
		}
	}
}

func (b *batchAgg) fireBatch(events []*Event) {
	start := time.Now().UTC()
	if b.testNower != nil {
		start = b.testNower.Now()
	}
	if len(events) == 0 {
		// we managed to create a batch key with no events. odd. move on.
		return
	}
	encEvs, numEncoded := b.encodeBatch(events)
	// if we failed to encode any events skip this batch
	if numEncoded == 0 {
		return
	}
	// get some attributes common to this entire batch up front off the first
	// valid event (some may be nil)
	var apiHost, writeKey, dataset string
	for _, ev := range events {
		if ev != nil {
			apiHost = ev.APIHost
			writeKey = ev.APIKey
			dataset = ev.Dataset
			break
		}
	}

	// sigh. dislike
	userAgent := fmt.Sprintf("libhoney-go/%s", Version)
	if b.userAgentAddition != "" {
		userAgent = fmt.Sprintf("%s %s", userAgent, strings.TrimSpace(b.userAgentAddition))
	}

	// build the HTTP request
	reqBody, gzipped := buildReqReader(encEvs, !b.disableGzipCompression)
	url, err := url.Parse(apiHost)
	if err != nil {
		end := time.Now().UTC()
		if b.testNower != nil {
			end = b.testNower.Now()
		}
		dur := end.Sub(start)
		b.metrics.Increment("send_errors")
		for _, ev := range events {
			// Pass the parsing error down responses channel for each event that
			// didn't already error during encoding
			if ev != nil {
				b.enqueueResponse(Response{
					Duration: dur / time.Duration(numEncoded),
					Metadata: ev.Metadata,
					Err:      err,
				})
			}
		}
		return
	}
	url.Path = path.Join(url.Path, "/1/batch", dataset)
	req, err := http.NewRequest("POST", url.String(), reqBody)
	req.Header.Set("Content-Type", "application/json")
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Add("X-Honeycomb-Team", writeKey)
	// send off batch!
	resp, err := b.httpClient.Do(req)
	end := time.Now().UTC()
	if b.testNower != nil {
		end = b.testNower.Now()
	}
	dur := end.Sub(start)

	// if the entire HTTP POST failed, send a failed response for every event
	if err != nil {
		b.metrics.Increment("send_errors")
		// Pass the top-level send error down responses channel for each event
		// that didn't already error during encoding
		b.enqueueErrResponses(err, events, dur/time.Duration(numEncoded))
		// the POST failed so we're done with this batch key's worth of events
		return
	}

	// ok, the POST succeeded, let's process each individual response
	b.metrics.Increment("batches_sent")
	b.metrics.Count("messages_sent", numEncoded)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.metrics.Increment("send_errors")
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			b.enqueueErrResponses(fmt.Errorf("Got HTTP error code but couldn't read response body: %v", err),
				events, dur/time.Duration(numEncoded))
			return
		}
		for _, ev := range events {
			if ev != nil {
				b.enqueueResponse(Response{
					StatusCode: resp.StatusCode,
					Body:       body,
					Duration:   dur / time.Duration(numEncoded),
					Metadata:   ev.Metadata,
				})
			}
		}
		return
	}

	// decode the responses
	batchResponses := []Response{}
	err = json.NewDecoder(resp.Body).Decode(&batchResponses)
	if err != nil {
		// if we can't decode the responses, just error out all of them
		b.metrics.Increment("response_decode_errors")
		b.enqueueErrResponses(err, events, dur/time.Duration(numEncoded))
		return
	}

	// Go through the responses and send them down the queue. If an Event
	// triggered a JSON error, it wasn't sent to the server and won't have a
	// returned response... so we have to be a bit more careful matching up
	// responses with Events.
	var eIdx int
	for _, resp := range batchResponses {
		resp.Duration = dur / time.Duration(numEncoded)
		for events[eIdx] == nil {
			fmt.Printf("incr, eIdx: %d, len(evs): %d\n", eIdx, len(events))
			eIdx++
		}
		if eIdx == len(events) { // just in case
			break
		}
		resp.Metadata = events[eIdx].Metadata
		b.enqueueResponse(resp)
		eIdx++
	}
}

// create the JSON for this event list manually so that we can send
// responses down the response queue for any that fail to marshal
func (b *batchAgg) encodeBatch(events []*Event) ([]byte, int) {
	// track first vs. rest events for commas
	first := true
	// track how many we successfully encode for later bookkeeping
	var numEncoded int
	buf := bytes.Buffer{}
	buf.WriteByte('[')
	bytesTotal := 1
	// ok, we've got our array, let's populate it with JSON events
	for i, ev := range events {
		if !first {
			buf.WriteByte(',')
			bytesTotal++
		}
		first = false
		evByt, err := json.Marshal(ev)
		if err != nil {
			b.enqueueResponse(Response{
				Err:      err,
				Metadata: ev.Metadata,
			})
			// nil out the invalid Event so we can line up sent Events with server
			// responses if needed. don't delete to preserve slice length.
			events[i] = nil
			continue
		}
		// if the event is too large to ever send, add an error to the queue
		if len(evByt) > apiEventSizeMax {
			b.enqueueResponse(Response{
				Err:      fmt.Errorf("event exceeds max event size of %d bytes, API will not accept this event", apiEventSizeMax),
				Metadata: ev.Metadata,
			})
			events[i] = nil
			continue
		}
		bytesTotal += len(evByt)

		// count for the trailing ]
		if bytesTotal+1 > apiMaxBatchSize {
			b.reenqueueEvents(events[i:])
			break
		}
		buf.Write(evByt)
		numEncoded++
	}
	buf.WriteByte(']')
	return buf.Bytes(), numEncoded
}

func (b *batchAgg) enqueueErrResponses(err error, events []*Event, duration time.Duration) {
	for _, ev := range events {
		if ev != nil {
			b.enqueueResponse(Response{
				Err:      err,
				Duration: duration,
				Metadata: ev.Metadata,
			})
		}
	}
}

// buildReqReader returns an io.Reader and a boolean, indicating whether or not
// the io.Reader is gzip-compressed.
func buildReqReader(jsonEncoded []byte, useGzip bool) (io.Reader, bool) {
	if useGzip {
		buf := bytes.Buffer{}
		g := gzip.NewWriter(&buf)
		if _, err := g.Write(jsonEncoded); err == nil {
			if err = g.Close(); err == nil { // flush
				return &buf, true
			}
		}

		return bytes.NewReader(jsonEncoded), false
	}
	return bytes.NewReader(jsonEncoded), false
}

// nower to make testing easier
type nower interface {
	Now() time.Time
}
