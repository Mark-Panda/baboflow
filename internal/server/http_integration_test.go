package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"
	"baboflow/internal/service"
)

func TestHTTPProtoAuthenticationAndSidecars(t *testing.T) {
	server := newHTTPIntegrationServer(t)
	login := serveHTTP(server, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"secret"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body=%s", login.Code, http.StatusOK, login.Body.String())
	}
	sessionCookie := findCookie(t, login.Result().Cookies(), biz.SessionCookieName)

	t.Run("login sets session cookie", func(t *testing.T) {
		if sessionCookie.Value == "" {
			t.Fatalf("%s cookie must have a value", biz.SessionCookieName)
		}
	})

	t.Run("protected proto route rejects unauthenticated request", func(t *testing.T) {
		response := serveHTTP(server, http.MethodGet, "/api/v1/auth/me", "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
		}
	})

	t.Run("authenticated proto route succeeds", func(t *testing.T) {
		response := serveHTTP(server, http.MethodGet, "/api/v1/auth/me", "", []*http.Cookie{sessionCookie})
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("response must be JSON: %v", err)
		}
		if body["userId"] != "7" {
			t.Fatalf("unexpected authenticated response: %s", response.Body.String())
		}
	})

	t.Run("invalid proto JSON returns native bad request", func(t *testing.T) {
		response := serveHTTP(server, http.MethodPost, "/api/v1/auth/login", `{"username":`, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
		}
		if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("response must be JSON: %v", err)
		}
		if _, ok := body["message"]; !ok {
			t.Fatalf("native error must include message: %s", response.Body.String())
		}
	})

	t.Run("authenticated session reaches health sidecar with plain-text response", func(t *testing.T) {
		health := serveHTTP(server, http.MethodGet, "/healthz", "", []*http.Cookie{sessionCookie})
		if health.Code != http.StatusOK {
			t.Fatalf("health sidecar status = %d, want %d; body=%s", health.Code, http.StatusOK, health.Body.String())
		}
		if !strings.HasPrefix(health.Header().Get("Content-Type"), "text/plain") {
			t.Fatalf("health sidecar Content-Type = %q, want text/plain", health.Header().Get("Content-Type"))
		}
		if health.Body.String() != "ok" {
			t.Fatalf("health sidecar body = %q, want %q", health.Body.String(), "ok")
		}
	})

	t.Run("protected sidecars return JSON when unauthenticated", func(t *testing.T) {
		asset := serveHTTP(server, http.MethodPost, "/api/v1/agent-assets", "", nil)
		if asset.Code != http.StatusUnauthorized || !strings.HasPrefix(asset.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("asset sidecar = %d %q", asset.Code, asset.Header().Get("Content-Type"))
		}
		mcp := serveHTTP(server, http.MethodGet, "/mcp/sse", "", nil)
		if mcp.Code != http.StatusUnauthorized || !strings.HasPrefix(mcp.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("MCP sidecar = %d %q", mcp.Code, mcp.Header().Get("Content-Type"))
		}
	})
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("expected %s cookie, got %#v", name, cookies)
	return nil
}

func newHTTPIntegrationServer(t *testing.T) http.Handler {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	auth := biz.NewAuthUsecase(&httpIntegrationAuthRepo{
		sessions: map[string]*po.Session{},
		users: map[int64]*po.AdminUser{
			7: {ID: 7, Username: "admin", DisplayName: "管理员", PasswordHash: string(hash)},
		},
	})
	authProto, archeryProto, llmProto, componentProto, ruleChainProto, agentProto, skillProto, mcpProto, boardProto, auditProto, cronProto := testProtoServices()
	authProto = service.NewAuthProtoService(auth, nil)
	rateLimiters := service.NewRateLimiters()
	t.Cleanup(rateLimiters.Stop)
	return NewHTTPServer(
		&conf.Config{HTTPAddr: ":0"},
		auth,
		authProto, archeryProto, llmProto, componentProto, ruleChainProto, agentProto, skillProto, mcpProto, boardProto, auditProto, cronProto,
		rateLimiters,
		service.NewFeishuHandler(nil), service.NewAgentHandler(nil), service.NewSkillHandler(nil),
		service.NewWsHub(nil, nil), biz.NewMcpUsecase(nil, nil), &gorm.DB{},
	)
}

func serveHTTP(server http.Handler, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

type httpIntegrationAuthRepo struct {
	sessions map[string]*po.Session
	users    map[int64]*po.AdminUser
}

func (r *httpIntegrationAuthRepo) FindUserByUsername(_ context.Context, username string) (*po.AdminUser, error) {
	for _, user := range r.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

func (*httpIntegrationAuthRepo) FindUserByFeishuOpenID(context.Context, string) (*po.AdminUser, error) {
	return nil, errors.New("user not found")
}

func (*httpIntegrationAuthRepo) CreateUser(context.Context, *po.AdminUser) error { return nil }

func (*httpIntegrationAuthRepo) UpdateFeishuProfile(context.Context, int64, string, string, string, string) error {
	return nil
}

func (r *httpIntegrationAuthRepo) FindUserByID(_ context.Context, id int64) (*po.AdminUser, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (*httpIntegrationAuthRepo) UpdateUserPassword(context.Context, int64, string, bool) error {
	return nil
}

func (*httpIntegrationAuthRepo) TouchLastLogin(context.Context, int64) error { return nil }

func (r *httpIntegrationAuthRepo) CreateSession(_ context.Context, session *po.Session) error {
	r.sessions[session.ID] = session
	return nil
}

func (r *httpIntegrationAuthRepo) FindSession(_ context.Context, id string) (*po.Session, error) {
	session, ok := r.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (*httpIntegrationAuthRepo) TouchSession(context.Context, string, time.Time) error { return nil }

func (r *httpIntegrationAuthRepo) DeleteSession(_ context.Context, id string) error {
	delete(r.sessions, id)
	return nil
}

func (*httpIntegrationAuthRepo) DeleteOtherSessions(context.Context, int64, string) error { return nil }
