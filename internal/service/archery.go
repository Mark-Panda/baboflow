package service

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

// ArcheryHandler 提供 Archery 连接的管理接口（凭据加密存库，密码脱敏回显）。
type ArcheryHandler struct {
	uc      *biz.ArcheryUsecase
	auditor *biz.AuditUsecase
}

func NewArcheryHandler(uc *biz.ArcheryUsecase) *ArcheryHandler { return &ArcheryHandler{uc: uc} }

// SetAuditor 注入审计（M7）。
func (h *ArcheryHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *ArcheryHandler) audit(c *gin.Context, action, targetID string, detail map[string]any) {
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, action, "archery", targetID, c.ClientIP(), detail)
	}
}

func (h *ArcheryHandler) ListConnections(c *gin.Context) {
	list, err := h.uc.ListConnections(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *ArcheryHandler) GetConnection(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	view, err := h.uc.GetConnection(c.Request.Context(), id)
	if err != nil {
		httputil.NotFound(c, "连接不存在")
		return
	}
	httputil.OK(c, view)
}

func (h *ArcheryHandler) CreateConnection(c *gin.Context) {
	var in biz.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误：name/endpoint/username 必填")
		return
	}
	conn, err := h.uc.CreateConnection(c.Request.Context(), &in)
	if err != nil {
		httputil.BadRequest(c, err.Error())
		return
	}
	h.audit(c, biz.AuditArcheryCreate, strconv.FormatInt(conn.ID, 10), map[string]any{"name": in.Name})
	httputil.OK(c, gin.H{"id": conn.ID})
}

func (h *ArcheryHandler) UpdateConnection(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误")
		return
	}
	if err := h.uc.UpdateConnection(c.Request.Context(), id, &in); err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditArcheryUpdate, strconv.FormatInt(id, 10), map[string]any{"name": in.Name})
	httputil.OK(c, gin.H{})
}

func (h *ArcheryHandler) DeleteConnection(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.DeleteConnection(c.Request.Context(), id); err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditArcheryDelete, strconv.FormatInt(id, 10), nil)
	httputil.OK(c, gin.H{})
}

// TestConnection 验证连接可用（登录 + 列库），供前端"测试连接"按钮。
func (h *ArcheryHandler) TestConnection(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	res, err := h.uc.TestConnection(c.Request.Context(), id)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, res)
}

// ListInstances 返回某连接下已同步的实例。
func (h *ArcheryHandler) ListInstances(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.uc.ListInstances(c.Request.Context(), id)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

// SyncInstances 重新从 Archery 拉取该连接下所有实例并 upsert（更新/新建/清理），
// 供前端"更新实例"按钮。
func (h *ArcheryHandler) SyncInstances(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.uc.SyncInstances(c.Request.Context(), id)
	if err != nil {
		httputil.BadRequest(c, err.Error())
		return
	}
	h.audit(c, biz.AuditArcheryUpdate, strconv.FormatInt(id, 10), map[string]any{"syncInstances": len(list)})
	httputil.OK(c, gin.H{"list": list})
}
