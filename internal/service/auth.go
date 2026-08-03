package service

import (
	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type AuthHandler struct {
	auth   *biz.AuthUsecase
	auditor *biz.AuditUsecase
}

func NewAuthHandler(auth *biz.AuthUsecase) *AuthHandler { return &AuthHandler{auth: auth} }

// SetAuditor 注入审计（M7）。
func (h *AuthHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *AuthHandler) audit(c *gin.Context, uid *int64, action, targetType, targetID string, detail map[string]any) {
	if h.auditor != nil {
		h.auditor.Record(c.Request.Context(), uid, action, targetType, targetID, c.ClientIP(), detail)
	}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "参数错误")
		return
	}
	res, err := h.auth.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.audit(c, nil, biz.AuditLoginFailed, "auth", req.Username, map[string]any{"reason": err.Error()})
		httputil.Unauthorized(c, err.Error())
		return
	}
	h.audit(c, &res.UserID, biz.AuditLogin, "auth", req.Username, nil)
	// HttpOnly Cookie, 7 天
	c.SetCookie(biz.SessionCookieName, res.SessionID, 7*24*3600, "/", "", false, true)
	httputil.OK(c, gin.H{
		"userId": res.UserID, "username": res.Username,
		"displayName": res.DisplayName, "mustChangePwd": res.MustChangePwd,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	uid := CurrentUserID(c)
	_ = h.auth.Logout(c.Request.Context(), CurrentSessionID(c))
	h.audit(c, &uid, biz.AuditLogout, "auth", "", nil)
	c.SetCookie(biz.SessionCookieName, "", -1, "/", "", false, true)
	httputil.OK(c, gin.H{})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid := CurrentUserID(c)
	user, err := h.auth.Me(c.Request.Context(), uid)
	if err != nil {
		httputil.Unauthorized(c, "会话无效")
		return
	}
	httputil.OK(c, gin.H{
		"userId": user.ID, "username": user.Username,
		"displayName": user.DisplayName, "avatar": user.Avatar, "email": user.Email,
	})
}

type changePwdReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "参数错误：新密码至少 6 位")
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), CurrentUserID(c), req.OldPassword, req.NewPassword); err != nil {
		httputil.BadRequest(c, err.Error())
		return
	}
	uid := CurrentUserID(c)
	h.audit(c, &uid, biz.AuditChangePassword, "auth", "", nil)
	httputil.OK(c, gin.H{})
}
