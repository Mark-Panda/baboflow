package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"baboflow/internal/biz/rulegokit"
	"baboflow/internal/data/po"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrChainPublished = errors.New("规则链已发布，需先撤销发布再删除")
	ErrChainNotLoaded = errors.New("规则链未发布，无法运行")
)

type RuleChainRepo interface {
	Create(ctx context.Context, c *po.RuleChain) error
	Update(ctx context.Context, c *po.RuleChain) error
	Get(ctx context.Context, id string) (*po.RuleChain, error)
	List(ctx context.Context, status, keyword string, page, pageSize int) ([]po.RuleChain, int64, error)
	Delete(ctx context.Context, id string) error

	CreateVersion(ctx context.Context, v *po.RuleChainVersion) error
	ListVersions(ctx context.Context, chainID string) ([]po.RuleChainVersion, error)
	GetVersion(ctx context.Context, chainID string, version int) (*po.RuleChainVersion, error)

	CreateRun(ctx context.Context, r *po.ChainRun) error
	UpdateRun(ctx context.Context, r *po.ChainRun) error
	ListRuns(ctx context.Context, chainID, status string, page, pageSize int) ([]po.ChainRun, int64, error)
	GetRun(ctx context.Context, id int64) (*po.ChainRun, error)
}

type RuleChainUsecase struct {
	repo RuleChainRepo
	eng  *rulegokit.Manager
}

func NewRuleChainUsecase(repo RuleChainRepo, eng *rulegokit.Manager) *RuleChainUsecase {
	return &RuleChainUsecase{repo: repo, eng: eng}
}

// ---- 视图 ----

type ChainView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Version     int       `json:"version"`
	Source      string    `json:"source"`
	DebugMode   bool      `json:"debugMode"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func toChainView(c *po.RuleChain) ChainView {
	return ChainView{
		ID: c.ID, Name: c.Name, Description: c.Description, Status: c.Status,
		Version: c.Version, Source: c.Source, DebugMode: c.DebugMode, UpdatedAt: c.UpdatedAt,
	}
}

// ---- CRUD ----

type ChainInput struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	DSL         json.RawMessage `json:"dsl"`
	DebugMode   bool            `json:"debugMode"`
	Source      string          `json:"source"`
}

func (uc *RuleChainUsecase) Create(ctx context.Context, in *ChainInput, userID int64) (*po.RuleChain, error) {
	id := "chain_" + uuid.NewString()[:8]
	dsl := in.DSL
	if len(dsl) == 0 {
		dsl = skeletonDSL(id, in.Name)
	} else {
		dsl = ensureChainID(dsl, id)
		if err := rulegokit.Validate(dsl); err != nil {
			return nil, err
		}
	}
	src := in.Source
	if src == "" {
		src = "manual"
	}
	c := &po.RuleChain{
		ID: id, Name: in.Name, Description: in.Description,
		DSL: datatypes.JSON(dsl), Status: "draft", Version: 0,
		DebugMode: in.DebugMode, Source: src, CreatedBy: &userID,
	}
	if err := uc.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (uc *RuleChainUsecase) Update(ctx context.Context, id string, in *ChainInput) error {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	c.Name = in.Name
	c.Description = in.Description
	c.DebugMode = in.DebugMode
	if len(in.DSL) > 0 {
		dsl := ensureChainID(in.DSL, id)
		if err := rulegokit.Validate(dsl); err != nil {
			return err
		}
		c.DSL = datatypes.JSON(dsl)
	}
	return uc.repo.Update(ctx, c)
}

func (uc *RuleChainUsecase) Get(ctx context.Context, id string) (*po.RuleChain, error) {
	return uc.repo.Get(ctx, id)
}

func (uc *RuleChainUsecase) List(ctx context.Context, status, keyword string, page, pageSize int) ([]ChainView, int64, error) {
	list, total, err := uc.repo.List(ctx, status, keyword, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ChainView, 0, len(list))
	for i := range list {
		out = append(out, toChainView(&list[i]))
	}
	return out, total, nil
}

func (uc *RuleChainUsecase) Delete(ctx context.Context, id string) error {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if c.Status == "published" {
		return ErrChainPublished
	}
	return uc.repo.Delete(ctx, id)
}

// ---- 发布 / 撤销 / 版本 ----

func (uc *RuleChainUsecase) Publish(ctx context.Context, id string, userID int64) (int, error) {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if err := rulegokit.Validate(c.DSL); err != nil {
		return 0, err
	}
	if err := uc.eng.Load(id, c.DSL); err != nil {
		return 0, err
	}
	c.Version++
	c.Status = "published"
	if err := uc.repo.Update(ctx, c); err != nil {
		return 0, err
	}
	v := &po.RuleChainVersion{
		ChainID: id, Version: c.Version, DSL: c.DSL, PublishedBy: &userID, PublishedAt: time.Now(),
	}
	if err := uc.repo.CreateVersion(ctx, v); err != nil {
		return 0, err
	}
	return c.Version, nil
}

func (uc *RuleChainUsecase) Offline(ctx context.Context, id string) error {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	uc.eng.Unload(id)
	c.Status = "draft"
	return uc.repo.Update(ctx, c)
}

func (uc *RuleChainUsecase) Versions(ctx context.Context, id string) ([]po.RuleChainVersion, error) {
	return uc.repo.ListVersions(ctx, id)
}

func (uc *RuleChainUsecase) Rollback(ctx context.Context, id string, version int, userID int64) error {
	v, err := uc.repo.GetVersion(ctx, id, version)
	if err != nil {
		return err
	}
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	c.DSL = v.DSL
	if err := uc.repo.Update(ctx, c); err != nil {
		return err
	}
	// 若当前为已发布态，重新加载并生成新版本快照
	if c.Status == "published" {
		_, err = uc.Publish(ctx, id, userID)
		return err
	}
	return nil
}

// ---- 导入 / 导出 ----

type ChainExport struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     int             `json:"version"`
	DSL         json.RawMessage `json:"dsl"`
}

func (uc *RuleChainUsecase) Export(ctx context.Context, id string) (*ChainExport, error) {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ChainExport{Name: c.Name, Description: c.Description, Version: c.Version, DSL: json.RawMessage(c.DSL)}, nil
}

func (uc *RuleChainUsecase) Import(ctx context.Context, in *ChainExport, userID int64) (*po.RuleChain, error) {
	return uc.Create(ctx, &ChainInput{
		Name: in.Name, Description: in.Description, DSL: in.DSL, Source: "manual",
	}, userID)
}

// ---- 运行 / 调试 ----

type RunInput struct {
	DataType string            `json:"dataType"`
	Data     string            `json:"data"`
	Metadata map[string]string `json:"metadata"`
}

type RunView struct {
	RunID  int64                 `json:"runId"`
	Status string                `json:"status"`
	Output string                `json:"output"`
	Error  string                `json:"error"`
	Traces []rulegokit.NodeTrace `json:"nodeTrace"`
}

// Run 运行已发布链（写 chain_run 落库）。
func (uc *RuleChainUsecase) Run(ctx context.Context, id string, in *RunInput, trigger string) (*RunView, error) {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != "published" {
		return nil, ErrChainNotLoaded
	}
	return uc.execute(ctx, id, in, trigger, false)
}

// Debug 调试用当前草稿 DSL（无需发布），写 chain_run。
func (uc *RuleChainUsecase) Debug(ctx context.Context, id string, in *RunInput) (*RunView, error) {
	if _, err := uc.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return uc.execute(ctx, id, in, "manual", true)
}

func (uc *RuleChainUsecase) execute(ctx context.Context, id string, in *RunInput, trigger string, useDraft bool) (*RunView, error) {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// 开启全节点 debugMode 以收集逐节点事件
	dsl := withDebugMode(c.DSL, true)

	inputJSON, _ := json.Marshal(in)
	run := &po.ChainRun{
		ChainID: id, Trigger: trigger, Status: "running",
		Input: datatypes.JSON(inputJSON), StartedAt: time.Now(),
	}
	if err := uc.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	var res *rulegokit.RunResult
	if useDraft {
		res, err = rulegokit.RunDSL(id, dsl, in.DataType, in.Data, in.Metadata)
	} else {
		// 已发布链以库内 DSL 调试模式运行（保证拿到 trace）
		res, err = rulegokit.RunDSL(id, dsl, in.DataType, in.Data, in.Metadata)
	}

	now := time.Now()
	run.FinishedAt = &now
	run.Output = datatypes.JSON([]byte(strOrEmptyJSON(resOutput(res))))
	traces, _ := json.Marshal(resTraces(res))
	run.NodeTrace = datatypes.JSON(traces)
	if err != nil || (res != nil && res.Err != nil) {
		run.Status = "failure"
		if err != nil {
			run.Error = err.Error()
		} else {
			run.Error = res.Err.Error()
		}
	} else {
		run.Status = "success"
	}
	_ = uc.repo.UpdateRun(ctx, run)

	// 指标：执行次数 + 耗时
	ChainExecTotal.WithLabelValues(id, trigger, run.Status).Inc()
	ChainExecDuration.WithLabelValues(id).Observe(time.Since(run.StartedAt).Seconds())

	return &RunView{
		RunID: run.ID, Status: run.Status, Output: resOutput(res),
		Error: run.Error, Traces: resTraces(res),
	}, nil
}

func resOutput(r *rulegokit.RunResult) string {
	if r == nil {
		return ""
	}
	return r.Output
}

func resTraces(r *rulegokit.RunResult) []rulegokit.NodeTrace {
	if r == nil {
		return []rulegokit.NodeTrace{}
	}
	return r.Traces
}

func strOrEmptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

// ---- 运行日志 ----

func (uc *RuleChainUsecase) ListRuns(ctx context.Context, chainID, status string, page, pageSize int) ([]po.ChainRun, int64, error) {
	return uc.repo.ListRuns(ctx, chainID, status, page, pageSize)
}

func (uc *RuleChainUsecase) GetRun(ctx context.Context, id int64) (*po.ChainRun, error) {
	return uc.repo.GetRun(ctx, id)
}

// RunPublished 实现 biz.ChainRunner：供 MCP 工具/看板任务执行已发布链。
// data 为规则链 msg.data 的 JSON 字符串；返回链输出文本。
func (uc *RuleChainUsecase) RunPublished(ctx context.Context, chainID string, data string) (string, error) {
	return uc.RunPublishedAs(ctx, chainID, data, "mcp")
}

// RunPublishedAs 同 RunPublished，但可指定触发来源（mcp/task/cron），用于 chain_run 留痕。
func (uc *RuleChainUsecase) RunPublishedAs(ctx context.Context, chainID string, data string, trigger string) (string, error) {
	c, err := uc.repo.Get(ctx, chainID)
	if err != nil {
		return "", ErrNotFound
	}
	if c.Status != "published" {
		return "", ErrChainNotLoaded
	}
	if data == "" {
		data = "{}"
	}
	if trigger == "" {
		trigger = "manual"
	}
	view, err := uc.execute(ctx, chainID, &RunInput{DataType: "JSON", Data: data}, trigger, false)
	if err != nil {
		return "", err
	}
	if view.Status != "success" {
		return "", fmt.Errorf("规则链执行失败: %s", view.Error)
	}
	return view.Output, nil
}

// RestorePublished 启动时把所有已发布链加载进引擎池。
func (uc *RuleChainUsecase) RestorePublished(ctx context.Context) error {
	list, _, err := uc.repo.List(ctx, "published", "", 1, 10000)
	if err != nil {
		return err
	}
	for i := range list {
		if err := uc.eng.Load(list[i].ID, list[i].DSL); err != nil {
			continue
		}
	}
	return nil
}

var _ = gorm.ErrRecordNotFound
