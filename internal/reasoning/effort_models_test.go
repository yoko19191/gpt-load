package reasoning

import "testing"

func TestEffortModelsNormalizeAndResolve(t *testing.T) {
	t.Parallel()

	raw := &EffortModels{
		Low:    " claude-opus-5-5-low ",
		Medium: " claude-opus-5-5-medium ",
		High:   " claude-opus-5-5-high ",
	}
	normalized, err := raw.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if normalized.Default != EffortLevelMedium {
		t.Fatalf("Default = %q, want %q", normalized.Default, EffortLevelMedium)
	}

	budget4k := int64(4096)
	budget16k := int64(16000)
	budget32k := int64(32000)

	cases := []struct {
		name       string
		config     Config
		wantModel  string
		wantLevel  string
	}{
		{
			name:      "default when empty",
			config:    Config{},
			wantModel: "claude-opus-5-5-medium",
			wantLevel: "medium",
		},
		{
			name:      "explicit high effort",
			config:    Config{Effort: "high"},
			wantModel: "claude-opus-5-5-high",
			wantLevel: "high",
		},
		{
			name:      "xhigh clamps to high",
			config:    Config{Effort: "xhigh"},
			wantModel: "claude-opus-5-5-high",
			wantLevel: "high",
		},
		{
			name:      "minimal clamps to low",
			config:    Config{Effort: "minimal"},
			wantModel: "claude-opus-5-5-low",
			wantLevel: "low",
		},
		{
			name:      "budget 4096 -> low",
			config:    Config{Mode: "enabled", BudgetTokens: &budget4k},
			wantModel: "claude-opus-5-5-low",
			wantLevel: "low",
		},
		{
			name:      "budget 16000 -> medium",
			config:    Config{Mode: "enabled", BudgetTokens: &budget16k},
			wantModel: "claude-opus-5-5-medium",
			wantLevel: "medium",
		},
		{
			name:      "budget 32000 -> high",
			config:    Config{Mode: "enabled", BudgetTokens: &budget32k},
			wantModel: "claude-opus-5-5-high",
			wantLevel: "high",
		},
		{
			name:      "disabled mode -> low",
			config:    Config{Mode: "disabled"},
			wantModel: "claude-opus-5-5-low",
			wantLevel: "low",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotModel, gotLevel, ok := normalized.Resolve(tc.config)
			if !ok || gotModel != tc.wantModel || gotLevel != tc.wantLevel {
				t.Fatalf("Resolve(%+v) = (%q, %q, %v), want (%q, %q, true)",
					tc.config, gotModel, gotLevel, ok, tc.wantModel, tc.wantLevel)
			}
		})
	}
}

func TestEffortModelsPartialSlotsFallbackToDefault(t *testing.T) {
	t.Parallel()

	raw := &EffortModels{
		Low:     "gemini-3.8-flash-low",
		High:    "gemini-3.8-flash-high",
		Default: "high",
	}
	normalized, err := raw.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	gotModel, gotLevel, ok := normalized.Resolve(Config{Effort: "medium"})
	if !ok || gotModel != "gemini-3.8-flash-high" || gotLevel != "high" {
		t.Fatalf("Resolve(medium) = (%q, %q, %v), want (gemini-3.8-flash-high, high, true)", gotModel, gotLevel, ok)
	}
}
