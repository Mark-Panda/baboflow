package service

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

// MCPAuthMiddleware 鉴权 /mcp/* SSE 端点。此前该端点对任何可达 :8000 的主机完全开放，
// 可未登录枚举并调用全部已暴露规则链（即未授权远程执行）。现要求以下任一凭据：
//  1. 同源已登录会话 Cookie（baboflow_sid）—— 供平台内/同域 MCP 客户端使用；
//  2. `Authorization: Bearer <MCP_AUTH_TOKEN>` —— 供外部 MCP 客户端（无法携带 Cookie）使用。
// token 校验使用常数时间比较。MCP_AUTH_TOKEN 为空时仅接受会话。
func MCPAuthMiddleware(auth *biz.AuthUsecase, token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) 会话 Cookie
		if sid, err := c.Cookie(biz.SessionCookieName); err == nil && sid != "" {
			if user, verr := auth.Validate(c.Request.Context(), sid); verr == nil && user != nil {
				c.Set(ctxUserID, user.ID)
				c.Set(ctxSession, sid)
				c.Next()
				return
			}
		}
		// 2) Bearer 令牌（常数时间比较）
		if token != "" {
			provided := bearerToken(c.GetHeader("Authorization"))
			if provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
				c.Next()
				return
			}
		}
		httputil.Unauthorized(c, "MCP 端点需要登录会话或有效 Bearer 令牌")
		c.Abort()
	}
}

// bearerToken 提取 "Authorization: Bearer <token>" 中的令牌部分。
func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return ""
}
