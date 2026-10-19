package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// Reproduce the old saved provider with no decide field. Its migrated
// classifier must send typed questions to System One, signed with the same
// plan key, and the public System One endpoint must reach that model too.
func TestBailianMigratedClassifierUsesSystemOne(t *testing.T) {
	fresh(t)
	const old = `{
		"providers":[{"id":"bailian-token-plan","name":"Bailian","preset":"bailian-token-plan","key":"sk-sp-fixture",
			"chat":"https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
			"models":["qwen3.8-max","decision-model-preview"]}],
		"groups":[{"id":"work","name":"Work","members":["bailian-token-plan/qwen3.8-max"],
			"classifier":"bailian-token-plan/decision-model-preview","effort":"auto"}]
	}`
	if err := os.MkdirAll(filepath.Dir(provider.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provider.Path(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.SplitBailianTokenPlan(); err != nil {
		t.Fatal(err)
	}
	gs, err := provider.StoredGroups()
	if err != nil || len(gs) != 1 {
		t.Fatalf("groups: %+v %v", gs, err)
	}
	var mu sync.Mutex
	calls := 0
	s := New()
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Host != "token-plan.cn-beijing.maas.aliyuncs.com" ||
			r.URL.Path != "/compatible-mode/v1/systemone" || r.Header.Get("Authorization") != "Bearer sk-sp-fixture" {
			return nil, fmt.Errorf("wrong decision request: %s %s", r.Method, r.URL)
		}
		var body struct {
			Model     string         `json:"model"`
			State     map[string]any `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if body.Model != "decision-model-preview" || len(body.State) == 0 || len(body.Questions) == 0 {
			return nil, fmt.Errorf("not a System One request: %+v", body)
		}
		mu.Lock()
		calls++
		mu.Unlock()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"model":"decision-model-preview","answers":{
				"levels":{"type":"noul","noul":0},
				"intent":{"type":"choice","choice":"coding","confidence":1,"probabilities":{"coding":1,"writing":0}},
				"effort":{"type":"score","score":1,"confidence":1}
			},"usage":{"input_tokens":40}}`))}, nil
	})
	v, err := s.askClassifier(gs[0].Classifier, []string{"coding", "writing"}, before{}, true, "fix the login bug")
	if err != nil || v.Intent != "coding" || v.Effort != "medium" {
		t.Fatalf("classification: %+v %v", v, err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(
		`{"model":"bailian-decision/decision-model-preview","state":{"text":"fix the bug"},"questions":{"intent":{"type":"choice","criteria":{"coding":null,"writing":null}}}}`))
	r.Header.Set("Authorization", "Bearer magpie")
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"coding"`) {
		t.Fatalf("System One: %d %s", w.Code, w.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	// One call calibrates the kinds, one classifies the user's turn, and
	// one is the public System One request.
	if calls != 3 {
		t.Fatalf("asked %d times, want 3", calls)
	}
}
