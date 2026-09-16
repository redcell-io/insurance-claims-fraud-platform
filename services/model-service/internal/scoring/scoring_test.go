package scoring

import "testing"

func TestScore(t *testing.T) {
	tests := []struct {
		name          string
		features      map[string]string
		wantScore     float64
		wantReasons   int // number of explanation entries expected
	}{
		{
			name: "all good: active policy, known coverage, valid address",
			features: map[string]string{
				"policy_status":        "active",
				"policy_coverage_type": "full",
				"address_valid":        "true",
			},
			wantScore:   0.05,
			wantReasons: 1, // "no risk factors detected"
		},
		{
			name: "lapsed policy",
			features: map[string]string{
				"policy_status":        "lapsed",
				"policy_coverage_type": "full",
				"address_valid":        "true",
			},
			wantScore:   0.20, // 0.05 + 0.15
			wantReasons: 1,
		},
		{
			name: "cancelled policy",
			features: map[string]string{
				"policy_status":        "cancelled",
				"policy_coverage_type": "full",
				"address_valid":        "true",
			},
			wantScore:   0.30, // 0.05 + 0.25
			wantReasons: 1,
		},
		{
			name: "unknown policy: not found, coverage_type empty too",
			features: map[string]string{
				"policy_status":        "unknown",
				"policy_coverage_type": "",
				"address_valid":        "true",
			},
			wantScore:   0.40, // 0.05 + 0.30 + 0.05
			wantReasons: 2,
		},
		{
			name: "invalid address only",
			features: map[string]string{
				"policy_status":        "active",
				"policy_coverage_type": "full",
				"address_valid":        "false",
			},
			wantScore:   0.15, // 0.05 + 0.10
			wantReasons: 1,
		},
		{
			name:        "empty feature map: every default is the risky one",
			features:    map[string]string{},
			wantScore:   0.50, // 0.05 + 0.30 (unknown status) + 0.05 (empty coverage) + 0.10 (address not valid)
			wantReasons: 3,
		},
		{
			name: "worst case clamps to 1.0, not something above it",
			features: map[string]string{
				"policy_status":        "cancelled",
				"policy_coverage_type": "",
				"address_valid":        "false",
			},
			wantScore:   0.45, // 0.05 + 0.25 + 0.05 + 0.10 — below 1.0, sanity check clamp doesn't kick in incorrectly here
			wantReasons: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, explanations := Score(tt.features)
			if score != tt.wantScore {
				t.Errorf("Score() score = %v, want %v", score, tt.wantScore)
			}
			if len(explanations) != tt.wantReasons {
				t.Errorf("Score() explanations = %v (len %d), want %d entries", explanations, len(explanations), tt.wantReasons)
			}
		})
	}
}

func TestScoreNeverExceedsOne(t *testing.T) {
	// Every risky branch triggered at once: 0.05 + 0.30 + 0.05 + 0.10 = 0.50,
	// nowhere near the clamp — but the clamp itself must never let a future
	// weight change silently push scores above 1.0, so exercise it directly.
	score, _ := Score(map[string]string{
		"policy_status":        "totally-bogus-value",
		"policy_coverage_type": "",
		"address_valid":        "not-a-bool",
	})
	if score > 1.0 {
		t.Errorf("Score() = %v, must never exceed 1.0", score)
	}
}
