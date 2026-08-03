package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

func init() { gin.SetMode(gin.TestMode) }

// ---- httputil 信封（经 service 间接覆盖）----
// 直接断言见 internal/server/httputil；这里覆盖中间件对信封/状态码的使用。

// ---- RateLimitMiddleware ----

func TestRateLimitMiddleware(t *testing.T) {
	// 容量 2、每秒补 1：前 2 个通过，第 3 个被限流。
	rl := RateLimitMiddleware(func(c *gin.Context) string { return "k" }, rate.Every(time.Second), 2)

	newReq := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
		rl(c)
		return w
	}

	if w := newReq(); w.Code != http.StatusOK {
		t.Fatalf("req1 should pass, got %d", w.Code)
	}
	if w := newReq(); w.Code != http.StatusOK {
		t.Fatalf("req2 should pass, got %d", w.Code)
	}
	w3 := newReq()
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("req3 should be 429, got %d", w3.Code)
	}
	if body := w3.Body.String(); !strings.Contains(body, "429") {
		t.Fatalf("429 body should carry code 429, got %s", body)
	}
}

func TestRateLimitMiddleware_PerKeyIsolation(t *testing.T) {
	rl := RateLimitMiddleware(func(c *gin.Context) string { return c.Query("k") }, rate.Every(time.Minute), 1)
	mk := func(key string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/x?k="+key, nil)
		rl(c)
		return w
	}
	// keyA 用掉额度后，keyB 仍应通过（互不影响）。
	if w := mk("a"); w.Code != http.StatusOK {
		t.Fatalf("a1 should pass")
	}
	if w := mk("a"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("a2 should be limited")
	}
	if w := mk("b"); w.Code != http.StatusOK {
		t.Fatalf("b1 should pass (isolated key)")
	}
}

// ---- MCPAuthMiddleware ----

// stubAuthRepo 提供最小可用的 AuthRepo（仅 Validate 路径需要）。
type stubAuthRepo struct {
	sessions map[string]*po.Session
	users    map[int64]*po.AdminUser
}

func (s *stubAuthRepo) FindUserByUsername(ctx context.Context, u string) (*po.AdminUser, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAuthRepo) FindUserByFeishuOpenID(ctx context.Context, openid string) (*po.AdminUser, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAuthRepo) CreateUser(ctx context.Context, u *po.AdminUser) error { return nil }
func (s *stubAuthRepo) UpdateFeishuProfile(ctx context.Context, id int64, dn, av, em, un string) error {
	return nil
}
func (s *stubAuthRepo) FindUserByID(ctx context.Context, id int64) (*po.AdminUser, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("user not found")
}
func (s *stubAuthRepo) UpdateUserPassword(ctx context.Context, id int64, h string, m bool) error {
	return nil
}
func (s *stubAuthRepo) TouchLastLogin(ctx context.Context, id int64) error { return nil }
func (s *stubAuthRepo) CreateSession(ctx context.Context, sess *po.Session) error {
	s.sessions[sess.ID] = sess
	return nil
}
func (s *stubAuthRepo) FindSession(ctx context.Context, id string) (*po.Session, error) {
	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}
	return nil, errors.New("session not found")
}
func (s *stubAuthRepo) TouchSession(ctx context.Context, id string, exp time.Time) error { return nil }
func (s *stubAuthRepo) DeleteSession(ctx context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

func newMCPTestServer(token string, repo biz.AuthRepo) *gin.Engine {
	r := gin.New()
	authUC := biz.NewAuthUsecase(repo)
	r.GET("/mcp/sse", MCPAuthMiddleware(authUC, token), func(c *gin.Context) {
		c.String(http.StatusOK, "sse-ok")
	})
	return r
}

func TestMCPAuth_NoCredentials_Rejected(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401 without credentials, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestMCPAuth_ValidBearerToken_Allowed(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "sse-ok" {
		t.Fatalf("expect 200 with valid token, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestMCPAuth_WrongBearerToken_Rejected(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401 with wrong token, got %d", w.Code)
	}
}

func TestMCPAuth_ValidSessionCookie_Allowed(t *testing.T) {
	repo := &stubAuthRepo{
		sessions: map[string]*po.Session{
			"sid-1": {ID: "sid-1", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)},
		},
		users: map[int64]*po.AdminUser{7: {ID: 7, Username: "admin"}},
	}
	r := newMCPTestServer("", repo) // 无 token：仅会话可用

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "sid-1"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expect 200 with valid session cookie, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestMCPAuth_ExpiredSessionCookie_Rejected(t *testing.T) {
	repo := &stubAuthRepo{
		sessions: map[string]*po.Session{
			"sid-old": {ID: "sid-old", UserID: 7, ExpiresAt: time.Now().Add(-time.Hour)},
		},
		users: map[int64]*po.AdminUser{7: {ID: 7, Username: "admin"}},
	}
	r := newMCPTestServer("", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "sid-old"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401 with expired session, got %d", w.Code)
	}
}
