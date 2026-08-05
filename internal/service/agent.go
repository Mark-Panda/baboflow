package service

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"baboflow/internal/biz"
)

type AgentHandler struct {
	uc *biz.AgentUsecase
}

const maxAssetUploadBytes = 20 << 20

type agentAssetResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Size      string `json:"size"`
	SessionID string `json:"sessionId"`
	CreatedAt string `json:"createdAt"`
}

func NewAgentHandler(uc *biz.AgentUsecase) *AgentHandler {
	return &AgentHandler{uc: uc}
}

func (h *AgentHandler) agentErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, biz.ErrNotFound):
		ginError(c, http.StatusNotFound, "资源不存在")
	default:
		ginError(c, http.StatusBadRequest, err.Error())
	}
}

// UploadAsset 处理 multipart 文件上传（字段 file，query sessionId）。
func (h *AgentHandler) UploadAsset(c *gin.Context) {
	sessionID := c.PostForm("sessionId")
	if sessionID == "" {
		ginError(c, http.StatusBadRequest, "缺少 sessionId")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		ginError(c, http.StatusBadRequest, "缺少文件: "+err.Error())
		return
	}
	if fh.Size > maxAssetUploadBytes {
		ginError(c, http.StatusBadRequest, "文件超过 20MB 上限")
		return
	}
	f, err := fh.Open()
	if err != nil {
		ginError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAssetUploadBytes+1))
	if err != nil {
		ginError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if len(data) > maxAssetUploadBytes {
		ginError(c, http.StatusBadRequest, "文件超过 20MB 上限")
		return
	}
	asset, err := h.uc.SaveAsset(c.Request.Context(), sessionID, fh.Filename, data, CurrentUserID(c))
	if err != nil {
		h.agentErr(c, err)
		return
	}
	c.JSON(http.StatusOK, agentAssetResponse{
		ID:        strconv.FormatInt(asset.ID, 10),
		Name:      asset.Name,
		Mime:      asset.Mime,
		Size:      strconv.FormatInt(asset.Size, 10),
		SessionID: asset.SessionID,
		CreatedAt: asset.CreatedAt.Format(time.RFC3339Nano),
	})
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
	c.Data(http.StatusOK, asset.Mime, data)
}
