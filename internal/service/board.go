package service

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type BoardHandler struct {
	uc      *biz.BoardUsecase
	auditor *biz.AuditUsecase
}

func NewBoardHandler(uc *biz.BoardUsecase) *BoardHandler {
	return &BoardHandler{uc: uc}
}

// SetAuditor 注入审计（M7）。
func (h *BoardHandler) SetAuditor(a *biz.AuditUsecase) { h.auditor = a }

func (h *BoardHandler) boardErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "资源不存在")
	default:
		httputil.BadRequest(c, err.Error())
	}
}

// ---- board ----

func (h *BoardHandler) ListBoards(c *gin.Context) {
	list, err := h.uc.ListBoards(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

func (h *BoardHandler) CreateBoard(c *gin.Context) {
	var in biz.BoardInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	b, err := h.uc.CreateBoard(c.Request.Context(), &in)
	if err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, b)
}

func (h *BoardHandler) UpdateBoard(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.BoardInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.UpdateBoard(c.Request.Context(), id, &in); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *BoardHandler) DeleteBoard(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.DeleteBoard(c.Request.Context(), id); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *BoardHandler) GetBoard(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	detail, err := h.uc.GetBoardDetail(c.Request.Context(), id)
	if err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, detail)
}

// ---- column ----

func (h *BoardHandler) CreateColumn(c *gin.Context) {
	boardID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.ColumnInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	col, err := h.uc.CreateColumn(c.Request.Context(), boardID, &in)
	if err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, col)
}

func (h *BoardHandler) UpdateColumn(c *gin.Context) {
	id, ok := pathID(c, "cid")
	if !ok {
		return
	}
	var in biz.ColumnInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.UpdateColumn(c.Request.Context(), id, &in); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *BoardHandler) DeleteColumn(c *gin.Context) {
	id, ok := pathID(c, "cid")
	if !ok {
		return
	}
	if err := h.uc.DeleteColumn(c.Request.Context(), id); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

// ---- task ----

func (h *BoardHandler) CreateTask(c *gin.Context) {
	columnID, ok := pathID(c, "cid")
	if !ok {
		return
	}
	var in biz.TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	t, err := h.uc.CreateTask(c.Request.Context(), columnID, &in)
	if err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, t)
}

func (h *BoardHandler) UpdateTask(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in biz.TaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.UpdateTask(c.Request.Context(), id, &in); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *BoardHandler) DeleteTask(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := h.uc.DeleteTask(c.Request.Context(), id); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

type moveTaskReq struct {
	ToColumnID int64 `json:"toColumnId" binding:"required"`
	ToSort     int64 `json:"toSort"`
}

func (h *BoardHandler) MoveTask(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in moveTaskReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httputil.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.uc.MoveTask(c.Request.Context(), id, in.ToColumnID, in.ToSort); err != nil {
		h.boardErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"ok": true})
}

func (h *BoardHandler) TriggerTask(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	t, err := h.uc.TriggerTask(c.Request.Context(), id)
	if err != nil {
		h.boardErr(c, err)
		return
	}
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, biz.AuditTaskTrigger, "task", t.AssignedChainID, c.ClientIP(), map[string]any{"taskId": id, "title": t.Title, "status": t.Status})
	}
	httputil.OK(c, t)
}
