package graph

import (
	"context"
	"fmt"
	"strings"
)

// EvaluateAlert runs a rule's metric/scope/filters against the graph and returns
// one row per scope instance (e.g. per probe-target pair, per AS pair). It does
// NOT apply the threshold comparison — that's the evaluator's job — but it does
// pre-filter rows so we don't ship the whole graph.
//
// The query is built per (scope, metric) because FalkorDB Cypher's structure
// differs by grouping. Filters are injected as WHERE predicates.
func (s *Store) EvaluateAlert(ctx context.Context, rule AlertRuleSpec) ([]AlertScopeRow, error) {
	q, err := buildAlertCypher(rule)
	if err != nil {
		return nil, err
	}
	if q.cypher == "" {
		return nil, nil
	}
	q.params["cutoff"] = s.activeCutoff()
	rows, err := s.rows(ctx, q.cypher, q.params)
	if err != nil {
		return nil, err
	}
	out := make([]AlertScopeRow, 0, len(rows))
	for _, r := range rows {
		ctx := map[string]any{}
		// Carry through any returned context columns.
		for k, v := range r {
			if k == "scope_key" || k == "value" {
				continue
			}
			ctx[k] = v
		}
		out = append(out, AlertScopeRow{
			ScopeKey: asString(r["scope_key"]),
			Value:    asFloat(r["value"]),
			Context:  ctx,
		})
	}
	return out, nil
}

type builtQuery struct {
	cypher string
	params map[string]any
}

// metricExpr returns the FalkorDB scalar expression for a given metric. Returns
// ("", false) for target_lost, which is handled as a WHERE rcvd==0 filter.
func metricExpr(metric string) (expr string, aggregate bool) {
	switch metric {
	case "loss_ratio":
		// aggregate avg over the scope's PING edges
		return "avg(e.loss_ratio)", true
	case "avg_rtt_ms":
		return "avg(e.avg_rtt_ms)", true
	case "target_lost":
		// not a numeric aggregate; treated as a boolean condition on rcvd==0
		return "", false
	}
	return "", false
}

// buildAlertCypher constructs the metric+scope+filter Cypher. Returns an empty
// builtQuery (no error) when the rule's combination is a no-op (no data).
func buildAlertCypher(rule AlertRuleSpec) (builtQuery, error) {
	metricExprStr, _ := metricExpr(rule.Metric)

	// WHERE fragments shared across scopes.
	var where []string
	params := map[string]any{
		"minSent": int64(rule.MinSent),
		"minLoss": rule.MinLoss,
	}
	if rule.MinSent > 0 {
		where = append(where, "e.sent >= $minSent")
	} else {
		// Always require some traffic to avoid dividing by zero.
		where = append(where, "e.sent > 0")
	}
	// Alerts describe live state; retained historical edges must never keep an
	// alert firing after its evidence has gone stale.
	where = append(where, "e.last_seen >= $cutoff")
	if rule.Metric == "target_lost" {
		where = append(where, "e.rcvd = 0")
	} else if rule.Metric == "loss_ratio" {
		// Healthy edges must remain in the denominator or a single lossy probe
		// turns a mostly healthy scope into an apparent 100% incident.
		if rule.MinLoss > 0 {
			where = append(where, "e.loss_ratio >= $minLoss")
		}
	} else if rule.MinLoss > 0 {
		where = append(where, "e.loss_ratio >= $minLoss")
	}

	var q strings.Builder
	q.WriteString("MATCH ")
	switch rule.Scope {
	case "probe_target":
		q.WriteString("(p:Probe)-[e:PING]->(t:IP)\n")
		if rule.ProbeID > 0 {
			params["probe"] = rule.ProbeID
			where = append(where, "p.id = $probe")
		}
		if rule.TargetIP != "" {
			params["target"] = rule.TargetIP
			where = append(where, "t.addr = $target")
		}
	case "asn_pair":
		q.WriteString("(p:Probe)-[:LOCATED_AT]->(src:IP)-[:IN_AS]->(srcAS:AS)\n")
		q.WriteString("MATCH (p)-[e:PING]->(t:IP)-[:IN_AS]->(dstAS:AS)\n")
		if rule.SrcASN > 0 {
			params["srcAsn"] = rule.SrcASN
			where = append(where, "srcAS.asn = $srcAsn")
		}
		if rule.DstASN > 0 {
			params["dstAsn"] = rule.DstASN
			where = append(where, "dstAS.asn = $dstAsn")
		}
	case "target":
		q.WriteString("(p:Probe)-[e:PING]->(t:IP)\n")
		if rule.TargetIP != "" {
			params["target"] = rule.TargetIP
			where = append(where, "t.addr = $target")
		}
	case "asn_dst":
		q.WriteString("(p:Probe)-[e:PING]->(t:IP)-[:IN_AS]->(dstAS:AS)\n")
		if rule.DstASN > 0 {
			params["dstAsn"] = rule.DstASN
			where = append(where, "dstAS.asn = $dstAsn")
		}
	default:
		return builtQuery{}, fmt.Errorf("unsupported scope %q", rule.Scope)
	}

	// Apply WHERE.
	q.WriteString("WHERE " + strings.Join(where, " AND ") + "\n")

	// GROUP BY + RETURN.
	switch rule.Scope {
	case "probe_target":
		q.WriteString("WITH p.id AS probe_id, t.addr AS target, (1.0 * (sum(e.sent) - sum(e.rcvd)) / sum(e.sent)) AS avg_loss,\n")
		q.WriteString("     max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt, count(*) AS samples\n")
		q.WriteString("RETURN toString(probe_id) + '→' + target AS scope_key,\n")
		q.WriteString(formatMetricReturn(rule.Metric) + " AS value,\n")
		q.WriteString("probe_id AS probe_id, target AS target, samples AS samples, avg_loss AS avg_loss, avg_rtt AS avg_rtt\n")
		q.WriteString("ORDER BY value DESC LIMIT 200")
	case "asn_pair":
		q.WriteString("WITH srcAS.asn AS src_asn, srcAS.org AS src_org, dstAS.asn AS dst_asn, dstAS.org AS dst_org,\n")
		q.WriteString("     (1.0 * (sum(e.sent) - sum(e.rcvd)) / sum(e.sent)) AS avg_loss, max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt,\n")
		q.WriteString("     count(*) AS samples, count(DISTINCT p.id) AS probes\n")
		q.WriteString("WHERE probes >= 2\n")
		q.WriteString("RETURN toString(src_asn) + '→' + toString(dst_asn) AS scope_key,\n")
		q.WriteString(formatMetricReturn(rule.Metric) + " AS value,\n")
		q.WriteString("src_asn AS src_asn, src_org AS src_org, dst_asn AS dst_asn, dst_org AS dst_org,\n")
		q.WriteString("samples AS samples, probes AS probes, avg_loss AS avg_loss, avg_rtt AS avg_rtt\n")
		q.WriteString("ORDER BY value DESC LIMIT 200")
	case "target":
		q.WriteString("WITH t.addr AS target, (1.0 * (sum(e.sent) - sum(e.rcvd)) / sum(e.sent)) AS avg_loss, max(e.loss_ratio) AS max_loss,\n")
		q.WriteString("     avg(e.avg_rtt_ms) AS avg_rtt, count(DISTINCT p.id) AS probes, count(*) AS samples\n")
		q.WriteString("WHERE probes >= 3\n")
		q.WriteString("RETURN target AS scope_key,\n")
		q.WriteString(formatMetricReturn(rule.Metric) + " AS value,\n")
		q.WriteString("target AS target, samples AS samples, probes AS probes, avg_loss AS avg_loss, avg_rtt AS avg_rtt\n")
		q.WriteString("ORDER BY value DESC LIMIT 200")
	case "asn_dst":
		q.WriteString("WITH dstAS.asn AS dst_asn, dstAS.org AS dst_org, (1.0 * (sum(e.sent) - sum(e.rcvd)) / sum(e.sent)) AS avg_loss,\n")
		q.WriteString("     max(e.loss_ratio) AS max_loss, avg(e.avg_rtt_ms) AS avg_rtt,\n")
		q.WriteString("     count(DISTINCT p.id) AS probes, count(DISTINCT p.source_asn) AS source_ases, count(*) AS samples\n")
		q.WriteString("WHERE probes >= 3 AND source_ases >= 2\n")
		q.WriteString("RETURN toString(dst_asn) AS scope_key,\n")
		q.WriteString(formatMetricReturn(rule.Metric) + " AS value,\n")
		q.WriteString("dst_asn AS dst_asn, dst_org AS dst_org, samples AS samples, probes AS probes, source_ases AS source_ases,\n")
		q.WriteString("avg_loss AS avg_loss, avg_rtt AS avg_rtt\n")
		q.WriteString("ORDER BY value DESC LIMIT 200")
	}

	_ = metricExprStr
	return builtQuery{cypher: q.String(), params: params}, nil
}

// formatMetricReturn maps a rule metric to the RETURN expression for `value`.
// target_lost has no numeric aggregate; we report 1.0 (100% loss) since the
// WHERE already restricts to rcvd==0 edges.
func formatMetricReturn(metric string) string {
	switch metric {
	case "loss_ratio":
		return "round(100.0 * avg_loss)"
	case "avg_rtt_ms":
		return "round(avg_rtt)"
	case "target_lost":
		return "100.0"
	}
	return "0"
}
