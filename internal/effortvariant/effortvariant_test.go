package effortvariant

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gpt-load/internal/reasoning"
)

const sample = `[{"name":"claude-opus-5-5-ag","levels":{"low":"claude-opus-5-5-low","medium":"claude-opus-5-5-medium","high":"claude-opus-5-5-high"}}]`

func budget(v int64) *int64 { return &v }

func TestResolve(t *testing.T) {
	table, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		config reasoning.Config
		want   string
	}{
		{"no params uses default", reasoning.Config{}, "claude-opus-5-5-medium"},
		{"effort low", reasoning.Config{Effort: "low"}, "claude-opus-5-5-low"},
		{"effort case-insensitive", reasoning.Config{Effort: "HIGH"}, "claude-opus-5-5-high"},
		{"minimal -> weakest", reasoning.Config{Effort: "minimal"}, "claude-opus-5-5-low"},
		{"xhigh -> strongest", reasoning.Config{Effort: "xhigh"}, "claude-opus-5-5-high"},
		{"max -> strongest", reasoning.Config{Effort: "max"}, "claude-opus-5-5-high"},
		{"unknown effort falls to default", reasoning.Config{Effort: "weird"}, "claude-opus-5-5-medium"},
		{"budget below 8k", reasoning.Config{Mode: "enabled", BudgetTokens: budget(4000)}, "claude-opus-5-5-low"},
		{"budget at 8k", reasoning.Config{BudgetTokens: budget(8192)}, "claude-opus-5-5-medium"},
		{"budget below 24k", reasoning.Config{BudgetTokens: budget(24575)}, "claude-opus-5-5-medium"},
		{"budget at 24k", reasoning.Config{BudgetTokens: budget(24576)}, "claude-opus-5-5-high"},
		{"effort beats budget", reasoning.Config{Effort: "low", BudgetTokens: budget(50000)}, "claude-opus-5-5-low"},
		{"thinking disabled -> weakest", reasoning.Config{Mode: "disabled"}, "claude-opus-5-5-low"},
		{"adaptive -> default", reasoning.Config{Mode: "adaptive"}, "claude-opus-5-5-medium"},
	}
	for _, tc := range cases {
		got, _, ok := table.Resolve("claude-opus-5-5-ag", tc.config)
		if !ok || got != tc.want {
			t.Errorf("%s: got %q ok=%v want %q", tc.name, got, ok, tc.want)
		}
	}
	if _, _, ok := table.Resolve("claude-opus-5-5-high", reasoning.Config{}); ok {
		t.Error("upstream names must not be treated as variants")
	}
	var nilTable *Table
	if _, _, ok := nilTable.Resolve("x", reasoning.Config{}); ok {
		t.Error("nil table must not resolve")
	}
}

func TestParseRejectsBadConfig(t *testing.T) {
	bad := map[string]string{
		"no name":         `[{"levels":{"low":"a"}}]`,
		"no levels":       `[{"name":"x"}]`,
		"self target":     `[{"name":"x","levels":{"low":"x"}}]`,
		"bad default":     `[{"name":"x","levels":{"low":"a"},"default":"high"}]`,
		"order unknown":   `[{"name":"x","levels":{"low":"a"},"order":["low","high"]}]`,
		"duplicate":       `[{"name":"x","levels":{"low":"a"},"default":"low"},{"name":"x","levels":{"low":"b"},"default":"low"}]`,
		"chained":         `[{"name":"x","levels":{"low":"y"},"default":"low"},{"name":"y","levels":{"low":"b"},"default":"low"}]`,
		"threshold count": `[{"name":"x","levels":{"low":"a","high":"b"},"order":["low","high"],"default":"low","budget_thresholds":[1,2]}]`,
		"not ascending":   `[{"name":"x","levels":{"a":"1","b":"2","c":"3"},"order":["a","b","c"],"default":"a","budget_thresholds":[9,3]}]`,
		"not json":        `nope`,
	}
	for name, input := range bad {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestCustomOrderWithoutBudgetIgnoresBudget(t *testing.T) {
	table, err := Parse([]byte(`[{"name":"x","levels":{"fast":"a","deep":"b"},"order":["fast","deep"],"default":"fast"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got, _, _ := table.Resolve("x", reasoning.Config{BudgetTokens: budget(99999)}); got != "a" {
		t.Errorf("got %q want default a", got)
	}
}

func TestSourceReloadKeepsLastGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "effort-variants.json")
	clock := time.Unix(1000, 0)
	source := NewSource(path, nil)
	source.now = func() time.Time { return clock }

	if source.Table() != nil {
		t.Fatal("missing file must mean feature off")
	}
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(10 * time.Second)
	if source.Table() == nil {
		t.Fatal("expected table after file appears")
	}
	if err := os.WriteFile(path, []byte(`[{"name":"broken"`), 0o600); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(10 * time.Second)
	if source.Table() == nil {
		t.Fatal("invalid edit must keep the last good table")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(10 * time.Second)
	if source.Table() != nil {
		t.Fatal("removed file must disable the feature")
	}
}
