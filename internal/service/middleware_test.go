package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

func init() { gin.SetMode(gin.TestMode) }

// ---- MCPAuthMiddleware ----

// stubAuthRepo 提供最小可用的 AuthRepo（仅 Validate 路径需要）。
type stubAuthRepo struct {
	sessions map[string]*po.Session
	users    map[int64]*po.AdminUser
}

func (s *stubAuthRepo) FindUserByUsername(ctx context.Context, u string) (*po.AdminUser, error) {
	for _, user := range s.users {
		if user.Username == u {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
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
func (s *stubAuthRepo) DeleteOtherSessions(ctx context.Context, userID int64, keepSessionID string) error {
	return nil
}

func newMCPTestServer(token string, repo biz.AuthRepo) *gin.Engine {
	r := gin.New()
	authUC := biz.NewAuthUsecase(repo)
	r.POST("/mcp/message", MCPAuthMiddleware(authUC, token), func(c *gin.Context) {
		c.String(http.StatusOK, "message-ok")
	})
	return r
}

func TestGinErrorIncludesFrontendMessageContract(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		ginError(c, http.StatusBadRequest, "请求错误")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "请求错误" {
		t.Fatalf("message = %q, want %q; body=%s", body["message"], "请求错误", w.Body.String())
	}
}

func TestMCPAuth_NoCredentials_Rejected(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401 without credentials, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestMCPAuth_ValidBearerToken_Allowed(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "message-ok" {
		t.Fatalf("expect 200 with valid token, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestMCPAuth_WrongBearerToken_Rejected(t *testing.T) {
	repo := &stubAuthRepo{sessions: map[string]*po.Session{}, users: map[int64]*po.AdminUser{}}
	r := newMCPTestServer("secret-token", repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/mcp/message", nil)
	req.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "sid-old"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expect 401 with expired session, got %d", w.Code)
	}
}
