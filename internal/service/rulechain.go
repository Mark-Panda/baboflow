package service

import (
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/biz/rulegokit"
	"baboflow/internal/server/httputil"
)

type RuleChainHandler struct {
	uc      *biz.RuleChainUsecase
	auditor *biz.AuditUsecase
}

func NewRuleChainHandler(uc *biz.RuleChainUsecase) *RuleChainHandler {
	return &RuleChainHandler{uc: uc}
}

// SetAuditor 注入审计（M7）。
func (h *RuleChainHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *RuleChainHandler) audit(c *gin.Context, action, targetID string, detail map[string]any) {
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, action, "rule_chain", targetID, c.ClientIP(), detail)
	}
}

func (h *RuleChainHandler) chainErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "规则链不存在")
	case errors.Is(err, biz.ErrChainPublished):
		httputil.Conflict(c, err.Error())
	case errors.Is(err, biz.ErrChainNotLoaded):
		httputil.Conflict(c, err.Error())
	default:
		httputil.BadRequest(c, err.Error())
	}
}

// ---- CRUD ----

func (h *RuleChainHandler) List(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	status := c.Query("status")
	keyword := c.Query("keyword")
	list, total, err := h.uc.List(c.Request.Context(), status, keyword, page, pageSize)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OKPage(c, list, total, page, pageSize)
}

func (h *RuleChainHandler) Create(c *gin.Context) {
	var in biz.ChainInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	chain, err := h.uc.Create(c.Request.Context(), &in, CurrentUserID(c))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, chain)
}

func (h *RuleChainHandler) Get(c *gin.Context) {
	chain, err := h.uc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, chain)
}

func (h *RuleChainHandler) Update(c *gin.Context) {
	var in biz.ChainInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.Update(c.Request.Context(), c.Param("id"), &in); err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *RuleChainHandler) Delete(c *gin.Context) {
	if err := h.uc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.chainErr(c, err)
		return
	}
	h.audit(c, biz.AuditChainDelete, c.Param("id"), nil)
	httputil.OK(c, gin.H{"ok": true})
}

// ---- 校验 ----

type validateReq struct {
	DSL json.RawMessage `json:"dsl" binding:"required"`
}

func (h *RuleChainHandler) Validate(c *gin.Context) {
	var in validateReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := rulegokit.Validate(in.DSL); err != nil {
		httputil.OK(c, gin.H{"valid": false, "error": err.Error()})
		return
	}
	httputil.OK(c, gin.H{"valid": true})
}

// ---- 发布 / 撤销 / 版本 ----

func (h *RuleChainHandler) Publish(c *gin.Context) {
	ver, err := h.uc.Publish(c.Request.Context(), c.Param("id"), CurrentUserID(c))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	h.audit(c, biz.AuditChainPublish, c.Param("id"), map[string]any{"version": ver})
	httputil.OK(c, gin.H{"version": ver})
}

func (h *RuleChainHandler) Offline(c *gin.Context) {
	if err := h.uc.Offline(c.Request.Context(), c.Param("id")); err != nil {
		h.chainErr(c, err)
		return
	}
	h.audit(c, biz.AuditChainOffline, c.Param("id"), nil)
	httputil.OK(c, gin.H{"ok": true})
}

func (h *RuleChainHandler) Versions(c *gin.Context) {
	list, err := h.uc.Versions(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

type rollbackReq struct {
	Version int `json:"version" binding:"required"`
}

func (h *RuleChainHandler) Rollback(c *gin.Context) {
	var in rollbackReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.Rollback(c.Request.Context(), c.Param("id"), in.Version, CurrentUserID(c)); err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

// ---- 导入 / 导出 ----

func (h *RuleChainHandler) Export(c *gin.Context) {
	out, err := h.uc.Export(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, out)
}

func (h *RuleChainHandler) Import(c *gin.Context) {
	var in biz.ChainExport
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	chain, err := h.uc.Import(c.Request.Context(), &in, CurrentUserID(c))
	if err != nil {
		h.chainErr(c, err)
		return
	}
	h.audit(c, biz.AuditChainImport, chain.ID, map[string]any{"name": chain.Name})
	httputil.OK(c, chain)
}

// ---- 运行 / 调试 ----

func (h *RuleChainHandler) Run(c *gin.Context) {
	var in biz.RunInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	view, err := h.uc.Run(c.Request.Context(), c.Param("id"), &in, "manual")
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, view)
}

func (h *RuleChainHandler) Debug(c *gin.Context) {
	var in biz.RunInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	view, err := h.uc.Debug(c.Request.Context(), c.Param("id"), &in)
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, view)
}

// ---- 运行日志 ----

func (h *RuleChainHandler) Runs(c *gin.Context) {
	page, pageSize := httputil.PageParams(c)
	chainID := c.Query("chainId")
	status := c.Query("status")
	list, total, err := h.uc.ListRuns(c.Request.Context(), chainID, status, page, pageSize)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OKPage(c, list, total, page, pageSize)
}

func (h *RuleChainHandler) RunDetail(c *gin.Context) {
	runID, ok := pathID(c, "runId")
	if !ok {
		return
	}
	run, err := h.uc.GetRun(c.Request.Context(), runID)
	if err != nil {
		h.chainErr(c, err)
		return
	}
	httputil.OK(c, run)
}
