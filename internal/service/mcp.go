package service

import (
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
	"baboflow/internal/server/httputil"
)

type McpHandler struct {
	uc      *biz.McpUsecase
	builder *agentkit.McpClientBuilder
	auditor *biz.AuditUsecase
}

func NewMcpHandler(uc *biz.McpUsecase, builder *agentkit.McpClientBuilder) *McpHandler {
	return &McpHandler{uc: uc, builder: builder}
}

// SetAuditor 注入审计（M7）。
func (h *McpHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *McpHandler) audit(c *gin.Context, action, targetID string, detail map[string]any) {
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, action, "mcp", targetID, c.ClientIP(), detail)
	}
}

func (h *McpHandler) mcpErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "资源不存在")
	default:
		httputil.BadRequest(c, err.Error())
	}
}

// ---- server 配置 ----

func (h *McpHandler) ListServers(c *gin.Context) {
	list, err := h.uc.ListServers(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *McpHandler) CreateServer(c *gin.Context) {
	var in biz.McpServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	s, err := h.uc.CreateServer(c.Request.Context(), &in)
	if err != nil {
		h.mcpErr(c, err)
		return
	}
	httputil.OK(c, s)
}

func (h *McpHandler) UpdateServer(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.McpServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.UpdateServer(c.Request.Context(), id, &in); err != nil {
		h.mcpErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *McpHandler) DeleteServer(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.DeleteServer(c.Request.Context(), id); err != nil {
		h.mcpErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *McpHandler) ToggleServer(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	s, err := h.uc.ToggleServer(c.Request.Context(), id)
	if err != nil {
		h.mcpErr(c, err)
		return
	}
	httputil.OK(c, s)
}

// TestServer 连通性检测：连接并列出远程工具。
func (h *McpHandler) TestServer(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	s, err := h.uc.GetServerForTest(c.Request.Context(), id)
	if err != nil {
		h.mcpErr(c, err)
		return
	}
	tools, err := h.builder.ListToolNames(c.Request.Context(), s)
	if err != nil {
		httputil.OK(c, gin.H{"ok": false, "error": err.Error(), "tools": []string{}})
		return
	}
	httputil.OK(c, gin.H{"ok": true, "tools": tools})
}

// ---- exposure ----

func (h *McpHandler) ListExposures(c *gin.Context) {
	list, err := h.uc.ListExposures(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

type exposeReq struct {
	ToolName    string          `json:"toolName" binding:"required"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func (h *McpHandler) Expose(c *gin.Context) {
	chainID := c.Param("id")
	var in exposeReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	e, err := h.uc.ExposeChain(c.Request.Context(), chainID, in.ToolName, in.Description, in.InputSchema)
	if err != nil {
		h.mcpErr(c, err)
		return
	}
	h.audit(c, biz.AuditMcpExpose, chainID, map[string]any{"toolName": e.ToolName})
	httputil.OK(c, gin.H{"id": e.ID, "toolName": e.ToolName, "mcpEndpoint": "/mcp"})
}

func (h *McpHandler) RemoveExposure(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.RemoveExposure(c.Request.Context(), id); err != nil {
		h.mcpErr(c, err)
		return
	}
	h.audit(c, biz.AuditMcpRemove, "", map[string]any{"exposureId": id})
	httputil.OK(c, gin.H{"ok": true})
}
