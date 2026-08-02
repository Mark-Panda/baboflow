package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// BoardDataRepo 看板持久化接口。
type BoardDataRepo interface {
	// board
	ListBoards(ctx context.Context) ([]po.Board, error)
	GetBoard(ctx context.Context, id int64) (*po.Board, error)
	CreateBoard(ctx context.Context, b *po.Board) error
	UpdateBoard(ctx context.Context, b *po.Board) error
	DeleteBoard(ctx context.Context, id int64) error

	// column
	ListColumns(ctx context.Context, boardID int64) ([]po.BoardColumn, error)
	GetColumn(ctx context.Context, id int64) (*po.BoardColumn, error)
	CreateColumn(ctx context.Context, c *po.BoardColumn) error
	UpdateColumn(ctx context.Context, c *po.BoardColumn) error
	DeleteColumn(ctx context.Context, id int64) error

	// task
	ListTasksByBoard(ctx context.Context, boardID int64) ([]po.Task, error)
	GetTask(ctx context.Context, id int64) (*po.Task, error)
	CreateTask(ctx context.Context, t *po.Task) error
	UpdateTask(ctx context.Context, t *po.Task) error
	DeleteTask(ctx context.Context, id int64) error
}

// BoardUsecase 看板 + 任务触发规则链。
type BoardUsecase struct {
	repo   BoardDataRepo
	runner ChainRunner

	stopCh chan struct{}
}

func NewBoardUsecase(repo BoardDataRepo, runner ChainRunner) *BoardUsecase {
	return &BoardUsecase{repo: repo, runner: runner, stopCh: make(chan struct{})}
}

// Stop 通知所有进行中的重试循环尽快退出（优雅停机）。在途的 runOnce 由自身超时收尾。
func (uc *BoardUsecase) Stop() {
	select {
	case <-uc.stopCh:
	default:
		close(uc.stopCh)
	}
}

// ---- 视图 ----

type ColumnView struct {
	po.BoardColumn
	Tasks []po.Task `json:"tasks"`
}

type BoardDetail struct {
	po.Board
	Columns []ColumnView `json:"columns"`
}

// ---- Board ----

func (uc *BoardUsecase) ListBoards(ctx context.Context) ([]po.Board, error) {
	return uc.repo.ListBoards(ctx)
}

type BoardInput struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

func (uc *BoardUsecase) CreateBoard(ctx context.Context, in *BoardInput) (*po.Board, error) {
	b := &po.Board{Name: in.Name, Description: in.Description}
	if err := uc.repo.CreateBoard(ctx, b); err != nil {
		return nil, err
	}
	// 默认三列
	defaults := []string{"待办", "进行中", "已完成"}
	for i, name := range defaults {
		_ = uc.repo.CreateColumn(ctx, &po.BoardColumn{BoardID: b.ID, Name: name, Sort: i})
	}
	return b, nil
}

func (uc *BoardUsecase) UpdateBoard(ctx context.Context, id int64, in *BoardInput) error {
	b, err := uc.repo.GetBoard(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	b.Name = in.Name
	b.Description = in.Description
	return uc.repo.UpdateBoard(ctx, b)
}

func (uc *BoardUsecase) DeleteBoard(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetBoard(ctx, id); err != nil {
		return ErrNotFound
	}
	return uc.repo.DeleteBoard(ctx, id)
}

// GetBoardDetail 一次性返回看板 + 列 + 任务。
func (uc *BoardUsecase) GetBoardDetail(ctx context.Context, id int64) (*BoardDetail, error) {
	b, err := uc.repo.GetBoard(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	cols, err := uc.repo.ListColumns(ctx, id)
	if err != nil {
		return nil, err
	}
	tasks, err := uc.repo.ListTasksByBoard(ctx, id)
	if err != nil {
		return nil, err
	}
	byCol := map[int64][]po.Task{}
	for _, t := range tasks {
		byCol[t.ColumnID] = append(byCol[t.ColumnID], t)
	}
	detail := &BoardDetail{Board: *b, Columns: make([]ColumnView, 0, len(cols))}
	for _, c := range cols {
		ts := byCol[c.ID]
		if ts == nil {
			ts = []po.Task{}
		}
		detail.Columns = append(detail.Columns, ColumnView{BoardColumn: c, Tasks: ts})
	}
	return detail, nil
}

// ---- Column ----

type ColumnInput struct {
	Name string `json:"name" binding:"required"`
	Sort int    `json:"sort"`
}

func (uc *BoardUsecase) CreateColumn(ctx context.Context, boardID int64, in *ColumnInput) (*po.BoardColumn, error) {
	c := &po.BoardColumn{BoardID: boardID, Name: in.Name, Sort: in.Sort}
	if err := uc.repo.CreateColumn(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (uc *BoardUsecase) UpdateColumn(ctx context.Context, id int64, in *ColumnInput) error {
	c, err := uc.repo.GetColumn(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	c.Name = in.Name
	c.Sort = in.Sort
	return uc.repo.UpdateColumn(ctx, c)
}

func (uc *BoardUsecase) DeleteColumn(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetColumn(ctx, id); err != nil {
		return ErrNotFound
	}
	return uc.repo.DeleteColumn(ctx, id)
}

// ---- Task ----

type TaskInput struct {
	Title           string          `json:"title" binding:"required"`
	Payload         json.RawMessage `json:"payload"`
	AssignedChainID string          `json:"assignedChainId"`
	RetryMax        int             `json:"retryMax"`
	TimeoutSec      int             `json:"timeoutSec"`
}

func (uc *BoardUsecase) CreateTask(ctx context.Context, columnID int64, in *TaskInput) (*po.Task, error) {
	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	timeout := in.TimeoutSec
	if timeout <= 0 {
		timeout = 300
	}
	t := &po.Task{
		ColumnID: columnID, Title: in.Title, Payload: datatypes.JSON(payload),
		AssignedChainID: in.AssignedChainID, RetryMax: in.RetryMax,
		TimeoutSec: timeout, Status: "pending",
	}
	if err := uc.repo.CreateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (uc *BoardUsecase) UpdateTask(ctx context.Context, id int64, in *TaskInput) error {
	t, err := uc.repo.GetTask(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	t.Title = in.Title
	t.AssignedChainID = in.AssignedChainID
	if len(in.Payload) > 0 {
		t.Payload = datatypes.JSON(in.Payload)
	}
	if in.RetryMax > 0 {
		t.RetryMax = in.RetryMax
	}
	if in.TimeoutSec > 0 {
		t.TimeoutSec = in.TimeoutSec
	}
	return uc.repo.UpdateTask(ctx, t)
}

func (uc *BoardUsecase) DeleteTask(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetTask(ctx, id); err != nil {
		return ErrNotFound
	}
	return uc.repo.DeleteTask(ctx, id)
}

// MoveTask 拖拽流转：改列 + 排序。
func (uc *BoardUsecase) MoveTask(ctx context.Context, id, toColumnID, toSort int64) error {
	t, err := uc.repo.GetTask(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if _, err := uc.repo.GetColumn(ctx, toColumnID); err != nil {
		return errors.New("目标列不存在")
	}
	t.ColumnID = toColumnID
	t.Sort = int(toSort)
	return uc.repo.UpdateTask(ctx, t)
}

// TriggerTask 用分配的已发布链执行任务（带超时 + 失败指数退避重试），结果回写。
func (uc *BoardUsecase) TriggerTask(ctx context.Context, id int64) (*po.Task, error) {
	t, err := uc.repo.GetTask(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if t.AssignedChainID == "" {
		return nil, errors.New("任务未分配规则链")
	}
	t.Status = "running"
	t.RetryCount = 0
	_ = uc.repo.UpdateTask(ctx, t)

	timeout := t.TimeoutSec
	if timeout <= 0 {
		timeout = 300
	}

	// 首次同步执行（让调用方拿到即时结果）；失败后按 retryMax 异步指数退避重试。
	out, runErr := uc.runOnce(ctx, t, timeout)
	if runErr == nil {
		t.Status = "success"
		t.Result = datatypes.JSON([]byte(mustJSONObj(map[string]any{"output": out})))
		if err := uc.repo.UpdateTask(ctx, t); err != nil {
			return t, err
		}
		return t, nil
	}

	// 首次失败
	t.Result = datatypes.JSON([]byte(mustJSONObj(map[string]any{"error": runErr.Error()})))
	if t.RetryMax <= 0 {
		t.Status = "failure"
		if err := uc.repo.UpdateTask(ctx, t); err != nil {
			return t, err
		}
		return t, nil
	}

	// 标记为运行中（待重试），异步退避重试，最终回写 success/failure。
	t.Status = "running"
	if err := uc.repo.UpdateTask(ctx, t); err != nil {
		return t, err
	}
	go uc.retryLoop(t.ID, timeout)
	return t, nil
}

// runOnce 带超时执行一次。
func (uc *BoardUsecase) runOnce(ctx context.Context, t *po.Task, timeout int) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	return runChainWithTrigger(runCtx, uc.runner, t.AssignedChainID, string(t.Payload), "task")
}

// retryLoop 异步指数退避重试（5s/15s/45s…），直到成功或达上限。
func (uc *BoardUsecase) retryLoop(taskID int64, timeout int) {
	// 兜底：单次重试 panic 不应拖垮整个进程；记录后按 failure 收尾。
	defer func() {
		if r := recover(); r != nil {
			bg := context.Background()
			if t, err := uc.repo.GetTask(bg, taskID); err == nil {
				t.Status = "failure"
				t.Result = datatypes.JSON([]byte(mustJSONObj(map[string]any{"error": "panic: " + fmt.Sprint(r)})))
				_ = uc.repo.UpdateTask(bg, t)
			}
		}
	}()
	bg := context.Background()
	t, err := uc.repo.GetTask(bg, taskID)
	if err != nil {
		return
	}
	for t.RetryCount < t.RetryMax {
		t.RetryCount++
		_ = uc.repo.UpdateTask(bg, t)
		// 指数退避：5s, 15s, 45s, ...（可被打断以便优雅停机）
		backoff := retryBackoff(t.RetryCount)
		select {
		case <-uc.stopCh:
			return // 停机：保持 running 状态，待下次启动由人工/对账处理
		case <-time.After(backoff):
		}

		out, runErr := uc.runOnce(bg, t, timeout)
		// 重新读取（避免覆盖期间的状态变化）
		fresh, err := uc.repo.GetTask(bg, taskID)
		if err != nil {
			return
		}
		t = fresh
		if runErr == nil {
			t.Status = "success"
			t.Result = datatypes.JSON([]byte(mustJSONObj(map[string]any{"output": out})))
			_ = uc.repo.UpdateTask(bg, t)
			return
		}
		t.Result = datatypes.JSON([]byte(mustJSONObj(map[string]any{"error": runErr.Error()})))
		_ = uc.repo.UpdateTask(bg, t)
	}
	// 超过上限
	t.Status = "failure"
	_ = uc.repo.UpdateTask(bg, t)
}

// retryBackoff 指数退避：第 n 次重试等待 5 * 3^(n-1) 秒（5/15/45/...），封顶 5 分钟。
func retryBackoff(attempt int) time.Duration {
	d := 5 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 3
		if d > 5*time.Minute {
			return 5 * time.Minute
		}
	}
	return d
}

func mustJSONObj(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
