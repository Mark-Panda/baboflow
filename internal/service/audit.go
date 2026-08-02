package service

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type AuditHandler struct {
	uc *biz.AuditUsecase
}

func NewAuditHandler(uc *biz.AuditUsecase) *AuditHandler {
	return &AuditHandler{uc: uc}
}

// List GET /audit?action=&userId=&page=&pageSize= （仅 admin 路由挂载）
func (h *AuditHandler) List(c *gin.Context) {
	action := c.Query("action")
	var userID *int64
	if v := c.Query("userId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			userID = &n
		}
	}
	page, pageSize := httputil.PageParams(c)
	list, total, err := h.uc.List(c.Request.Context(), action, userID, page, pageSize)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OKPage(c, list, total, page, pageSize)
}
