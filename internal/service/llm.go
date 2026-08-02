package service

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type LLMHandler struct {
	uc      *biz.LLMUsecase
	auditor *biz.AuditUsecase
}

func NewLLMHandler(uc *biz.LLMUsecase) *LLMHandler { return &LLMHandler{uc: uc} }

// SetAuditor 注入审计（M7）。
func (h *LLMHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *LLMHandler) audit(c *gin.Context, action, targetID string, detail map[string]any) {
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, action, "llm", targetID, c.ClientIP(), detail)
	}
}

func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		httputil.BadRequest(c, "非法 id")
		return 0, false
	}
	return id, true
}

// ---- Provider ----

func (h *LLMHandler) ListProviders(c *gin.Context) {
	list, err := h.uc.ListProviders(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *LLMHandler) CreateProvider(c *gin.Context) {
	var in biz.ProviderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误：name/baseUrl 必填")
		return
	}
	p, err := h.uc.CreateProvider(c.Request.Context(), &in)
	if err != nil {
		if errors.Is(err, biz.ErrInvalidBaseURL) {
			httputil.BadRequest(c, err.Error())
			return
		}
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMCreate, strconv.FormatInt(p.ID, 10), map[string]any{"name": in.Name, "type": "provider"})
	httputil.OK(c, gin.H{"id": p.ID})
}

func (h *LLMHandler) UpdateProvider(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.ProviderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误")
		return
	}
	if err := h.uc.UpdateProvider(c.Request.Context(), id, &in); err != nil {
		if errors.Is(err, biz.ErrInvalidBaseURL) {
			httputil.BadRequest(c, err.Error())
			return
		}
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMUpdate, strconv.FormatInt(id, 10), map[string]any{"name": in.Name, "type": "provider"})
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) DeleteProvider(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.DeleteProvider(c.Request.Context(), id); err != nil {
		if err == biz.ErrReferenced {
			httputil.Conflict(c, err.Error())
			return
		}
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMDelete, strconv.FormatInt(id, 10), map[string]any{"type": "provider"})
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) TestProvider(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	res, err := h.uc.TestProvider(c.Request.Context(), id)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, res)
}

func (h *LLMHandler) RemoteModels(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	models, err := h.uc.ProviderModels(c.Request.Context(), id)
	if err != nil {
		httputil.BadRequest(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"models": models})
}

// ---- Model ----

func (h *LLMHandler) ListModels(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.uc.ListModels(c.Request.Context(), id)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

type createModelsReq struct {
	Models []biz.ModelInput `json:"models" binding:"required"`
}

func (h *LLMHandler) CreateModels(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var req createModelsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Models) == 0 {
		httputil.BadRequest(c, "参数错误：models 必填")
		return
	}
	if err := h.uc.CreateModels(c.Request.Context(), id, req.Models); err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMCreate, strconv.FormatInt(id, 10), map[string]any{"type": "model", "count": len(req.Models)})
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) UpdateModel(c *gin.Context) {
	id, ok := pathID(c, "modelId")
	if !ok {
		return
	}
	var in biz.ModelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误")
		return
	}
	if err := h.uc.UpdateModel(c.Request.Context(), id, &in); err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMUpdate, strconv.FormatInt(id, 10), map[string]any{"type": "model", "model": in.Model})
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) DeleteModel(c *gin.Context) {
	id, ok := pathID(c, "modelId")
	if !ok {
		return
	}
	if err := h.uc.DeleteModel(c.Request.Context(), id); err != nil {
		if err == biz.ErrReferenced {
			httputil.Conflict(c, err.Error())
			return
		}
		httputil.Internal(c, err.Error())
		return
	}
	h.audit(c, biz.AuditLLMDelete, strconv.FormatInt(id, 10), map[string]any{"type": "model"})
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) SetDefaultModel(c *gin.Context) {
	id, ok := pathID(c, "modelId")
	if !ok {
		return
	}
	if err := h.uc.SetDefaultModel(c.Request.Context(), id); err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{})
}

func (h *LLMHandler) TestModel(c *gin.Context) {
	id, ok := pathID(c, "modelId")
	if !ok {
		return
	}
	res, err := h.uc.TestModel(c.Request.Context(), id)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, res)
}
