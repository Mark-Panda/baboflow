package service

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type SkillHandler struct {
	uc      *biz.SkillUsecase
	auditor *biz.AuditUsecase
}

func NewSkillHandler(uc *biz.SkillUsecase) *SkillHandler {
	return &SkillHandler{uc: uc}
}

// SetAuditor 注入审计（M7）。
func (h *SkillHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *SkillHandler) skillErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "SKILL 不存在")
	default:
		httputil.BadRequest(c, err.Error())
	}
}

func (h *SkillHandler) List(c *gin.Context) {
	list, err := h.uc.List(c.Request.Context(), c.Query("source"), c.Query("keyword"))
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *SkillHandler) Get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	v, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	httputil.OK(c, v)
}

type uploadSkillReq struct {
	Content string `json:"content" binding:"required"`
	Source  string `json:"source"`
}

func (h *SkillHandler) Upload(c *gin.Context) {
	var in uploadSkillReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	v, err := h.uc.Upload(c.Request.Context(), in.Content, in.Source)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	httputil.OK(c, v)
}

func (h *SkillHandler) Delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.Delete(c.Request.Context(), id); err != nil {
		h.skillErr(c, err)
		return
	}
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, biz.AuditSkillDelete, "skill", "", c.ClientIP(), map[string]any{"skillId": id})
	}
	httputil.OK(c, gin.H{"ok": true})
}

// Generate 从已发布链反生成 SKILL（Agent2）。
func (h *SkillHandler) Generate(c *gin.Context) {
	chainID := c.Param("id")
	v, err := h.uc.GenerateFromChain(c.Request.Context(), chainID)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	httputil.OK(c, v)
}
