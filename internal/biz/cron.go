package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// CronDataRepo 定时任务持久化接口。
type CronDataRepo interface {
	List(ctx context.Context) ([]po.CronJob, error)
	ListEnabled(ctx context.Context) ([]po.CronJob, error)
	GetByID(ctx context.Context, id int64) (*po.CronJob, error)
	Create(ctx context.Context, j *po.CronJob) error
	Update(ctx context.Context, j *po.CronJob) error
	Delete(ctx context.Context, id int64) error
}

// AgentRunner 执行 agent 目标（需求1：cron 可指向 agent）。
type AgentRunner func(ctx context.Context, agentKey, prompt string) error

// CronUsecase 定时任务调度：once/interval/cron 触发已发布规则链或 Agent。
type CronUsecase struct {
	repo      CronDataRepo
	runner    ChainRunner
	agentRun  AgentRunner
	auditor   *AuditUsecase

	cron    *cron.Cron
	mu      sync.Mutex
	entries map[int64]cron.EntryID // jobID → entryID
	timers  map[int64]*time.Timer // once 任务的一次性定时器（Stop 时取消）
	started bool
}

func NewCronUsecase(repo CronDataRepo, runner ChainRunner) *CronUsecase {
	// 支持秒级 cron（可选秒字段），兼容标准 5 段表达式。
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	return &CronUsecase{
		repo:    repo,
		runner:  runner,
		cron:    cron.New(cron.WithParser(parser)),
		entries: map[int64]cron.EntryID{},
		timers:  map[int64]*time.Timer{},
	}
}

// SetAgentRunner 注入 Agent 执行器（targetType=agent）。
func (uc *CronUsecase) SetAgentRunner(r AgentRunner) { uc.agentRun = r }

// SetAuditor 注入审计。
func (uc *CronUsecase) SetAuditor(a *AuditUsecase) { uc.auditor = a }

// ---- CRUD ----

func (uc *CronUsecase) List(ctx context.Context) ([]po.CronJob, error) {
	return uc.repo.List(ctx)
}

type CronInput struct {
	Name         string          `json:"name"`
	TargetType   string          `json:"targetType" binding:"required"`
	TargetID     string          `json:"targetId" binding:"required"`
	ScheduleType string          `json:"scheduleType"`
	CronExpr     string          `json:"cronExpr"`
	IntervalSec  int             `json:"intervalSec"`
	RunAt        *time.Time      `json:"runAt"`
	Payload      json.RawMessage `json:"payload"`
}

func (uc *CronUsecase) Create(ctx context.Context, in *CronInput) (*po.CronJob, error) {
	j := &po.CronJob{Enabled: true}
	if err := applyCronInput(j, in); err != nil {
		return nil, err
	}
	if err := uc.repo.Create(ctx, j); err != nil {
		return nil, err
	}
	uc.schedule(j)
	return j, nil
}

func (uc *CronUsecase) Update(ctx context.Context, id int64, in *CronInput) error {
	j, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if err := applyCronInput(j, in); err != nil {
		return err
	}
	if err := uc.repo.Update(ctx, j); err != nil {
		return err
	}
	uc.unschedule(id)
	if j.Enabled {
		uc.schedule(j)
	}
	return nil
}

func (uc *CronUsecase) Delete(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetByID(ctx, id); err != nil {
		return ErrNotFound
	}
	uc.unschedule(id)
	return uc.repo.Delete(ctx, id)
}

func (uc *CronUsecase) Toggle(ctx context.Context, id int64) (*po.CronJob, error) {
	j, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	j.Enabled = !j.Enabled
	if err := uc.repo.Update(ctx, j); err != nil {
		return nil, err
	}
	if j.Enabled {
		uc.schedule(j)
	} else {
		uc.unschedule(id)
	}
	return j, nil
}

// applyCronInput 校验并填充字段。
func applyCronInput(j *po.CronJob, in *CronInput) error {
	if in.TargetType != "chain" && in.TargetType != "agent" {
		return errors.New("targetType 仅支持 chain / agent")
	}
	st := in.ScheduleType
	if st == "" {
		st = "cron"
	}
	switch st {
	case "cron":
		if in.CronExpr == "" {
			return errors.New("cron 模式需提供 cronExpr")
		}
	case "interval":
		if in.IntervalSec <= 0 {
			return errors.New("interval 模式需提供 intervalSec > 0")
		}
	case "once":
		if in.RunAt == nil {
			return errors.New("once 模式需提供 runAt")
		}
	default:
		return errors.New("scheduleType 仅支持 once/interval/cron")
	}
	j.Name = in.Name
	j.TargetType = in.TargetType
	j.TargetID = in.TargetID
	j.ScheduleType = st
	j.CronExpr = in.CronExpr
	j.IntervalSec = in.IntervalSec
	j.RunAt = in.RunAt
	if len(in.Payload) > 0 {
		j.Payload = datatypes.JSON(in.Payload)
	} else {
		j.Payload = []byte("{}")
	}
	return nil
}

// ---- 调度 ----

// Start 启动调度器并加载所有启用中的任务（幂等）。
func (uc *CronUsecase) Start(ctx context.Context) {
	uc.mu.Lock()
	if uc.started {
		uc.mu.Unlock()
		return
	}
	uc.started = true
	uc.cron.Start()
	uc.mu.Unlock()

	jobs, err := uc.repo.ListEnabled(ctx)
	if err != nil {
		return
	}
	for i := range jobs {
		uc.schedule(&jobs[i])
	}
}

// Stop 停止调度器（优雅关闭）：停止 cron 调度并取消所有待触发的一次性定时器。
func (uc *CronUsecase) Stop() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.started {
		<-uc.cron.Stop().Done()
		uc.started = false
		uc.entries = map[int64]cron.EntryID{}
	}
	for id, t := range uc.timers {
		t.Stop()
		delete(uc.timers, id)
	}
}

func (uc *CronUsecase) schedule(j *po.CronJob) {
	if !j.Enabled {
		return
	}
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if !uc.started {
		return
	}
	if _, ok := uc.entries[j.ID]; ok {
		return
	}
	jobID := j.ID
	run := func() { uc.execute(jobID) }

	var entryID cron.EntryID
	var err error
	switch j.ScheduleType {
	case "interval":
		entryID, err = uc.cron.AddJob(fmt.Sprintf("@every %ds", j.IntervalSec), cron.FuncJob(run))
	case "once":
		// once：用单次定时器，到点执行一次后自动移除。定时器登记以便 Stop 时取消。
		if j.RunAt != nil && j.RunAt.After(time.Now()) {
			delay := time.Until(*j.RunAt)
			uc.timers[jobID] = time.AfterFunc(delay, func() {
				uc.execute(jobID)
				uc.unschedule(jobID)
				_ = uc.repo.Delete(context.Background(), jobID) // 一次性任务执行后删除
			})
			return
		}
		return // 已过期的一次性任务不再调度
	default: // cron
		entryID, err = uc.cron.AddJob(j.CronExpr, cron.FuncJob(run))
	}
	if err == nil {
		uc.entries[j.ID] = entryID
	}
}

func (uc *CronUsecase) unschedule(id int64) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if eid, ok := uc.entries[id]; ok {
		uc.cron.Remove(eid)
		delete(uc.entries, id)
	}
	if t, ok := uc.timers[id]; ok {
		t.Stop()
		delete(uc.timers, id)
	}
}

// execute 真正触发目标（chain 执行 / agent 对话）。
func (uc *CronUsecase) execute(jobID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	j, err := uc.repo.GetByID(ctx, jobID)
	if err != nil || !j.Enabled {
		return
	}
	status := "success"
	var runErr error
	switch j.TargetType {
	case "chain":
		_, runErr = runChainWithTrigger(ctx, uc.runner, j.TargetID, string(j.Payload), "cron")
	case "agent":
		if uc.agentRun == nil {
			runErr = errors.New("agent 执行器未就绪")
		} else {
			prompt := string(j.Payload)
			runErr = uc.agentRun(ctx, j.TargetID, prompt)
		}
	}
	if runErr != nil {
		status = "failure"
	}
	CronFireTotal.WithLabelValues(j.TargetType, status).Inc()

	now := time.Now()
	j.LastRunAt = &now
	j.LastStatus = status
	_ = uc.repo.Update(ctx, j)

	if uc.auditor != nil {
		uc.auditor.Record(ctx, nil, AuditCronTrigger, j.TargetType, j.TargetID, "", map[string]any{
			"cronId": jobID, "name": j.Name, "status": status,
		})
	}
}
