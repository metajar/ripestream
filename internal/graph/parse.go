package graph

import (
	"encoding/json"

	"ripestream/internal/atlas"
)

// traceroutePayload is the subset of an Atlas traceroute result we need.
type traceroutePayload struct {
	Result []tracerouteHop `json:"result"`
}

type tracerouteHop struct {
	Hop    int                     `json:"hop"`
	Result []tracerouteHopResponse `json:"result"`
}

type tracerouteHopResponse struct {
	From string  `json:"from"`
	RTT  float64 `json:"rtt"`
	X    string  `json:"x"` // "*" on timeout
}

// pingPayload is the subset of an Atlas ping result we need.
type pingPayload struct {
	Avg  float64 `json:"avg"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Sent uint32  `json:"sent"`
	Rcvd uint32  `json:"rcvd"`
}

// hopIP is one responsive hop with an aggregated RTT and optional ASN.
type hopIP struct {
	Addr string
	RTT  float64
	ASN  int64
	Org  string
}

// extractResponsiveHops returns the responsive hop IPs in order, skipping
// timeout hops. Consecutive NEXT_HOP edges are formed across skipped timeouts.
func extractResponsiveHops(raw string) ([]hopIP, error) {
	var pl traceroutePayload
	if err := json.Unmarshal([]byte(raw), &pl); err != nil {
		return nil, err
	}
	out := make([]hopIP, 0, len(pl.Result))
	for _, hop := range pl.Result {
		addr, rtt, ok := hopBestReply(hop.Result)
		if !ok {
			continue
		}
		out = append(out, hopIP{Addr: addr, RTT: rtt})
	}
	return out, nil
}

// hopBestReply picks the first responsive reply and averages RTTs for that IP.
func hopBestReply(replies []tracerouteHopResponse) (addr string, avgRTT float64, ok bool) {
	var sum float64
	var n int
	for _, r := range replies {
		if r.From == "" || r.X == "*" {
			continue
		}
		if addr == "" {
			addr = r.From
		}
		if r.From != addr {
			continue
		}
		sum += r.RTT
		n++
	}
	if addr == "" || n == 0 {
		return "", 0, false
	}
	return addr, sum / float64(n), true
}

func parsePing(raw string) (pingPayload, error) {
	var pl pingPayload
	err := json.Unmarshal([]byte(raw), &pl)
	return pl, err
}

func lossRatio(sent, rcvd uint32) float64 {
	if sent == 0 {
		return 0
	}
	if rcvd >= sent {
		return 0
	}
	return float64(sent-rcvd) / float64(sent)
}

// isGraphType reports whether the record should update the FalkorDB graph.
func isGraphType(rec atlas.Record) bool {
	switch rec.Type {
	case "traceroute", "ping":
		return true
	default:
		return false
	}
}
