package biz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"baboflow/internal/biz/agentkit"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

// AgentDataRepo Agent 域持久化接口（data 层实现）。
type AgentDataRepo interface {
	// agent CRUD
	ListAgents(ctx context.Context, keyword string) ([]po.Agent, error)
	GetAgentByKey(ctx context.Context, key string) (*po.Agent, error)
	CreateAgent(ctx context.Context, a *po.Agent) error
	UpdateAgent(ctx context.Context, a *po.Agent) error
	DeleteAgent(ctx context.Context, id int64) error
	SetSubAgents(ctx context.Context, parentID int64, childIDs []int64) error
	ListSubAgentIDs(ctx context.Context, parentID int64) ([]int64, error)

	// session
	CreateSession(ctx context.Context, s *po.AgentSession) error
	GetSession(ctx context.Context, id string) (*po.AgentSession, error)
	ListSessions(ctx context.Context, agentKey string, userID int64) ([]po.AgentSession, error)
	UpdateSessionTitle(ctx context.Context, id, title string) error
	DeleteSession(ctx context.Context, id string) error

	// message
	CreateMessage(ctx context.Context, m *po.AgentMessage) error
	ListMessages(ctx context.Context, sessionID string, limit int) ([]po.AgentMessage, error)

	// asset
	CreateAsset(ctx context.Context, a *po.Asset) error
	GetAsset(ctx context.Context, id int64) (*po.Asset, error)
}

// AgentUsecase 聚合 Agent 配置、会话与对话运行。
type AgentUsecase struct {
	repo    AgentDataRepo
	manager *agentkit.Manager
	cfg     *conf.Config
	assets  AssetStore
	tracer  *agentkit.Tracer
	memory  SessionMemoryCleaner
}

// AssetStore 抽象会话附件的落盘（本地实现于 data 层）。
type AssetStore interface {
	Save(sessionID string, name string, data []byte) (relPath string, err error)
	Read(relPath string) ([]byte, error)
	DeleteBySession(sessionID string) error
}

// SessionMemoryCleaner 清理指定用户会话在记忆存储中的会话数据。
type SessionMemoryCleaner interface {
	DeleteSessionData(ctx context.Context, userID, sessionID string) error
}

func NewAgentUsecase(repo AgentDataRepo, manager *agentkit.Manager, c *conf.Config, assets AssetStore, tracer *agentkit.Tracer) *AgentUsecase {
	return &AgentUsecase{repo: repo, manager: manager, cfg: c, assets: assets, tracer: tracer}
}

func (uc *AgentUsecase) SetSessionMemoryCleaner(cleaner SessionMemoryCleaner) {
	uc.memory = cleaner
}

// ---- Agent CRUD ----

type AgentView struct {
	ID           int64     `json:"id"`
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Instruction  string    `json:"instruction"`
	LLMModelID   *int64    `json:"llmModelId"`
	SkillIDs     []int64   `json:"skillIds"`
	McpIDs       []int64   `json:"mcpIds"`
	BuiltinTools []string  `json:"builtinTools"`
	SubAgentIDs  []int64   `json:"subAgentIds"`
	IsBuiltin    bool      `json:"isBuiltin"`
	Enabled      bool      `json:"enabled"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (uc *AgentUsecase) toView(ctx context.Context, a *po.Agent) *AgentView {
	v := &AgentView{
		ID: a.ID, Key: a.Key, Name: a.Name, Instruction: a.Instruction,
		LLMModelID: a.LLMModelID, IsBuiltin: a.IsBuiltin, Enabled: a.Enabled, UpdatedAt: a.UpdatedAt,
	}
	_ = json.Unmarshal(a.SkillIDs, &v.SkillIDs)
	_ = json.Unmarshal(a.McpIDs, &v.McpIDs)
	_ = json.Unmarshal(a.BuiltinTools, &v.BuiltinTools)
	if ids, err := uc.repo.ListSubAgentIDs(ctx, a.ID); err == nil {
		v.SubAgentIDs = ids
	}
	return v
}

func (uc *AgentUsecase) List(ctx context.Context, keyword string) ([]AgentView, error) {
	list, err := uc.repo.ListAgents(ctx, keyword)
	if err != nil {
		return nil, err
	}
	out := make([]AgentView, 0, len(list))
	for i := range list {
		out = append(out, *uc.toView(ctx, &list[i]))
	}
	return out, nil
}

func (uc *AgentUsecase) Get(ctx context.Context, key string) (*AgentView, error) {
	a, err := uc.repo.GetAgentByKey(ctx, key)
	if err != nil {
		return nil, ErrNotFound
	}
	return uc.toView(ctx, a), nil
}

type AgentInput struct {
	Name         string   `json:"name" binding:"required"`
	Instruction  string   `json:"instruction"`
	LLMModelID   *int64   `json:"llmModelId"`
	SkillIDs     []int64  `json:"skillIds"`
	McpIDs       []int64  `json:"mcpIds"`
	BuiltinTools []string `json:"builtinTools"`
	SubAgentIDs  []int64  `json:"subAgentIds"`
	Enabled      *bool    `json:"enabled"`
}

func (uc *AgentUsecase) Create(ctx context.Context, key string, in *AgentInput) (*AgentView, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("key 不能为空")
	}
	a := &po.Agent{Key: key}
	applyAgentInput(a, in)
	a.Enabled = true
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	if err := uc.repo.CreateAgent(ctx, a); err != nil {
		return nil, err
	}
	if err := uc.repo.SetSubAgents(ctx, a.ID, in.SubAgentIDs); err != nil {
		return nil, err
	}
	return uc.Get(ctx, key)
}

func (uc *AgentUsecase) Update(ctx context.Context, key string, in *AgentInput) error {
	a, err := uc.repo.GetAgentByKey(ctx, key)
	if err != nil {
		return ErrNotFound
	}
	// 内置 Agent 仅允许改挂载（技能/MCP/子 Agent）与启用开关；
	// 名称、系统提示、模型、内置工具等核心定义锁定，防止绕过前端直接调 API 篡改。
	if a.IsBuiltin {
		in.Name = a.Name
		in.Instruction = a.Instruction
		in.LLMModelID = a.LLMModelID
		in.BuiltinTools = decodeStrings(a.BuiltinTools)
	}
	applyAgentInput(a, in)
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	if err := uc.repo.UpdateAgent(ctx, a); err != nil {
		return err
	}
	if err := uc.repo.SetSubAgents(ctx, a.ID, in.SubAgentIDs); err != nil {
		return err
	}
	uc.manager.Invalidate(key)
	return nil
}

func (uc *AgentUsecase) Delete(ctx context.Context, key string) error {
	a, err := uc.repo.GetAgentByKey(ctx, key)
	if err != nil {
		return ErrNotFound
	}
	if a.IsBuiltin {
		return errors.New("内置 Agent 不可删除")
	}
	if err := uc.repo.DeleteAgent(ctx, a.ID); err != nil {
		return err
	}
	uc.manager.Invalidate(key)
	return nil
}

func applyAgentInput(a *po.Agent, in *AgentInput) {
	a.Name = in.Name
	a.Instruction = in.Instruction
	a.LLMModelID = in.LLMModelID
	a.SkillIDs = mustJSON(in.SkillIDs, "[]")
	a.McpIDs = mustJSON(in.McpIDs, "[]")
	if in.BuiltinTools == nil {
		a.BuiltinTools = datatypes.JSON([]byte(`["bash","read","write","edit","grep"]`))
	} else {
		a.BuiltinTools = mustJSON(in.BuiltinTools, "[]")
	}
}

func mustJSON(v any, def string) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte(def))
	}
	return datatypes.JSON(b)
}

// decodeStrings 把 jsonb 字符串数组解回 []string（用于内置 Agent 锁定时回填现有值）。
func decodeStrings(j datatypes.JSON) []string {
	var out []string
	_ = json.Unmarshal(j, &out)
	return out
}

// ---- 会话 ----

func (uc *AgentUsecase) ListSessions(ctx context.Context, agentKey string, userID int64) ([]po.AgentSession, error) {
	return uc.repo.ListSessions(ctx, agentKey, userID)
}

func (uc *AgentUsecase) CreateSession(ctx context.Context, agentKey, title string, userID int64) (*po.AgentSession, error) {
	if _, err := uc.repo.GetAgentByKey(ctx, agentKey); err != nil {
		return nil, ErrNotFound
	}
	if title == "" {
		title = "新会话"
	}
	s := &po.AgentSession{
		ID:       uuid.NewString(),
		AgentKey: agentKey,
		UserID:   &userID,
		Title:    title,
	}
	if err := uc.repo.CreateSession(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (uc *AgentUsecase) DeleteSession(ctx context.Context, id string, userID int64) error {
	s, err := uc.repo.GetSession(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if s.UserID != nil && *s.UserID != userID {
		return ErrNotFound
	}
	if uc.memory != nil {
		if err := uc.memory.DeleteSessionData(ctx, int64ToStr(userID), id); err != nil {
			return err
		}
	}
	if err := uc.repo.DeleteSession(ctx, id); err != nil {
		return err
	}
	// 级联清理会话工作区与附件
	if uc.assets != nil {
		_ = uc.assets.DeleteBySession(id)
	}
	return nil
}

func (uc *AgentUsecase) ListMessages(ctx context.Context, sessionID string, userID int64) ([]po.AgentMessage, error) {
	if _, err := uc.ownSession(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return uc.repo.ListMessages(ctx, sessionID, 200)
}

func (uc *AgentUsecase) ownSession(ctx context.Context, sessionID string, userID int64) (*po.AgentSession, error) {
	s, err := uc.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, ErrNotFound
	}
	if s.UserID != nil && *s.UserID != userID {
		return nil, ErrNotFound
	}
	return s, nil
}

// ValidateSessionAccess 校验用户是否有权访问指定会话，供 WebSocket 订阅鉴权使用。
func (uc *AgentUsecase) ValidateSessionAccess(ctx context.Context, sessionID string, userID int64) error {
	_, err := uc.ownSession(ctx, sessionID, userID)
	return err
}

// ---- 对话 ----

// ChatAttachment 前端随消息上传的附件（已落盘的 asset id）。
type ChatAttachment struct {
	AssetID int64  `json:"assetId"`
	Name    string `json:"name"`
	Mime    string `json:"mime"`
}

// Chat 执行一轮对话：持久化 user 消息、运行 agent 流式回调、持久化 assistant 消息。
func (uc *AgentUsecase) Chat(ctx context.Context, sessionID, text string, atts []ChatAttachment, userID int64, onEvent func(*agentkit.StreamEvent)) (*agentkit.RunResult, error) {
	sess, err := uc.ownSession(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	historyLimit := 50
	if uc.cfg != nil && uc.cfg.MemoryEnabled && uc.cfg.MemoryLimit > 0 {
		historyLimit = uc.cfg.MemoryLimit
	}
	historyRows, err := uc.repo.ListMessages(ctx, sessionID, historyLimit)
	if err != nil {
		return nil, err
	}
	history := agentHistory(historyRows)

	// 构建多模态输入（图片附件 → ImageInput）
	in := &agentkit.Input{Text: text}
	attJSON := []map[string]any{}
	for _, at := range atts {
		asset, err := uc.repo.GetAsset(ctx, at.AssetID)
		if err != nil {
			continue
		}
		if !allowAssetForSession(asset, sessionID) {
			continue
		}
		if isImageMime(asset.Mime) {
			data, err := uc.assets.Read(asset.Path)
			if err == nil {
				in.Images = append(in.Images, agentkit.ImageInput{
					Base64Data: base64Encode(data),
					MIMEType:   asset.Mime,
				})
			}
		}
		attJSON = append(attJSON, map[string]any{"assetId": asset.ID, "name": asset.Name, "mime": asset.Mime})
	}

	// 持久化 user 消息
	userMsg := &po.AgentMessage{
		SessionID:  sessionID,
		Role:       "user",
		Content:    text,
		Attachment: mustJSON(attJSON, "[]"),
	}
	if err := uc.repo.CreateMessage(ctx, userMsg); err != nil {
		return nil, err
	}

	// 运行 agent
	ag, err := uc.manager.Get(ctx, sess.AgentKey)
	if err != nil {
		return nil, err
	}
	useMemoryHistory := uc.cfg == nil || !uc.cfg.MemoryEnabled || !uc.cfg.MemorySessionSummary
	res, err := agentkit.RunWithMemoryHistory(ctx, ag, history, in, &agentkit.RunCallbacks{OnEvent: onEvent}, uc.tracer, int64ToStr(userID), sessionID, useMemoryHistory)
	if err != nil {
		// 仍把失败以 assistant 消息形式留痕
		_ = uc.repo.CreateMessage(ctx, &po.AgentMessage{
			SessionID: sessionID, Role: "assistant",
			Content: "运行出错: " + err.Error(),
		})
		return res, err
	}

	// 持久化 assistant 消息（含工具调用记录）
	toolCalls, _ := json.Marshal(res.ToolCalls)
	asstMsg := &po.AgentMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   res.Text,
		ToolCalls: datatypes.JSON(toolCalls),
	}
	if err := uc.repo.CreateMessage(ctx, asstMsg); err != nil {
		return res, err
	}

	// 首条消息后自动命名会话
	if sess.Title == "新会话" && text != "" {
		title := text
		if len([]rune(title)) > 24 {
			title = string([]rune(title)[:24]) + "…"
		}
		_ = uc.repo.UpdateSessionTitle(ctx, sessionID, title)
	}
	return res, nil
}

func agentHistory(rows []po.AgentMessage) []*schema.AgenticMessage {
	history := make([]*schema.AgenticMessage, 0, len(rows))
	for _, row := range rows {
		switch row.Role {
		case "user":
			history = append(history, schema.UserAgenticMessage(row.Content))
		case "assistant":
			history = append(history, &schema.AgenticMessage{
				Role: schema.AgenticRoleTypeAssistant,
				ContentBlocks: []*schema.ContentBlock{
					schema.NewContentBlock(&schema.AssistantGenText{Text: row.Content}),
				},
			})
		}
	}
	return history
}

// ---- 附件 ----

var allowedMimes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"text/plain": true, "text/markdown": true, "application/json": true,
	"text/csv": true, "application/pdf": true,
}

const maxAssetSize = 20 << 20 // 20MB

// SaveAsset 校验并落盘一个会话附件。
func (uc *AgentUsecase) SaveAsset(ctx context.Context, sessionID, filename string, data []byte, userID int64) (*po.Asset, error) {
	if _, err := uc.ownSession(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("文件为空")
	}
	if len(data) > maxAssetSize {
		return nil, fmt.Errorf("文件超过 %dMB 上限", maxAssetSize>>20)
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	mimeType, _, _ = mime.ParseMediaType(mimeType)
	if !allowedMimes[mimeType] {
		return nil, fmt.Errorf("不支持的文件类型: %s", mimeType)
	}
	rel, err := uc.assets.Save(sessionID, filepath.Base(filename), data)
	if err != nil {
		return nil, err
	}
	a := &po.Asset{
		Name:      filepath.Base(filename),
		Mime:      mimeType,
		Size:      int64(len(data)),
		Path:      rel,
		SessionID: sessionID,
		CreatedBy: &userID,
	}
	if err := uc.repo.CreateAsset(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// GetAssetData 读取附件字节（校验会话归属）。
func (uc *AgentUsecase) GetAssetData(ctx context.Context, id int64, userID int64) (*po.Asset, []byte, error) {
	a, err := uc.repo.GetAsset(ctx, id)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	if _, err := uc.ownSession(ctx, a.SessionID, userID); err != nil {
		return nil, nil, err
	}
	data, err := uc.assets.Read(a.Path)
	if err != nil {
		return nil, nil, err
	}
	return a, data, nil
}

func isImageMime(m string) bool { return strings.HasPrefix(m, "image/") }

func allowAssetForSession(asset *po.Asset, sessionID string) bool {
	return asset != nil && asset.SessionID == sessionID
}

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func int64ToStr(v int64) string { return fmt.Sprintf("%d", v) }
