package service

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type AgentHandler struct {
	uc *biz.AgentUsecase
}

const maxAssetUploadBytes = 20 << 20

func NewAgentHandler(uc *biz.AgentUsecase) *AgentHandler {
	return &AgentHandler{uc: uc}
}

func (h *AgentHandler) agentErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "资源不存在")
	default:
		httputil.BadRequest(c, err.Error())
	}
}

// ---- Agent CRUD ----

func (h *AgentHandler) List(c *gin.Context) {
	list, err := h.uc.List(c.Request.Context(), c.Query("keyword"))
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *AgentHandler) Get(c *gin.Context) {
	v, err := h.uc.Get(c.Request.Context(), c.Param("key"))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, v)
}

type createAgentReq struct {
	Key string `json:"key" binding:"required"`
	biz.AgentInput
}

func (h *AgentHandler) Create(c *gin.Context) {
	var in createAgentReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	v, err := h.uc.Create(c.Request.Context(), in.Key, &in.AgentInput)
	if err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, v)
}

func (h *AgentHandler) Update(c *gin.Context) {
	var in biz.AgentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.Update(c.Request.Context(), c.Param("key"), &in); err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *AgentHandler) Delete(c *gin.Context) {
	if err := h.uc.Delete(c.Request.Context(), c.Param("key")); err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

// ---- 会话 ----

func (h *AgentHandler) ListSessions(c *gin.Context) {
	list, err := h.uc.ListSessions(c.Request.Context(), c.Param("key"), CurrentUserID(c))
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

type createSessionReq struct {
	Title string `json:"title"`
}

func (h *AgentHandler) CreateSession(c *gin.Context) {
	var in createSessionReq
	_ = c.ShouldBindJSON(&in)
	s, err := h.uc.CreateSession(c.Request.Context(), c.Param("key"), in.Title, CurrentUserID(c))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, s)
}

func (h *AgentHandler) DeleteSession(c *gin.Context) {
	if err := h.uc.DeleteSession(c.Request.Context(), c.Param("sessionId"), CurrentUserID(c)); err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *AgentHandler) ListMessages(c *gin.Context) {
	list, err := h.uc.ListMessages(c.Request.Context(), c.Param("sessionId"), CurrentUserID(c))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

// ---- 附件 ----

// UploadAsset 处理 multipart 文件上传（字段 file，query sessionId）。
func (h *AgentHandler) UploadAsset(c *gin.Context) {
	sessionID := c.PostForm("sessionId")
	if sessionID == "" {
		httputil.BadRequest(c, "缺少 sessionId")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		httputil.BadRequest(c, "缺少文件: "+err.Error())
		return
	}
	if fh.Size > maxAssetUploadBytes {
		httputil.BadRequest(c, "文件超过 20MB 上限")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAssetUploadBytes+1))
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	if len(data) > maxAssetUploadBytes {
		httputil.BadRequest(c, "文件超过 20MB 上限")
		return
	}
	asset, err := h.uc.SaveAsset(c.Request.Context(), sessionID, fh.Filename, data, CurrentUserID(c))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	httputil.OK(c, asset)
}

// GetAsset 下载附件内容。
func (h *AgentHandler) GetAsset(c *gin.Context) {
	id, ok := pathID(c, "assetId")
	if !ok {
		return
	}
	asset, data, err := h.uc.GetAssetData(c.Request.Context(), id, CurrentUserID(c))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	c.Header("Content-Type", asset.Mime)
	c.Header("Content-Disposition", "inline; filename=\""+asset.Name+"\"")
	c.Data(200, asset.Mime, data)
}
