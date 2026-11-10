package reasoning

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	EffortLevelLow    = "low"
	EffortLevelMedium = "medium"
	EffortLevelHigh   = "high"

	defaultLowBudgetThreshold  int64 = 8192
	defaultHighBudgetThreshold int64 = 24576
)

// EffortModels maps a single client-facing or group-configured model onto
// effort-tiered upstream model IDs (for example <base>-low, <base>-medium, <base>-high).
type EffortModels struct {
	Low     string `json:"low,omitempty"`
	Medium  string `json:"medium,omitempty"`
	High    string `json:"high,omitempty"`
	Default string `json:"default,omitempty"`
}

func (models *EffortModels) IsEmpty() bool {
	if models == nil {
		return true
	}
	return strings.TrimSpace(models.Low) == "" &&
		strings.TrimSpace(models.Medium) == "" &&
		strings.TrimSpace(models.High) == ""
}

func (models *EffortModels) Clone() *EffortModels {
	if models.IsEmpty() {
		return nil
	}
	cloned := *models
	return &cloned
}

func (models *EffortModels) Equal(other *EffortModels) bool {
	if models.IsEmpty() && other.IsEmpty() {
		return true
	}
	if models.IsEmpty() != other.IsEmpty() {
		return false
	}
	return models.Low == other.Low &&
		models.Medium == other.Medium &&
		models.High == other.High &&
		models.Default == other.Default
}

func (models *EffortModels) Normalize() (*EffortModels, error) {
	if models == nil {
		return nil, nil
	}
	low, err := normalizeEffortTargetModel(models.Low)
	if err != nil {
		return nil, fmt.Errorf("low effort model: %w", err)
	}
	medium, err := normalizeEffortTargetModel(models.Medium)
	if err != nil {
		return nil, fmt.Errorf("medium effort model: %w", err)
	}
	high, err := normalizeEffortTargetModel(models.High)
	if err != nil {
		return nil, fmt.Errorf("high effort model: %w", err)
	}
	defaultLevel := strings.ToLower(strings.TrimSpace(models.Default))
	if low == "" && medium == "" && high == "" {
		if defaultLevel != "" {
			return nil, fmt.Errorf("default effort level requires at least one effort model")
		}
		return nil, nil
	}
	normalized := &EffortModels{
		Low:    low,
		Medium: medium,
		High:   high,
	}
	if defaultLevel == "" {
		switch {
		case medium != "":
			defaultLevel = EffortLevelMedium
		case high != "":
			defaultLevel = EffortLevelHigh
		default:
			defaultLevel = EffortLevelLow
		}
	}
	switch defaultLevel {
	case EffortLevelLow:
		if low == "" {
			return nil, fmt.Errorf("default effort level %q has no model configured", defaultLevel)
		}
	case EffortLevelMedium:
		if medium == "" {
			return nil, fmt.Errorf("default effort level %q has no model configured", defaultLevel)
		}
	case EffortLevelHigh:
		if high == "" {
			return nil, fmt.Errorf("default effort level %q has no model configured", defaultLevel)
		}
	default:
		return nil, fmt.Errorf("unsupported default effort level %q", defaultLevel)
	}
	normalized.Default = defaultLevel
	return normalized, nil
}

func (models *EffortModels) ModelForLevel(level string) string {
	if models == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(level)) {
	case EffortLevelLow:
		return strings.TrimSpace(models.Low)
	case EffortLevelMedium:
		return strings.TrimSpace(models.Medium)
	case EffortLevelHigh:
		return strings.TrimSpace(models.High)
	default:
		return ""
	}
}

// Models returns the deduplicated non-empty upstream model IDs in low, medium, high order.
func (models *EffortModels) Models() []string {
	if models.IsEmpty() {
		return nil
	}
	result := make([]string, 0, 3)
	for _, candidate := range []string{models.Low, models.Medium, models.High} {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing == trimmed {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, trimmed)
		}
	}
	return result
}

// Resolve selects the concrete upstream model ID and effort level for a request's reasoning config.
func (models *EffortModels) Resolve(config Config) (targetModel string, level string, ok bool) {
	if models.IsEmpty() {
		return "", "", false
	}
	desired := models.desiredLevel(config)
	if target := models.ModelForLevel(desired); target != "" {
		return target, desired, true
	}
	for _, fallback := range []string{models.Default, EffortLevelMedium, EffortLevelHigh, EffortLevelLow} {
		if target := models.ModelForLevel(fallback); target != "" {
			return target, strings.ToLower(strings.TrimSpace(fallback)), true
		}
	}
	return "", "", false
}

func (models *EffortModels) desiredLevel(config Config) string {
	if effort := strings.ToLower(strings.TrimSpace(config.Effort)); effort != "" {
		switch effort {
		case "minimal", "none", EffortLevelLow:
			return EffortLevelLow
		case EffortLevelMedium:
			return EffortLevelMedium
		case EffortLevelHigh, "xhigh", "max", "ultra":
			return EffortLevelHigh
		}
	}
	if config.BudgetTokens != nil && *config.BudgetTokens > 0 {
		switch {
		case *config.BudgetTokens < defaultLowBudgetThreshold:
			return EffortLevelLow
		case *config.BudgetTokens < defaultHighBudgetThreshold:
			return EffortLevelMedium
		default:
			return EffortLevelHigh
		}
	}
	switch strings.ToLower(strings.TrimSpace(config.Mode)) {
	case "disabled", "none", "off":
		return EffortLevelLow
	}
	if def := strings.ToLower(strings.TrimSpace(models.Default)); def != "" {
		return def
	}
	return EffortLevelMedium
}

func normalizeEffortTargetModel(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("model id contains control character")
		}
	}
	return trimmed, nil
}
