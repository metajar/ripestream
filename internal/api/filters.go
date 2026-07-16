package api

import (
	"net/http"

	"ripestream/internal/graph"
)

func probeFilterFromQuery(r *http.Request) graph.ProbeFilter {
	return graph.ProbeFilter{
		ASN:       qInt64(r, "asn"),
		Limit:     qInt(r, "limit", 50),
		MinProbes: qInt64(r, "min_probes"),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
	}
}

func targetFilterFromQuery(r *http.Request) graph.TargetFilter {
	return graph.TargetFilter{
		ASN:       qInt64(r, "asn"),
		Limit:     qInt(r, "limit", 50),
		MinProbes: qInt64(r, "min_probes"),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
	}
}
