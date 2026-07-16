package alert

import (
	"testing"
)

// TestRuleValidate_Units verifies the public threshold-unit convention:
// loss_ratio and target_lost use percentages 0–100; avg_rtt_ms uses ms >= 0.
func TestRuleValidate_Units(t *testing.T) {
	valid := Rule{
		Name: "x", Metric: MetricLossRatio, Comparison: CmpGTE,
		Threshold: 50, Scope: ScopeASNDst, Enabled: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid loss rule (50%%): unexpected error: %v", err)
	}

	cases := []struct {
		name    string
		rule    Rule
		wantErr string
	}{
		{
			name:    "loss threshold over 100",
			rule:    Rule{Name: "x", Metric: MetricLossRatio, Comparison: CmpGTE, Threshold: 150, Scope: ScopeASNDst},
			wantErr: "percentage 0–100",
		},
		{
			name:    "loss threshold negative",
			rule:    Rule{Name: "x", Metric: MetricLossRatio, Comparison: CmpGTE, Threshold: -5, Scope: ScopeASNDst},
			wantErr: "percentage 0–100",
		},
		{
			name:    "rtt threshold negative",
			rule:    Rule{Name: "x", Metric: MetricAvgRTT, Comparison: CmpGTE, Threshold: -1, Scope: ScopeTarget},
			wantErr: ">= 0 ms",
		},
		{
			name:    "rtt threshold valid",
			rule:    Rule{Name: "x", Metric: MetricAvgRTT, Comparison: CmpGT, Threshold: 250, Scope: ScopeTarget},
			wantErr: "",
		},
		{
			name:    "target_lost valid at 100",
			rule:    Rule{Name: "x", Metric: MetricTargetLost, Comparison: CmpGTE, Threshold: 100, Scope: ScopeASNDst},
			wantErr: "",
		},
		{
			name:    "empty name",
			rule:    Rule{Name: " ", Metric: MetricLossRatio, Comparison: CmpGTE, Threshold: 50, Scope: ScopeASNDst},
			wantErr: "name is required",
		},
		{
			name:    "invalid metric",
			rule:    Rule{Name: "x", Metric: Metric("bogus"), Comparison: CmpGTE, Threshold: 50, Scope: ScopeASNDst},
			wantErr: "invalid metric",
		},
		{
			name:    "invalid scope",
			rule:    Rule{Name: "x", Metric: MetricLossRatio, Comparison: CmpGTE, Threshold: 50, Scope: Scope("nope")},
			wantErr: "invalid scope",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.rule.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", c.wantErr)
				return
			}
			if !contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestCompare verifies the evaluator's threshold comparison with percentage
// values (the unit the evaluator receives from the graph boundary).
func TestCompare(t *testing.T) {
	cases := []struct {
		cmp   Comparison
		val   float64
		thresh float64
		want  bool
	}{
		{CmpGTE, 92, 80, true},   // 92% >= 80% -> fire
		{CmpGTE, 50, 80, false},  // 50% >= 80% -> no
		{CmpGT, 80, 80, false},   // strictly greater
		{CmpGT, 81, 80, true},
		{CmpLT, 10, 20, true},    // recovery: 10% < 20%
		{CmpLTE, 20, 20, true},
		{CmpEQ, 100, 100, true},  // target_lost convention
	}
	for _, c := range cases {
		got := compare(c.cmp, c.val, c.thresh)
		if got != c.want {
			t.Errorf("compare(%s, %g, %g) = %v, want %v", c.cmp, c.val, c.thresh, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
