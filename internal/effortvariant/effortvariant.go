// Package effortvariant maps one client-facing model name onto a family of
// upstream models that differ only by reasoning effort (for example
// claude-opus-5-5-low/-medium/-high), choosing the member from the reasoning
// parameters the client sent (effort, thinking budget, thinking mode).
package effortvariant

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gpt-load/internal/reasoning"
)

const (
	// EnvFile overrides the config file location.
	EnvFile = "EFFORT_VARIANTS_FILE"
	// DefaultFileName is looked up inside DATA_DIR when EnvFile is unset.
	DefaultFileName = "effort-variants.json"

	reloadInterval = 5 * time.Second
)

var defaultOrder = []string{"low", "medium", "high"}

// defaultThresholds are used with the default three-level order:
// budget < 8192 -> low, < 24576 -> medium, otherwise high.
var defaultThresholds = []int64{8192, 24576}

// Variant is one client-facing model name backed by several upstream models.
type Variant struct {
	// Name is the client-facing model name.
	Name string `json:"name"`
	// Levels maps an effort level to the upstream client model that serves it.
	Levels map[string]string `json:"levels"`
	// Order lists levels from weakest to strongest. Defaults to low, medium, high.
	Order []string `json:"order,omitempty"`
	// Default is used when the request carries no usable reasoning parameter.
	Default string `json:"default"`
	// BudgetThresholds maps thinking budget tokens to Order positions: the
	// level index is the number of thresholds that are <= the budget. It must
	// hold len(Order)-1 ascending values; defaults exist for the default order.
	BudgetThresholds []int64 `json:"budget_thresholds,omitempty"`
}

func (variant *Variant) normalize() error {
	variant.Name = strings.TrimSpace(variant.Name)
	if variant.Name == "" {
		return errors.New("name is required")
	}
	if len(variant.Levels) == 0 {
		return fmt.Errorf("%s: levels is required", variant.Name)
	}
	levels := make(map[string]string, len(variant.Levels))
	for level, target := range variant.Levels {
		level, target = strings.ToLower(strings.TrimSpace(level)), strings.TrimSpace(target)
		if level == "" || target == "" {
			return fmt.Errorf("%s: empty level or target", variant.Name)
		}
		if target == variant.Name {
			return fmt.Errorf("%s: level %q points at the variant itself", variant.Name, level)
		}
		levels[level] = target
	}
	variant.Levels = levels
	if len(variant.Order) == 0 {
		variant.Order = append([]string(nil), defaultOrder...)
	}
	order := make([]string, 0, len(variant.Order))
	for _, level := range variant.Order {
		level = strings.ToLower(strings.TrimSpace(level))
		if _, ok := levels[level]; !ok {
			return fmt.Errorf("%s: order level %q has no entry in levels", variant.Name, level)
		}
		order = append(order, level)
	}
	variant.Order = order
	variant.Default = strings.ToLower(strings.TrimSpace(variant.Default))
	if variant.Default == "" {
		variant.Default = order[len(order)/2]
	}
	if _, ok := levels[variant.Default]; !ok {
		return fmt.Errorf("%s: default %q has no entry in levels", variant.Name, variant.Default)
	}
	if len(variant.BudgetThresholds) == 0 && len(order) == len(defaultOrder) && order[0] == defaultOrder[0] &&
		order[1] == defaultOrder[1] && order[2] == defaultOrder[2] {
		variant.BudgetThresholds = append([]int64(nil), defaultThresholds...)
	}
	if len(variant.BudgetThresholds) != 0 {
		if len(variant.BudgetThresholds) != len(order)-1 {
			return fmt.Errorf("%s: budget_thresholds needs %d values", variant.Name, len(order)-1)
		}
		for i := 1; i < len(variant.BudgetThresholds); i++ {
			if variant.BudgetThresholds[i] <= variant.BudgetThresholds[i-1] {
				return fmt.Errorf("%s: budget_thresholds must be ascending", variant.Name)
			}
		}
	}
	return nil
}

// levelFor picks the effort level for a request's reasoning parameters.
func (variant *Variant) levelFor(config reasoning.Config) string {
	order := variant.Order
	if effort := strings.ToLower(strings.TrimSpace(config.Effort)); effort != "" {
		switch effort {
		case "minimal", "none":
			return order[0]
		case "xhigh", "max", "ultra":
			return order[len(order)-1]
		}
		if _, ok := variant.Levels[effort]; ok {
			return effort
		}
	}
	if config.BudgetTokens != nil && *config.BudgetTokens > 0 && len(variant.BudgetThresholds) == len(order)-1 {
		index := 0
		for _, threshold := range variant.BudgetThresholds {
			if *config.BudgetTokens >= threshold {
				index++
			}
		}
		return order[index]
	}
	switch strings.ToLower(strings.TrimSpace(config.Mode)) {
	case "disabled", "none", "off":
		return order[0]
	}
	return variant.Default
}

// Table is an immutable set of variants keyed by client-facing name.
type Table struct {
	variants map[string]*Variant
}

// Parse decodes and validates a JSON array of variants. Names must be unique
// and may not also appear as another variant's upstream target.
func Parse(data []byte) (*Table, error) {
	var list []Variant
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode effort variants: %w", err)
	}
	table := &Table{variants: make(map[string]*Variant, len(list))}
	for i := range list {
		if err := list[i].normalize(); err != nil {
			return nil, err
		}
		if _, dup := table.variants[list[i].Name]; dup {
			return nil, fmt.Errorf("duplicate variant name %q", list[i].Name)
		}
		table.variants[list[i].Name] = &list[i]
	}
	for _, variant := range table.variants {
		for _, target := range variant.Levels {
			if _, chained := table.variants[target]; chained {
				return nil, fmt.Errorf("%s: target %q is itself a variant", variant.Name, target)
			}
		}
	}
	return table, nil
}

// Names returns the client-facing variant names.
func (table *Table) Names() []string {
	if table == nil {
		return nil
	}
	names := make([]string, 0, len(table.variants))
	for name := range table.variants {
		names = append(names, name)
	}
	return names
}

// Resolve returns the upstream client model for name and the level it chose.
// ok is false when name is not a variant.
func (table *Table) Resolve(name string, config reasoning.Config) (target, level string, ok bool) {
	if table == nil {
		return "", "", false
	}
	variant, found := table.variants[name]
	if !found {
		return "", "", false
	}
	level = variant.levelFor(config)
	return variant.Levels[level], level, true
}

// Source loads a Table from a file and reloads it when the file changes.
// A missing file means the feature is off; a broken file keeps the last good
// table so a bad edit never takes routing down.
type Source struct {
	path string
	now  func() time.Time
	warn func(format string, args ...any)

	mu        sync.Mutex
	table     *Table
	modTime   time.Time
	size      int64
	checkedAt time.Time
}

// NewSourceFromEnv resolves the config path from EFFORT_VARIANTS_FILE or
// DATA_DIR/effort-variants.json.
func NewSourceFromEnv(warn func(format string, args ...any)) *Source {
	path := strings.TrimSpace(os.Getenv(EnvFile))
	if path == "" {
		dir := strings.TrimSpace(os.Getenv("DATA_DIR"))
		if dir == "" {
			dir = "./data"
		}
		path = filepath.Join(dir, DefaultFileName)
	}
	return NewSource(path, warn)
}

// NewSource creates a Source for path.
func NewSource(path string, warn func(format string, args ...any)) *Source {
	if warn == nil {
		warn = func(string, ...any) {}
	}
	return &Source{path: path, now: time.Now, warn: warn}
}

// Table returns the current table, or nil when none is configured.
func (source *Source) Table() *Table {
	if source == nil {
		return nil
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	now := source.now()
	if !source.checkedAt.IsZero() && now.Sub(source.checkedAt) < reloadInterval {
		return source.table
	}
	source.checkedAt = now
	info, err := os.Stat(source.path)
	if err != nil {
		if source.table != nil && errors.Is(err, os.ErrNotExist) {
			source.warn("effort variants file removed, feature disabled: %s", source.path)
			source.table, source.modTime, source.size = nil, time.Time{}, 0
		}
		return source.table
	}
	if info.ModTime().Equal(source.modTime) && info.Size() == source.size {
		return source.table
	}
	source.modTime, source.size = info.ModTime(), info.Size()
	data, err := os.ReadFile(source.path)
	if err != nil {
		source.warn("effort variants: read %s: %v", source.path, err)
		return source.table
	}
	table, err := Parse(data)
	if err != nil {
		source.warn("effort variants: ignoring invalid %s (keeping previous): %v", source.path, err)
		return source.table
	}
	source.table = table
	source.warn("effort variants loaded: %d variant(s) from %s", len(table.variants), source.path)
	return source.table
}
