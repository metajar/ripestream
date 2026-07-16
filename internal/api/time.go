package api

import "time"

// parseISOTime parses an RFC3339 timestamp (as emitted by the alert store).
func parseISOTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
