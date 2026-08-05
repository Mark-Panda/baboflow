package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/metadata"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/peer"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

type testTransport struct {
	kind        transport.Kind
	header      transport.Header
	replyHeader transport.Header
}

type testHeader map[string][]string

func (h testHeader) Get(key string) string      { return http.Header(h).Get(key) }
func (h testHeader) Set(key, value string)      { http.Header(h).Set(key, value) }
func (h testHeader) Add(key, value string)      { http.Header(h).Add(key, value) }
func (h testHeader) Values(key string) []string { return http.Header(h).Values(key) }
func (h testHeader) Keys() []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		keys = append(keys, strings.ToLower(key))
	}
	return keys
}

func (t testTransport) Kind() transport.Kind            { return t.kind }
func (t testTransport) Endpoint() string                { return "" }
func (t testTransport) Operation() string               { return "" }
func (t testTransport) RequestHeader() transport.Header { return t.header }
func (t testTransport) ReplyHeader() transport.Header {
	if t.replyHeader != nil {
		return t.replyHeader
	}
	return testHeader{}
}

func testAuthUsecase() *biz.AuthUsecase {
	return biz.NewAuthUsecase(&stubAuthRepo{
		sessions: map[string]*po.Session{
			"valid-sid": {ID: "valid-sid", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)},
		},
		users: map[int64]*po.AdminUser{
			7: {ID: 7, Username: "admin"},
		},
	})
}

func invokeMiddleware(ctx context.Context, mw middleware.Middleware) (context.Context, error) {
	var handlerCtx context.Context
	_, err := mw(func(ctx context.Context, req any) (any, error) {
		handlerCtx = ctx
		return nil, nil
	})(ctx, nil)
	return handlerCtx, err
}

func TestKratosMiddlewareMissingSession(t *testing.T) {
	ctx := transport.NewServerContext(context.Background(), testTransport{
		kind:   transport.KindHTTP,
		header: testHeader{},
	})
	_, err := invokeMiddleware(ctx, AuthMiddleware(testAuthUsecase()))
	if !kerrors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized, got %v", err)
	}
}

func TestKratosMiddlewareInvalidSession(t *testing.T) {
	header := testHeader{}
	header.Set("Cookie", biz.SessionCookieName+"=invalid-sid")
	ctx := transport.NewServerContext(context.Background(), testTransport{
		kind:   transport.KindHTTP,
		header: header,
	})
	_, err := invokeMiddleware(ctx, AuthMiddleware(testAuthUsecase()))
	if !kerrors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized, got %v", err)
	}
}

func TestKratosMiddlewareValidCookie(t *testing.T) {
	header := testHeader{}
	header.Set("Cookie", biz.SessionCookieName+"=valid-sid")
	ctx := transport.NewServerContext(context.Background(), testTransport{
		kind:   transport.KindHTTP,
		header: header,
	})
	handlerCtx, err := invokeMiddleware(ctx, AuthMiddleware(testAuthUsecase()))
	if err != nil {
		t.Fatalf("expected valid session, got %v", err)
	}
	if got := handlerCtx.Value(ctxUserID); got != int64(7) {
		t.Fatalf("expected user ID 7, got %v", got)
	}
	if got := handlerCtx.Value(ctxSession); got != "valid-sid" {
		t.Fatalf("expected session ID valid-sid, got %v", got)
	}
}

func TestKratosMiddlewareValidGRPCBearer(t *testing.T) {
	ctx := metadata.NewServerContext(context.Background(), metadata.New(map[string][]string{
		"authorization": {"Bearer valid-sid"},
	}))
	handlerCtx, err := invokeMiddleware(ctx, AuthMiddleware(testAuthUsecase()))
	if err != nil {
		t.Fatalf("expected valid session, got %v", err)
	}
	if got := handlerCtx.Value(ctxUserID); got != int64(7) {
		t.Fatalf("expected user ID 7, got %v", got)
	}
}

func TestKratosMiddlewareGRPCBearerSchemeAndFormat(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		authorized    bool
	}{
		{name: "lowercase scheme", authorization: "bearer valid-sid", authorized: true},
		{name: "uppercase scheme", authorization: "BEARER valid-sid", authorized: true},
		{name: "missing token", authorization: "Bearer", authorized: false},
		{name: "extra value", authorization: "Bearer valid-sid extra", authorized: false},
		{name: "wrong scheme", authorization: "Basic valid-sid", authorized: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := metadata.NewServerContext(context.Background(), metadata.New(map[string][]string{
				"authorization": {tt.authorization},
			}))
			_, err := invokeMiddleware(ctx, AuthMiddleware(testAuthUsecase()))
			if tt.authorized && err != nil {
				t.Fatalf("expected authorization, got %v", err)
			}
			if !tt.authorized && !kerrors.IsUnauthorized(err) {
				t.Fatalf("expected Unauthorized, got %v", err)
			}
		})
	}
}

func TestGinAuthMiddlewareCompatibility(t *testing.T) {
	router := gin.New()
	router.GET("/", GinAuthMiddleware(testAuthUsecase()), func(c *gin.Context) {
		if got := CurrentUserID(c); got != 7 {
			t.Fatalf("expected user ID 7, got %d", got)
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "valid-sid"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", response.Code)
	}
}

func TestSessionCookie(t *testing.T) {
	header := http.Header{}
	SetSessionCookie(context.Background(), header, "valid-sid", 7*24*3600)
	response := &http.Response{Header: header}
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != biz.SessionCookieName || cookie.Value != "valid-sid" {
		t.Fatalf("unexpected cookie: %#v", cookie)
	}
	if !cookie.HttpOnly || cookie.Path != "/" || cookie.MaxAge != 7*24*3600 {
		t.Fatalf("unexpected cookie attributes: %#v", cookie)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
}

func TestSessionCookieSecurePolicy(t *testing.T) {
	header := http.Header{}
	ctx := context.WithValue(context.Background(), secureCookieContextKey{}, true)
	SetSessionCookie(ctx, header, "valid-sid", 60)
	cookies := (&http.Response{Header: header}).Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected Secure cookie, got %#v", cookies)
	}
}

func TestRateLimitLoginAndTriggerLimits(t *testing.T) {
	limits := NewRateLimiters()
	t.Cleanup(limits.Stop)
	header := testHeader{}
	header.Set("X-Forwarded-For", "198.51.100.9")
	ctx := transport.NewServerContext(context.Background(), testTransport{
		kind:   transport.KindHTTP,
		header: header,
	})
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}})
	if got := clientIP(ctx); got != "127.0.0.1" {
		t.Fatalf("gRPC rate limit key must use peer address, got %q", got)
	}
	assertLimit(t, limits.LoginMiddleware(), ctx, 5)
	assertLimit(t, limits.TriggerMiddleware(), ctx, 10)
}

func TestRateLimitersStop(t *testing.T) {
	limits := NewRateLimiters()
	done := make(chan struct{})
	go func() {
		limits.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rate limiter cleanup goroutines did not stop")
	}
}

func assertLimit(t *testing.T, mw middleware.Middleware, ctx context.Context, capacity int) {
	t.Helper()
	handler := mw(func(context.Context, any) (any, error) { return nil, nil })
	for i := 0; i < capacity; i++ {
		if _, err := handler(ctx, nil); err != nil {
			t.Fatalf("request %d should pass: %v", i+1, err)
		}
	}
	if _, err := handler(ctx, nil); kerrors.Code(err) != http.StatusTooManyRequests {
		t.Fatalf("request %d should be rate limited, got %v", capacity+1, err)
	}
}
