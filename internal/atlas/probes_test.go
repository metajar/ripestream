package atlas

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProbeClientFetchesBatchAndDerivesMetadata(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("id__in"); got != "7,9" {
			t.Errorf("id__in = %q, want 7,9", got)
		}
		if got := r.URL.Query().Get("format[datetime]"); got != "unix" {
			t.Errorf("format[datetime] = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"results":[{"id":7,"description":"Edge lab","country_code":"nl","is_anchor":false,"is_public":true,"firmware_version":5080,"status_since":100,"first_connected":10,"last_connected":200,"asn_v4":64500,"geometry":{"coordinates":[4.9,52.3]},"status":{"id":1,"name":"Connected"},"tags":[{"slug":"system-v5"},{"slug":"fibre"}]},{"id":9,"description":null,"country_code":"US","is_anchor":true,"is_public":true,"status":{"id":1,"name":"Connected"},"tags":[{"slug":"system-anchor"},{"slug":"system-virtual"}]}]}`)),
		}, nil
	})

	client := ProbeClient{BaseURL: "https://atlas.example/probes/", Client: &http.Client{Transport: transport}}
	got, err := client.Fetch(context.Background(), []int64{9, 7, 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("metadata count = %d, want 2", len(got))
	}
	if got[0].DisplayName != "Edge lab" || got[0].ProbeType != "Hardware v5" || got[0].CountryCode != "NL" {
		t.Fatalf("probe 7 = %#v", got[0])
	}
	if got[1].DisplayName != "Virtual anchor in US" || got[1].ProbeType != "Virtual anchor" {
		t.Fatalf("probe 9 = %#v", got[1])
	}
}
