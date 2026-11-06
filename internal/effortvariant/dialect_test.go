package effortvariant

import (
	"encoding/json"
	"testing"

	"gpt-load/internal/dialect"
)

// TestResolveFromRealRequests runs request bodies in the shapes clients send
// through the production dialects, then applies the variant table and rewrite.
func TestResolveFromRealRequests(t *testing.T) {
	table, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		d    interface {
			dialect.Dialect
			dialect.ModelRewriter
		}
		path string
		body string
		want string
	}{
		{"anthropic output_config.effort", dialect.NewAnthropic(), "/v1/messages",
			`{"model":"claude-opus-5-5-ag","max_tokens":64,"output_config":{"effort":"high"},"messages":[{"role":"user","content":"hi"}]}`,
			"claude-opus-5-5-high"},
		{"anthropic thinking budget", dialect.NewAnthropic(), "/v1/messages",
			`{"model":"claude-opus-5-5-ag","max_tokens":9000,"thinking":{"type":"enabled","budget_tokens":2000},"messages":[{"role":"user","content":"hi"}]}`,
			"claude-opus-5-5-low"},
		{"anthropic nothing", dialect.NewAnthropic(), "/v1/messages",
			`{"model":"claude-opus-5-5-ag","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`,
			"claude-opus-5-5-medium"},
		{"openai reasoning_effort", dialect.NewOpenAI(), "/v1/chat/completions",
			`{"model":"claude-opus-5-5-ag","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`,
			"claude-opus-5-5-low"},
	}
	for _, tc := range cases {
		req := &dialect.ParsedRequest{Method: "POST", Path: tc.path, Body: []byte(tc.body)}
		meta, err := tc.d.InspectRequest(req)
		if err != nil {
			t.Fatalf("%s: inspect: %v", tc.name, err)
		}
		if meta.Model == nil || *meta.Model != "claude-opus-5-5-ag" {
			t.Fatalf("%s: unexpected model %v", tc.name, meta.Model)
		}
		target, _, ok := table.Resolve(*meta.Model, meta.Reasoning)
		if !ok || target != tc.want {
			t.Fatalf("%s: resolved %q ok=%v want %q (reasoning %+v)", tc.name, target, ok, tc.want, meta.Reasoning)
		}
		rewritten, err := tc.d.RewriteRequestModel(req, target)
		if err != nil {
			t.Fatalf("%s: rewrite: %v", tc.name, err)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rewritten.Body, &body); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got string
		_ = json.Unmarshal(body["model"], &got)
		if got != tc.want {
			t.Errorf("%s: body model %q want %q", tc.name, got, tc.want)
		}
	}
}
