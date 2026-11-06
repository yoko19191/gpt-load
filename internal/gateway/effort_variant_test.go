package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gpt-load/internal/dialect"
	"gpt-load/internal/effortvariant"
	"gpt-load/internal/protocol"
	"gpt-load/internal/state"
)

func TestHandlerRoutesEffortVariantToUpstreamTier(t *testing.T) {
	var (
		mu     sync.Mutex
		models []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var parsed struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &parsed)
		mu.Lock()
		models = append(models, parsed.Model)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"` + parsed.Model + `","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "effort-variants.json")
	if err := os.WriteFile(configPath, []byte(`[{"name":"tier-ag","levels":{"low":"tier-low","medium":"tier-medium","high":"tier-high"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(effortvariant.EnvFile, configPath)

	engine, _ := newDialectGatewayEngine(t, protocol.OpenAICompletions, "tier-medium",
		dialect.NewSet(dialect.NewOpenAI()),
		dialectGatewayGroup{id: 1, name: "ag", upstreamURL: upstream.URL, apiKeys: []string{"sk-one"},
			models: []state.ModelConfig{{ID: "tier-low"}, {ID: "tier-medium"}, {ID: "tier-high"}}},
	)

	send := func(body string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer gl-client")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	}
	send(`{"model":"tier-ag","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	send(`{"model":"tier-ag","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`)
	send(`{"model":"tier-ag","messages":[{"role":"user","content":"hi"}]}`)
	send(`{"model":"tier-high","messages":[{"role":"user","content":"hi"}]}`)

	mu.Lock()
	defer mu.Unlock()
	want := []string{"tier-high", "tier-low", "tier-medium", "tier-high"}
	if len(models) != len(want) {
		t.Fatalf("upstream saw %v, want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("upstream saw %v, want %v", models, want)
		}
	}
}
