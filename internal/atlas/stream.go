// Package atlas reads the RIPE Atlas live result stream.
//
// The stream endpoint (https://atlas-stream.ripe.net/api/v2/stream/) emits
// newline-delimited JSON. Each line is a two-element array:
//
//	["atlas_subscribed", {"streamType":"result", ...}]   // ack
//	["atlas_result", { ...measurement result... }]        // data
//	["atlas_error",    {"code":..., "detail":...}]        // non-fatal error
//	["atlas_stop",     {...}]                             // subscription ended
//
// With no filters the stream is a firehose of all public results, mixing every
// measurement type (ping, traceroute, dns, http, ...). A single msm or prb can
// also be subscribed; repeated query parameters are rejected by the server, so
// each filter value gets its own connection.
package atlas

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultBaseURL is the public RIPE Atlas streaming endpoint.
const DefaultBaseURL = "https://atlas-stream.ripe.net/api/v2/stream/"

// Record is the universal projection of an atlas_result plus the verbatim
// payload. JSON tags line up with the ClickHouse atlas_results columns.
type Record struct {
	Timestamp  int64   `json:"timestamp"`
	MsmID      uint32  `json:"msm_id"`
	PrbID      uint32  `json:"prb_id"`
	Type       string  `json:"type"`
	MsmName    string  `json:"msm_name"`
	FromIP     string  `json:"from_ip"`
	AF         uint8   `json:"af"`
	Proto      string  `json:"proto"`
	DstName    string  `json:"dst_name"`
	DstAddr    string  `json:"dst_addr"`
	SrcAddr    string  `json:"src_addr"`
	FW         uint32  `json:"fw"`
	Sent       uint32  `json:"sent"`
	Rcvd       uint32  `json:"rcvd"`
	AvgRttMs   float64 `json:"avg_rtt_ms"`
	MinRttMs   float64 `json:"min_rtt_ms"`
	MaxRttMs   float64 `json:"max_rtt_ms"`
	ResultJSON string  `json:"result_json"`
}

// Params configures the subscription. If both MSM and PRB are empty the reader
// subscribes to the full public firehose.
type Params struct {
	MSM []int // measurement IDs (one connection each)
	PRB []int // probe IDs (one connection each)
}

// Options tunes connection behaviour.
type Options struct {
	BaseURL     string        // stream endpoint (DefaultBaseURL if empty)
	IdleTimeout time.Duration // force reconnect after this long with no data; 0 disables
	UserAgent   string
	Client      *http.Client // overrides the default client if non-nil
}

// payload extracts the universal envelope fields. Everything else is preserved
// via the surrounding json.RawMessage passed to Record.ResultJSON.
type payload struct {
	FW      *uint32 `json:"fw"`
	AF      *uint8  `json:"af"`
	Proto   string  `json:"proto"`
	DstName string  `json:"dst_name"`
	DstAddr string  `json:"dst_addr"`
	SrcAddr string  `json:"src_addr"`
	FromIP  string  `json:"from"`
	Type    string  `json:"type"`
	MsmName string  `json:"msm_name"`
	MsmID   uint32  `json:"msm_id"`
	PrbID   uint32  `json:"prb_id"`
	Ts      int64   `json:"timestamp"`
	Sent    uint32  `json:"sent"`
	Rcvd    uint32  `json:"rcvd"`
	Avg     float64 `json:"avg"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

// Subscribe opens one connection per subscription filter (or a single firehose
// connection when Params is empty) and sends parsed Records on the returned
// channel. The channel is closed once every connection has stopped, which in
// practice happens when ctx is cancelled.
func Subscribe(ctx context.Context, p Params, opts Options) <-chan Record {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "ripestream/0.1 (+https://atlas.ripe.net/)"
	}
	if opts.Client == nil {
		opts.Client = &http.Client{} // no overall timeout: the stream is long-lived
	}

	queries := buildQueries(p)
	out := make(chan Record, 8192)
	var wg sync.WaitGroup
	var total atomic.Int64

	for i := range queries {
		wg.Add(1)
		go func(q url.Values) {
			defer wg.Done()
			n := runSubscription(ctx, opts, q, out)
			if n >= 0 {
				total.Add(n)
			}
		}(queries[i])
	}
	go func() {
		wg.Wait()
		close(out)
		slog.Info("atlas stream stopped", "records_total", total.Load())
	}()
	return out
}

// buildQueries returns one url.Values per connection to open.
func buildQueries(p Params) []url.Values {
	if len(p.MSM) == 0 && len(p.PRB) == 0 {
		return []url.Values{{"streamType": {"result"}}}
	}
	qs := make([]url.Values, 0, len(p.MSM)+len(p.PRB))
	for _, msm := range p.MSM {
		qs = append(qs, url.Values{"streamType": {"result"}, "msm": {strconv.Itoa(msm)}})
	}
	for _, prb := range p.PRB {
		qs = append(qs, url.Values{"streamType": {"result"}, "prb": {strconv.Itoa(prb)}})
	}
	return qs
}

// runSubscription runs one connection with reconnect/backoff until ctx is done.
// Returns the number of Records produced (for the stop log).
func runSubscription(ctx context.Context, opts Options, q url.Values, out chan<- Record) int64 {
	desc := subscriptionDesc(q)
	backoff := time.Second
	const maxBackoff = 60 * time.Second
	var count int64

	for {
		if err := ctx.Err(); err != nil {
			return count
		}
		slog.Info("atlas connecting", "sub", desc)
		n, err := streamOnce(ctx, opts, q, out)
		count += n
		switch {
		case err == nil:
			// Body closed cleanly; reconnect immediately.
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			return count
		default:
			slog.Warn("atlas connection ended, reconnecting", "sub", desc, "err", err, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return count
		case <-time.After(backoff):
		}
		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
	}
}

// subscriptionDesc is a short human label for logs.
func subscriptionDesc(q url.Values) string {
	if v := q.Get("msm"); v != "" {
		return "msm=" + v
	}
	if v := q.Get("prb"); v != "" {
		return "prb=" + v
	}
	return "firehose"
}

// streamOnce opens a single connection and pumps records to out until the
// connection ends (EOF, idle timeout, or ctx cancellation).
func streamOnce(ctx context.Context, opts Options, q url.Values, out chan<- Record) (int64, error) {
	u := opts.BaseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("User-Agent", opts.UserAgent)

	resp, err := opts.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("stream http %d: %s", resp.StatusCode, body)
	}

	// Idle watchdog: low-volume subscriptions (e.g. a single msm) can sit
	// silent for long stretches. Close the body to force a reconnect.
	stopWatchdog := make(chan struct{})
	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixMilli())
	if opts.IdleTimeout > 0 {
		go func() {
			t := time.NewTicker(opts.IdleTimeout / 2)
			defer t.Stop()
			for {
				select {
				case <-stopWatchdog:
					return
				case <-ctx.Done():
					return
				case <-t.C:
					if time.Since(time.UnixMilli(lastRead.Load())) > opts.IdleTimeout {
						slog.Warn("atlas idle timeout, forcing reconnect",
							"sub", subscriptionDesc(q), "idle", opts.IdleTimeout)
						_ = resp.Body.Close()
						return
					}
				}
			}
		}()
	}
	defer close(stopWatchdog)

	r := bufio.NewReaderSize(resp.Body, 1<<20) // 1 MiB; traceroute results can be large
	var count int64
	for {
		lastRead.Store(time.Now().UnixMilli())
		line, rerr := r.ReadString('\n')
		if len(line) > 0 {
			if n, perr := handleLine(line, out, ctx); perr != nil {
				return count, perr // ctx cancelled while sending
			} else {
				count += n
			}
		}
		if rerr != nil {
			if errors.Is(rerr, context.Canceled) || errors.Is(rerr, context.DeadlineExceeded) {
				return count, rerr
			}
			if rerr == io.EOF {
				return count, nil
			}
			return count, rerr
		}
	}
}

// handleLine parses one NDJSON line and, for atlas_result, sends a Record.
// Returns the number of Records sent (0 or 1).
func handleLine(line string, out chan<- Record, ctx context.Context) (int64, error) {
	// Strip surrounding whitespace (including the trailing newline).
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, nil
	}

	var msg [2]json.RawMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		slog.Debug("atlas skipping unparseable line", "err", err, "line", string(line))
		return 0, nil
	}

	var event string
	if err := json.Unmarshal(msg[0], &event); err != nil {
		slog.Debug("atlas skipping line with bad event tag", "err", err)
		return 0, nil
	}

	switch event {
	case "atlas_subscribed":
		slog.Info("atlas subscribed", "ack", string(msg[1]))
		return 0, nil
	case "atlas_error":
		slog.Warn("atlas server error", "payload", string(msg[1]))
		return 0, nil
	case "atlas_stop":
		slog.Info("atlas subscription stopped by server", "payload", string(msg[1]))
		return 0, nil
	case "atlas_result":
		// fall through
	default:
		slog.Debug("atlas unknown event", "event", event, "payload", string(msg[1]))
		return 0, nil
	}

	var pl payload
	if err := json.Unmarshal(msg[1], &pl); err != nil {
		slog.Warn("atlas skipping result with bad payload", "err", err)
		return 0, nil
	}

	rec := Record{
		ResultJSON: string(msg[1]),
		Timestamp:  pl.Ts,
		MsmID:      pl.MsmID,
		PrbID:      pl.PrbID,
		Type:       pl.Type,
		MsmName:    pl.MsmName,
		FromIP:     pl.FromIP,
		AF:         deref8(pl.AF),
		Proto:      pl.Proto,
		DstName:    pl.DstName,
		DstAddr:    pl.DstAddr,
		SrcAddr:    pl.SrcAddr,
		FW:         deref32(pl.FW),
		Sent:       pl.Sent,
		Rcvd:       pl.Rcvd,
		AvgRttMs:   pl.Avg,
		MinRttMs:   pl.Min,
		MaxRttMs:   pl.Max,
	}
	if rec.Timestamp == 0 || rec.MsmID == 0 {
		slog.Debug("atlas result missing identity fields, dropping", "rec", rec)
		return 0, nil
	}

	select {
	case out <- rec:
		return 1, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func deref8(p *uint8) uint8 {
	if p == nil {
		return 0
	}
	return *p
}
func deref32(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}
