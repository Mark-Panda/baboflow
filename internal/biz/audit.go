package biz

import (
	"context"
	"encoding/json"
	"strings"

	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// AuditDataRepo 审计日志持久化接口。
type AuditDataRepo interface {
	Create(ctx context.Context, e *po.AuditLog) error
	List(ctx context.Context, action string, userID *int64, page, pageSize int) ([]po.AuditLog, int64, error)
}

// AuditUsecase 操作审计：敏感写操作留痕 + 查询。
type AuditUsecase struct {
	repo AuditDataRepo
}

// auditSlots 限制异步审计写入的并发量，避免高流量下每个请求创建一个 goroutine。
var auditSlots = make(chan struct{}, 128)

func NewAuditUsecase(repo AuditDataRepo) *AuditUsecase {
	return &AuditUsecase{repo: repo}
}

// Record 写一条审计（异步容错：写库失败不影响主流程）。
// userID 可空（登录失败等场景无会话）。
func (uc *AuditUsecase) Record(ctx context.Context, userID *int64, action, targetType, targetID, ip string, detail map[string]any) {
	if uc == nil || uc.repo == nil {
		return
	}
	var dj datatypes.JSON
	if detail != nil {
		if b, err := json.Marshal(redactDetail(detail)); err == nil {
			dj = datatypes.JSON(b)
		}
	}
	e := &po.AuditLog{
		UserID: userID, Action: action, TargetType: targetType,
		TargetID: targetID, Detail: dj, IP: ip,
	}
	// 异步写，避免阻塞请求；失败仅静默（审计不应拖垮业务）。
	select {
	case auditSlots <- struct{}{}:
	default:
		// 审计记录不可静默丢失：并发达到上限时回退为当前请求同步写入。
		_ = uc.repo.Create(context.WithoutCancel(ctx), e)
		return
	}
	go func() {
		defer func() { <-auditSlots }()
		_ = uc.repo.Create(context.WithoutCancel(ctx), e)
	}()
}

// 审计动作常量（与前端筛选项对应）。
const (
	AuditLogin          = "auth.login"
	AuditLoginFailed    = "auth.login_failed"
	AuditLoginFeishu    = "auth.login_feishu"
	AuditLogout         = "auth.logout"
	AuditChangePassword = "auth.change_password"

	AuditLLMCreate = "llm.create"
	AuditLLMUpdate = "llm.update"
	AuditLLMDelete = "llm.delete"

	AuditArcheryCreate = "archery.create"
	AuditArcheryUpdate = "archery.update"
	AuditArcheryDelete = "archery.delete"

	AuditChainPublish = "chain.publish"
	AuditChainOffline = "chain.offline"
	AuditChainDelete  = "chain.delete"
	AuditChainImport  = "chain.import"

	AuditMcpExpose = "mcp.expose"
	AuditMcpRemove = "mcp.remove_exposure"

	AuditSkillDelete = "skill.delete"
	AuditSkillUpload = "skill.upload"

	AuditTaskTrigger = "task.trigger"
	AuditCronTrigger = "cron.trigger"
)

// List 分页查询（仅 admin 路由暴露）。
func (uc *AuditUsecase) List(ctx context.Context, action string, userID *int64, page, pageSize int) ([]po.AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return uc.repo.List(ctx, action, userID, page, pageSize)
}

// redactDetail 脱敏：含敏感字样的键值一律掩码。
var sensitiveKeys = []string{"password", "secret", "apikey", "api_key", "token", "key", "authorization"}

func redactDetail(d map[string]any) map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		lk := strings.ToLower(k)
		masked := false
		for _, sk := range sensitiveKeys {
			if strings.Contains(lk, sk) {
				out[k] = "***"
				masked = true
				break
			}
		}
		if !masked {
			out[k] = v
		}
	}
	return out
}
