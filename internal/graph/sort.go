package graph

import "fmt"

// sortDir renders a validated ORDER BY direction. Defaults to DESC (worst-first).
// Only "asc"/"desc" are accepted; everything else becomes desc.
func sortDir(order string) string {
	if order == "asc" {
		return "ASC"
	}
	return "DESC"
}

// asnIssueSortExpr maps a validated sort key to the Cypher expression used in
// ORDER BY. The expressions reference aliased WITH variables produced by the
// ASNIssues queries (avg_loss, samples, probes, last_seen).
//
// "impact" is avg_loss × distinct_probe_count — a transparent ranking that
// weights breadth of observation. It is computed in the WITH clause as
// `impact` and ordered here. The caller must include it in the WITH.
func asnIssueSortExpr(sort string) (expr string, ok bool) {
	switch sort {
	case "loss":
		return "avg_loss", true
	case "probes":
		return "probes", true
	case "samples":
		return "samples", true
	case "last_seen":
		return "last_seen", true
	default:
		return "", false
	}
}

// probeSortExpr maps sort keys for the probe list.
func probeSortExpr(sort string) (expr string, ok bool) {
	switch sort {
	case "loss":
		return "avg_loss", true
	case "rtt":
		return "avg_rtt", true
	case "last_seen":
		return "last_seen", true
	default:
		return "", false
	}
}

// targetSortExpr maps sort keys for the target list.
func targetSortExpr(sort string) (expr string, ok bool) {
	switch sort {
	case "loss":
		return "avg_loss", true
	case "rtt":
		return "avg_rtt", true
	case "probes":
		return "probes", true
	case "last_seen":
		return "last_seen", true
	case "impact":
		return "impact", true
	default:
		return "", false
	}
}

func hopSortExpr(sort string) (expr string, ok bool) {
	switch sort {
	case "rtt":
		return "rtt", true
	case "observations":
		return "sc", true
	case "last_seen":
		return "ls", true
	default:
		return "", false
	}
}

func transitSortExpr(sort string) (expr string, ok bool) {
	switch sort {
	case "observations":
		return "sc", true
	case "last_seen":
		return "ls", true
	default:
		return "", false
	}
}

// validatedSort picks a valid sort expression or falls back to defExpr. This
// guarantees untrusted query input can never reach the Cypher ORDER BY clause.
func validatedSort(sort string, defExpr string, pick func(string) (string, bool)) string {
	if expr, ok := pick(sort); ok {
		return expr
	}
	return defExpr
}

// ErrInvalidSort is returned when a sort key is not in the whitelist.
type ErrInvalidSort struct{ Sort string }

func (e *ErrInvalidSort) Error() string {
	return fmt.Sprintf("invalid sort %q", e.Sort)
}
