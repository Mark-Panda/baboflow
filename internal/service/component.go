package service

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/server/httputil"
)

type ComponentHandler struct {
	repo biz.ComponentRepo
	sync *biz.ComponentSync
}

func NewComponentHandler(repo biz.ComponentRepo, sync *biz.ComponentSync) *ComponentHandler {
	return &ComponentHandler{repo: repo, sync: sync}
}

// componentView 返回给前端的组件视图（configSchema 直接透传 RuleGo ComponentForm）。
type componentView struct {
	Type         string          `json:"type"`
	Name         string          `json:"name"`
	Category     string          `json:"category"`
	Description  string          `json:"description"`
	ConfigSchema json.RawMessage `json:"configSchema"`
	Example      json.RawMessage `json:"example"`
}

func (h *ComponentHandler) List(c *gin.Context) {
	category := c.Query("category")
	keyword := c.Query("keyword")
	list, err := h.repo.SearchKeyword(c.Request.Context(), category, keyword)
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	out := make([]componentView, 0, len(list))
	for _, m := range list {
		out = append(out, componentView{
			Type: m.Type, Name: m.Name, Category: m.Category,
			Description: m.Description,
			ConfigSchema: json.RawMessage(m.ConfigSchema),
			Example:      json.RawMessage(m.Example),
		})
	}
	httputil.OK(c, gin.H{"list": out})
}

func (h *ComponentHandler) SyncStatus(c *gin.Context) {
	httputil.OK(c, h.sync.Last())
}

func (h *ComponentHandler) TriggerSync(c *gin.Context) {
	res, err := h.sync.Run(c.Request.Context())
	if err != nil {
		httputil.Internal(c, err.Error())
		return
	}
	httputil.OK(c, res)
}
