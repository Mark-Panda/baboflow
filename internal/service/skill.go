package service

import (
	"errors"
	"io"
	"mime"
	"strings"

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
	var internalErr *biz.SkillInternalError
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		httputil.NotFound(c, "SKILL 不存在")
	case errors.As(err, &internalErr):
		httputil.Internal(c, err.Error())
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

// skillPackageMaxBytes 上传技能包大小上限（与 biz 侧一致）。
const skillPackageMaxBytes = 20 << 20 // 20MB

// UploadPackage 处理技能包（.zip）multipart 上传（字段 file）。
func (h *SkillHandler) UploadPackage(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		httputil.BadRequest(c, "缺少文件: "+err.Error())
		return
	}
	if !strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
		httputil.BadRequest(c, "仅支持 .zip 技能包")
		return
	}
	if fh.Size > skillPackageMaxBytes {
		httputil.BadRequest(c, "技能包超过大小上限 20MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, skillPackageMaxBytes+1))
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	v, err := h.uc.UploadPackage(c.Request.Context(), data, c.PostForm("source"))
	if err != nil {
		h.skillErr(c, err)
		return
	}
	if h.auditor != nil {
		uid := CurrentUserID(c)
		h.auditor.Record(c.Request.Context(), &uid, biz.AuditSkillUpload, "skill", "", c.ClientIP(), map[string]any{"skillId": v.ID, "name": v.Name, "package": true})
	}
	httputil.OK(c, v)
}

// ListFiles 列出技能包内文件清单。
func (h *SkillHandler) ListFiles(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	list, err := h.uc.ListFiles(c.Request.Context(), id)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"list": list})
}

// ReadFile 读取技能包内单个文本文件内容（query path）。
func (h *SkillHandler) ReadFile(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	relPath := c.Query("path")
	if strings.TrimSpace(relPath) == "" {
		httputil.BadRequest(c, "缺少 path")
		return
	}
	content, err := h.uc.ReadFile(c.Request.Context(), id, relPath)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	httputil.OK(c, gin.H{"path": relPath, "content": content})
}

// DownloadPackage 下载技能包 zip 归档。
func (h *SkillHandler) DownloadPackage(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	name, data, err := h.uc.DownloadPackage(c.Request.Context(), id)
	if err != nil {
		h.skillErr(c, err)
		return
	}
	c.Header("Content-Type", "application/zip")
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		disposition = `attachment; filename="skill.zip"`
	}
	c.Header("Content-Disposition", disposition)
	c.Data(200, "application/zip", data)
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
