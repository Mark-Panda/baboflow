package service

import (
	"context"
	"net"
	"net/http"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/metadata"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc/peer"

	"baboflow/internal/biz"
)

const (
	authMissingReason = "AUTH_MISSING"
	authInvalidReason = "AUTH_INVALID"
)

// AuthMiddleware 校验 HTTP Cookie 或 gRPC Bearer 中携带的会话。
func AuthMiddleware(auth *biz.AuthUsecase) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			sid := sessionIDFromContext(ctx)
			if sid == "" {
				return nil, kerrors.Unauthorized(authMissingReason, "未登录")
			}
			user, err := auth.Validate(ctx, sid)
			if err != nil || user == nil {
				return nil, kerrors.Unauthorized(authInvalidReason, "会话已过期，请重新登录")
			}
			ctx = context.WithValue(ctx, ctxUserID, user.ID)
			ctx = context.WithValue(ctx, ctxSession, sid)
			return handler(ctx, req)
		}
	}
}

func sessionIDFromContext(ctx context.Context) string {
	if tr, ok := transport.FromServerContext(ctx); ok && tr.Kind() == transport.KindHTTP {
		request := &http.Request{Header: http.Header{}}
		for _, value := range tr.RequestHeader().Values("Cookie") {
			request.Header.Add("Cookie", value)
		}
		if cookie, err := request.Cookie(biz.SessionCookieName); err == nil {
			return cookie.Value
		}
		return ""
	}
	md, ok := metadata.FromServerContext(ctx)
	if !ok {
		return ""
	}
	return bearerToken(md.Get("authorization"))
}

// ClientMetadataFromContext returns the direct transport peer address and user agent.
// Forwarded headers are not trusted because public clients can forge them.
func ClientMetadataFromContext(ctx context.Context) (string, string) {
	headers := http.Header{}
	if tr, ok := transport.FromServerContext(ctx); ok {
		for _, key := range tr.RequestHeader().Keys() {
			for _, value := range tr.RequestHeader().Values(key) {
				headers.Add(key, value)
			}
		}
	}
	if md, ok := metadata.FromServerContext(ctx); ok {
		for key, values := range md {
			if headers.Values(key) == nil {
				for _, value := range values {
					headers.Add(key, value)
				}
			}
		}
	}
	if request, ok := khttp.RequestFromServerContext(ctx); ok {
		return remoteIP(request.RemoteAddr), headers.Get("User-Agent")
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return remoteIP(p.Addr.String()), headers.Get("User-Agent")
	}
	return "", headers.Get("User-Agent")
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
