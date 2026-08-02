package service

import (
	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

// context keys
const (
	ctxUserID  = "babo_uid"
	ctxSession = "babo_sid"
)

// AuthMiddleware Session 认证：从 Cookie 读 sid，校验后注入 userID。除白名单外强制登录。
func AuthMiddleware(auth *biz.AuthUsecase) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(biz.SessionCookieName)
		if err != nil || sid == "" {
			httputil.Unauthorized(c, "未登录")
			c.Abort()
			return
		}
		user, err := auth.Validate(c.Request.Context(), sid)
		if err != nil {
			httputil.Unauthorized(c, "会话已过期，请重新登录")
			c.Abort()
			return
		}
		c.Set(ctxUserID, user.ID)
		c.Set(ctxSession, sid)
		c.Next()
	}
}

func CurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get(ctxUserID); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

func CurrentSessionID(c *gin.Context) string {
	if v, ok := c.Get(ctxSession); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
