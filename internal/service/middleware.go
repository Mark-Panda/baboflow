package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"baboflow/internal/biz"
)

// context keys
const (
	ctxUserID  = "babo_uid"
	ctxSession = "babo_sid"
)

type secureCookieContextKey struct{}

func sessionCookieContext(c *gin.Context) context.Context {
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	return context.WithValue(c.Request.Context(), secureCookieContextKey{}, secure)
}

func setSessionCookie(c *gin.Context, name, value string, maxAge int) {
	setCookie(sessionCookieContext(c), c.Writer.Header(), name, value, maxAge)
}

// GinAuthMiddleware 为 multipart/raw 下载等 Gin 旁路提供 Session 认证。
func GinAuthMiddleware(auth *biz.AuthUsecase) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(biz.SessionCookieName)
		if err != nil || sid == "" {
			ginError(c, http.StatusUnauthorized, "未登录")
			c.Abort()
			return
		}
		user, err := auth.Validate(c.Request.Context(), sid)
		if err != nil {
			ginError(c, http.StatusUnauthorized, "会话已过期，请重新登录")
			c.Abort()
			return
		}
		c.Set(ctxUserID, user.ID)
		c.Set(ctxSession, sid)
		c.Next()
	}
}

// SetSessionCookie 将登录会话写入与传输实现无关的响应头。
func SetSessionCookie(ctx context.Context, header http.Header, sessionID string, maxAge int) {
	setCookie(ctx, header, biz.SessionCookieName, sessionID, maxAge)
}

func setCookie(ctx context.Context, header http.Header, name, value string, maxAge int) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   secureCookie(ctx),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	header.Add("Set-Cookie", cookie.String())
}

func secureCookie(ctx context.Context) bool {
	if secure, ok := ctx.Value(secureCookieContextKey{}).(bool); ok {
		return secure
	}
	request, ok := khttp.RequestFromServerContext(ctx)
	return ok && (request.TLS != nil || strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https"))
}

func CurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get(ctxUserID); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

func ginError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"message": message, "error": message})
}

func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		ginError(c, http.StatusBadRequest, "非法 id")
		return 0, false
	}
	return id, true
}
