package biz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归：baseUrl 缺 /v1 时，OpenAI 兼容上游在 {base}/models 返回 404（见 Kimi 接入排查）。
// FetchModels 应忠实把非 200 映射为「上游返回状态 N」，而不是误拼地址。
func TestFetchModelsUpstream404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
	}))
	defer srv.Close()

	_, err := newLLMClient().FetchModels(context.Background(), srv.URL, "sk-x")
	if err == nil {
		t.Fatal("expected error on upstream 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got %v", err)
	}
}

// FetchModels 命中 {base}/models 并解析出模型 id 列表。
func TestFetchModelsOK(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"},{"id":""},{"id":"m3"}]}`))
	}))
	defer srv.Close()

	// baseUrl 故意带尾斜杠，验证 normalizeBase 去重，不产生 "//models"。
	ids, err := newLLMClient().FetchModels(context.Background(), srv.URL+"/v1/", "sk-secret")
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("expected path /v1/models, got %q", gotPath)
	}
	if gotAuth != "Bearer sk-secret" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	if len(ids) != 3 || ids[0] != "m1" || ids[2] != "m3" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

// baseUrl 校验：拒绝非 http(s)、缺 host、内嵌 userinfo；允许合法 http(s)。
func TestValidateProviderBaseURL(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"https://api.kimi.com/coding/v1", false},
		{"http://127.0.0.1:9000/v1", false}, // 私有 LLM 网关允许
		{"", true},
		{"   ", true},
		{"ftp://x/v1", true},
		{"not-a-url", true},
		{"https:///nov1", true},
		{"https://user:pass@host/v1", true}, // 拒绝 userinfo（防 SSRF 伪装）
	}
	for _, c := range cases {
		err := validateProviderBaseURL(c.raw)
		if c.wantErr && err == nil {
			t.Errorf("expected error for %q", c.raw)
		}
		if !c.wantErr && err != nil {
			t.Errorf("unexpected error for %q: %v", c.raw, err)
		}
	}
}

// TestModel 上游非 200 时返回「上游返回状态 N」并给出延迟。
func TestTestModelUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	lat, err := newLLMClient().TestModel(context.Background(), srv.URL+"/v1", "sk-x", "m")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got lat=%d err=%v", lat, err)
	}
}
