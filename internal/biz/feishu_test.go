package biz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// fakeFeishuServer 伪造飞书开放平台登录所需的 3 个接口。
func fakeFeishuServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/open-apis/auth/v3/app_access_token/internal", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["app_id"] != "cli_test" || in["app_secret"] != "secret_test" {
			json.NewEncoder(w).Encode(map[string]any{"code": 10003, "msg": "invalid app_id/app_secret"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "app_access_token": "app-token-1", "expire": 7200})
	})
	mux.HandleFunc("/open-apis/authen/v1/oidc/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-token-1" {
			json.NewEncoder(w).Encode(map[string]any{"code": 99991661, "msg": "invalid app access token"})
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["code"] == "bad-code" {
			json.NewEncoder(w).Encode(map[string]any{"code": 20029, "msg": "code expired"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "msg": "ok",
			"data": map[string]any{"access_token": "user-token-1", "open_id": "ou_new_123", "name": "张三"},
		})
	})
	mux.HandleFunc("/open-apis/authen/v1/user_info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-token-1" {
			json.NewEncoder(w).Encode(map[string]any{"code": 99991663, "msg": "invalid user access token"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "msg": "ok",
			"data": map[string]any{
				"open_id": "ou_new_123", "union_id": "on_union_1",
				"name": "张三", "email": "zhangsan@example.com", "avatar_url": "https://a.b/c.png",
			},
		})
	})
	return httptest.NewServer(mux)
}

// memAuthRepo 内存版 AuthRepo，支持按 openid 查、建号、建会话。
type memAuthRepo struct {
	byOpenID map[string]*po.AdminUser
	byID     map[int64]*po.AdminUser
	sessions map[string]*po.Session
	nextID   int64
}

func newMemAuthRepo() *memAuthRepo {
	return &memAuthRepo{
		byOpenID: map[string]*po.AdminUser{}, byID: map[int64]*po.AdminUser{},
		sessions: map[string]*po.Session{}, nextID: 1,
	}
}

func (m *memAuthRepo) FindUserByUsername(ctx context.Context, u string) (*po.AdminUser, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *memAuthRepo) FindUserByID(ctx context.Context, id int64) (*po.AdminUser, error) {
	if u, ok := m.byID[id]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memAuthRepo) FindUserByFeishuOpenID(ctx context.Context, openid string) (*po.AdminUser, error) {
	if u, ok := m.byOpenID[openid]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memAuthRepo) CreateUser(ctx context.Context, u *po.AdminUser) error {
	u.ID = m.nextID
	m.nextID++
	m.byID[u.ID] = u
	if u.FeishuOpenID != nil {
		m.byOpenID[*u.FeishuOpenID] = u
	}
	return nil
}
func (m *memAuthRepo) UpdateFeishuProfile(ctx context.Context, id int64, dn, av, em, un string) error {
	if u, ok := m.byID[id]; ok {
		u.DisplayName, u.Avatar, u.Email, u.FeishuUnionID = dn, av, em, un
	}
	return nil
}
func (m *memAuthRepo) UpdateUserPassword(ctx context.Context, id int64, h string, mc bool) error {
	return nil
}
func (m *memAuthRepo) TouchLastLogin(ctx context.Context, id int64) error { return nil }
func (m *memAuthRepo) CreateSession(ctx context.Context, s *po.Session) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *memAuthRepo) FindSession(ctx context.Context, id string) (*po.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memAuthRepo) TouchSession(ctx context.Context, id string, exp time.Time) error { return nil }
func (m *memAuthRepo) DeleteSession(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func feishuTestConfig() *conf.Config {
	return &conf.Config{
		FeishuAppID: "cli_test", FeishuAppSecret: "secret_test",
		FeishuRedirectURI: "https://babo.test/api/v1/auth/feishu/callback",
	}
}

// withFakeFeishu 把包级飞书 base URL 指向 httptest 服务器，返回清理函数。
func withFakeFeishu(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := feishuBaseURL
	feishuBaseURL = srv.URL
	t.Cleanup(func() { feishuBaseURL = old })
}

func TestFeishuUsecase_Configured(t *testing.T) {
	uc := NewFeishuUsecase(newMemAuthRepo(), feishuTestConfig())
	if !uc.Configured() {
		t.Fatal("expected Configured=true when all three set")
	}
	uc2 := NewFeishuUsecase(newMemAuthRepo(), &conf.Config{})
	if uc2.Configured() {
		t.Fatal("expected Configured=false when empty")
	}
}

func TestFeishuUsecase_BuildAuthURL(t *testing.T) {
	uc := NewFeishuUsecase(newMemAuthRepo(), feishuTestConfig())
	u := uc.BuildAuthURL("state-1")
	for _, want := range []string{"app_id=cli_test", "state=state-1", "redirect_uri=", "authen/v1/authorize"} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth url missing %q: %s", want, u)
		}
	}
}

func TestFeishuUsecase_LoginByCode_CreatesUser(t *testing.T) {
	srv := fakeFeishuServer(t)
	defer srv.Close()
	withFakeFeishu(t, srv)

	repo := newMemAuthRepo()
	uc := NewFeishuUsecase(repo, feishuTestConfig())
	res, err := uc.LoginByCode(context.Background(), "good-code", "1.2.3.4", "agent")
	if err != nil {
		t.Fatalf("LoginByCode error: %v", err)
	}
	if res.SessionID == "" {
		t.Fatal("expected a session id")
	}
	if res.Username != "feishu_ou_new_123" {
		t.Fatalf("expected username=feishu_ou_new_123, got %q", res.Username)
	}
	if res.DisplayName != "张三" {
		t.Fatalf("expected displayName=张三, got %q", res.DisplayName)
	}
	// 会话确实落库，且能找到刚建的用户。
	if _, ok := repo.sessions[res.SessionID]; !ok {
		t.Fatal("session not persisted")
	}
	if _, ok := repo.byOpenID["ou_new_123"]; !ok {
		t.Fatal("user not indexed by openid")
	}
}

func TestFeishuUsecase_LoginByCode_ReusesExistingUser(t *testing.T) {
	srv := fakeFeishuServer(t)
	defer srv.Close()
	withFakeFeishu(t, srv)

	repo := newMemAuthRepo()
	uc := NewFeishuUsecase(repo, feishuTestConfig())

	first, err := uc.LoginByCode(context.Background(), "good-code", "1.2.3.4", "agent")
	if err != nil {
		t.Fatalf("first login error: %v", err)
	}
	second, err := uc.LoginByCode(context.Background(), "good-code", "1.2.3.4", "agent")
	if err != nil {
		t.Fatalf("second login error: %v", err)
	}
	if first.UserID != second.UserID {
		t.Fatalf("expected same user id across logins, got %d then %d", first.UserID, second.UserID)
	}
	if len(repo.byID) != 1 {
		t.Fatalf("expected exactly 1 user, got %d", len(repo.byID))
	}
	if first.SessionID == second.SessionID {
		t.Fatal("expected a fresh session per login")
	}
}

func TestFeishuUsecase_LoginByCode_BadCode(t *testing.T) {
	srv := fakeFeishuServer(t)
	defer srv.Close()
	withFakeFeishu(t, srv)

	uc := NewFeishuUsecase(newMemAuthRepo(), feishuTestConfig())
	_, err := uc.LoginByCode(context.Background(), "bad-code", "1.2.3.4", "agent")
	if err == nil {
		t.Fatal("expected error for bad code")
	}
	var fe *feishuErr
	if !errors.As(err, &fe) {
		t.Fatalf("expected feishuErr, got %T: %v", err, err)
	}
}

func TestFeishuUsecase_LoginByCode_NotConfigured(t *testing.T) {
	uc := NewFeishuUsecase(newMemAuthRepo(), &conf.Config{})
	if _, err := uc.LoginByCode(context.Background(), "x", "1.2.3.4", "agent"); !errors.Is(err, ErrFeishuNotConfigured) {
		t.Fatalf("expected ErrFeishuNotConfigured, got %v", err)
	}
}
