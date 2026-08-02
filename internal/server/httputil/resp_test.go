package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func run(t *testing.T, fn func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	fn(c)
	return w
}

type body struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decode(t *testing.T, w *httptest.ResponseRecorder) body {
	t.Helper()
	var b body
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode envelope: %v (raw=%s)", err, w.Body.String())
	}
	return b
}

func TestOK_EnvelopeAndStatus(t *testing.T) {
	w := run(t, func(c *gin.Context) { OK(c, map[string]int{"n": 1}) })
	if w.Code != http.StatusOK {
		t.Fatalf("OK status = %d", w.Code)
	}
	b := decode(t, w)
	if b.Code != 0 || b.Message != "ok" {
		t.Fatalf("OK envelope = %+v", b)
	}
}

func TestFail_HTTPStatusMatchesCode(t *testing.T) {
	cases := []struct {
		name       string
		fn         func(c *gin.Context)
		wantHTTP   int
		wantCode   int
	}{
		{"bad request", func(c *gin.Context) { BadRequest(c, "x") }, http.StatusBadRequest, CodeBadRequest},
		{"unauthorized", func(c *gin.Context) { Unauthorized(c, "x") }, http.StatusUnauthorized, CodeUnauthorized},
		{"not found", func(c *gin.Context) { NotFound(c, "x") }, http.StatusNotFound, CodeNotFound},
		{"conflict", func(c *gin.Context) { Conflict(c, "x") }, http.StatusConflict, CodeConflict},
		{"internal", func(c *gin.Context) { Internal(c, "x") }, http.StatusInternalServerError, CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := run(t, tc.fn)
			if w.Code != tc.wantHTTP {
				t.Fatalf("HTTP status = %d, want %d", w.Code, tc.wantHTTP)
			}
			if b := decode(t, w); b.Code != tc.wantCode {
				t.Fatalf("envelope code = %d, want %d", b.Code, tc.wantCode)
			}
		})
	}
}

func TestFail_NonStandardCodeFallsBackTo200(t *testing.T) {
	// 429 等非 4xx/5xx 标准段以外的业务码：HTTP 状态回退 200，由信封 code 表达。
	w := run(t, func(c *gin.Context) { Fail(c, 429, "too many") })
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("429 is a valid HTTP status, expect 429, got %d", w.Code)
	}
	if b := decode(t, w); b.Code != 429 {
		t.Fatalf("envelope code = %d, want 429", b.Code)
	}
}
