package service

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
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
		ginError(c, http.StatusNotFound, "SKILL 不存在")
	case errors.As(err, &internalErr):
		ginError(c, http.StatusInternalServerError, err.Error())
	default:
		ginError(c, http.StatusBadRequest, err.Error())
	}
}

// skillPackageMaxBytes 上传技能包大小上限（与 biz 侧一致）。
const skillPackageMaxBytes = 20 << 20 // 20MB

// UploadPackage 处理技能包（.zip）multipart 上传（字段 file）。
func (h *SkillHandler) UploadPackage(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		ginError(c, http.StatusBadRequest, "缺少文件: "+err.Error())
		return
	}
	if !strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
		ginError(c, http.StatusBadRequest, "仅支持 .zip 技能包")
		return
	}
	if fh.Size > skillPackageMaxBytes {
		ginError(c, http.StatusBadRequest, "技能包超过大小上限 20MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		ginError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, skillPackageMaxBytes+1))
	if err != nil {
		ginError(c, http.StatusInternalServerError, err.Error())
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
	c.JSON(http.StatusOK, v)
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
	c.Data(http.StatusOK, "application/zip", data)
}
