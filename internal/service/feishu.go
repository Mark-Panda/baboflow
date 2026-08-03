package service

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

// feishuStateCookie 存放 OAuth state 的短期 HttpOnly cookie（防 CSRF）。
const feishuStateCookie = "feishu_oauth_state"

// FeishuHandler 提供飞书 OAuth 登录的入口与回调（公开端点，无需已登录）。
type FeishuHandler struct {
	uc      *biz.FeishuUsecase
	auditor *biz.AuditUsecase
}

func NewFeishuHandler(uc *biz.FeishuUsecase) *FeishuHandler { return &FeishuHandler{uc: uc} }

// SetAuditor 注入审计（M7）。
func (h *FeishuHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *FeishuHandler) audit(c *gin.Context, uid *int64, action, targetID string, detail map[string]any) {
	if h.auditor != nil {
		h.auditor.Record(c.Request.Context(), uid, action, "auth", targetID, c.ClientIP(), detail)
	}
}

// Login 生成 CSRF state 写 cookie，302 跳转飞书授权页。
// GET /api/v1/auth/feishu/login
func (h *FeishuHandler) Login(c *gin.Context) {
	if !h.uc.Configured() {
		httputil.BadRequest(c, biz.ErrFeishuNotConfigured.Error())
		return
	}
	state := newOAuthState()
	// state cookie 10 分钟有效，HttpOnly，与 baboflow_sid 一致的 SameSite 默认。
	c.SetCookie(feishuStateCookie, state, 600, "/", "", false, true)
	c.Redirect(http.StatusFound, h.uc.BuildAuthURL(state))
}

// Callback 飞书授权后回跳：校验 state → 用 code 换用户并发证 → 302 回前端。
// GET /api/v1/auth/feishu/callback?code=..&state=..
func (h *FeishuHandler) Callback(c *gin.Context) {
	// 校验 state（防 CSRF），随后立即作废该 cookie。
	stateCookie, _ := c.Cookie(feishuStateCookie)
	c.SetCookie(feishuStateCookie, "", -1, "/", "", false, true)
	state := c.Query("state")
	if stateCookie == "" || state == "" || state != stateCookie {
		h.redirectLoginErr(c, "登录状态校验失败，请重试")
		return
	}
	code := c.Query("code")
	if code == "" {
		h.redirectLoginErr(c, "飞书未返回授权码")
		return
	}
	res, err := h.uc.LoginByCode(c.Request.Context(), code, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.audit(c, nil, biz.AuditLoginFailed, "feishu", map[string]any{"reason": err.Error()})
		h.redirectLoginErr(c, err.Error())
		return
	}
	h.audit(c, &res.UserID, biz.AuditLoginFeishu, res.Username, nil)
	// 与密码登录写同一个会话 cookie，后续 AuthMiddleware/WS/MCP/前端守卫自动生效。
	c.SetCookie(biz.SessionCookieName, res.SessionID, 7*24*3600, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

// redirectLoginErr 登录失败时回跳前端登录页并带上错误信息。
func (h *FeishuHandler) redirectLoginErr(c *gin.Context, msg string) {
	c.Redirect(http.StatusFound, "/login?err="+url.QueryEscape(msg))
}

// newOAuthState 生成 128-bit 随机 state。
func newOAuthState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
