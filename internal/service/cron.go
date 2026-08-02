package service

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type CronHandler struct {
	uc *biz.CronUsecase
}

func NewCronHandler(uc *biz.CronUsecase) *CronHandler {
	return &CronHandler{uc: uc}
}

func (h *CronHandler) cronErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "定时任务不存在")
	default:
		httputil.BadRequest(c, err.Error())
	}
}

func (h *CronHandler) List(c *gin.Context) {
	list, err := h.uc.List(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *CronHandler) Create(c *gin.Context) {
	var in biz.CronInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	j, err := h.uc.Create(c.Request.Context(), &in)
	if err != nil {
		h.cronErr(c, err)
		return
	}
	httputil.OK(c, j)
}

func (h *CronHandler) Update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.CronInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.Update(c.Request.Context(), id, &in); err != nil {
		h.cronErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *CronHandler) Delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.Delete(c.Request.Context(), id); err != nil {
		h.cronErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *CronHandler) Toggle(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	j, err := h.uc.Toggle(c.Request.Context(), id)
	if err != nil {
		h.cronErr(c, err)
		return
	}
	httputil.OK(c, j)
}
