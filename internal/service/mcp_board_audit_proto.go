package service

import (
	"context"
	"encoding/json"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
)

// McpProtoService 将 MCP 配置和规则链暴露适配为结构化 JSON RPC。
type McpProtoService struct {
	v1.UnimplementedMcpServiceServer
	uc      *biz.McpUsecase
	builder *agentkit.McpClientBuilder
	auditor *biz.AuditUsecase
}

func NewMcpProtoService(uc *biz.McpUsecase, builder *agentkit.McpClientBuilder, auditor *biz.AuditUsecase) *McpProtoService {
	return &McpProtoService{uc: uc, builder: builder, auditor: auditor}
}

func (s *McpProtoService) ListServers(ctx context.Context, _ *v1.Empty) (*v1.McpServerList, error) {
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	list, err := s.uc.ListServers(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.McpServer, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoMcpServer(&list[i]))
	}
	return &v1.McpServerList{List: out}, nil
}

func (s *McpProtoService) CreateServer(ctx context.Context, req *v1.McpServerInput) (*v1.McpServer, error) {
	in, err := mcpServerInput(req)
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	server, err := s.uc.CreateServer(ctx, in)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoMcpServer(server), nil
}

func (s *McpProtoService) UpdateServer(ctx context.Context, req *v1.McpServerRequest) (*v1.McpOk, error) {
	if req == nil || validID(req.Id) != nil {
		return nil, kerrors.BadRequest("INVALID_MCP_SERVER", "id 必填")
	}
	in, err := mcpServerInput(&v1.McpServerInput{Name: req.Name, Transport: req.Transport, Endpoint: req.Endpoint, Command: req.Command, Args: req.Args, Env: req.Env})
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	if err := s.uc.UpdateServer(ctx, req.Id, in); err != nil {
		return nil, protoError(err)
	}
	return &v1.McpOk{Ok: true}, nil
}

func (s *McpProtoService) DeleteServer(ctx context.Context, req *v1.McpServerIdRequest) (*v1.McpOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	if err := s.uc.DeleteServer(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	return &v1.McpOk{Ok: true}, nil
}

func (s *McpProtoService) ToggleServer(ctx context.Context, req *v1.McpServerIdRequest) (*v1.McpServer, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	server, err := s.uc.ToggleServer(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoMcpServer(server), nil
}

func (s *McpProtoService) TestServer(ctx context.Context, req *v1.McpServerIdRequest) (*v1.McpTestResult, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil || s.builder == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	server, err := s.uc.GetServerForTest(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	tools, err := s.builder.ListToolNames(ctx, server)
	if err != nil {
		return &v1.McpTestResult{Ok: false, Error: err.Error(), Tools: []string{}}, nil
	}
	return &v1.McpTestResult{Ok: true, Tools: tools}, nil
}

func (s *McpProtoService) ListExposures(ctx context.Context, _ *v1.Empty) (*v1.McpExposureList, error) {
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	list, err := s.uc.ListExposures(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.McpExposure, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoMcpExposure(&list[i]))
	}
	return &v1.McpExposureList{List: out}, nil
}

func (s *McpProtoService) Expose(ctx context.Context, req *v1.ExposeMcpRequest) (*v1.ExposeMcpResponse, error) {
	if req == nil || strings.TrimSpace(req.ChainId) == "" || strings.TrimSpace(req.ToolName) == "" || req.InputSchema == nil {
		return nil, kerrors.BadRequest("INVALID_MCP_EXPOSURE", "chainId、toolName 和 inputSchema 必填")
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	exposure, err := s.uc.ExposeChain(ctx, req.ChainId, req.ToolName, req.Description, jsonBytes(req.InputSchema))
	if err != nil {
		return nil, protoError(err)
	}
	if s.auditor != nil {
		uid := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &uid, biz.AuditMcpExpose, "mcp", req.ChainId, ip, map[string]any{"toolName": exposure.ToolName})
	}
	return &v1.ExposeMcpResponse{Id: exposure.ID, ToolName: exposure.ToolName, McpEndpoint: "/mcp"}, nil
}

func (s *McpProtoService) RemoveExposure(ctx context.Context, req *v1.McpServerIdRequest) (*v1.McpOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("MCP_UNAVAILABLE")
	}
	if err := s.uc.RemoveExposure(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	if s.auditor != nil {
		uid := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &uid, biz.AuditMcpRemove, "mcp", "", ip, map[string]any{"exposureId": req.Id})
	}
	return &v1.McpOk{Ok: true}, nil
}

// BoardProtoService 将看板、列和任务适配为结构化 JSON RPC。
type BoardProtoService struct {
	v1.UnimplementedBoardServiceServer
	uc      *biz.BoardUsecase
	auditor *biz.AuditUsecase
}

func NewBoardProtoService(uc *biz.BoardUsecase, auditor *biz.AuditUsecase) *BoardProtoService {
	return &BoardProtoService{uc: uc, auditor: auditor}
}

func (s *BoardProtoService) List(ctx context.Context, _ *v1.Empty) (*v1.BoardList, error) {
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	list, err := s.uc.ListBoards(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.Board, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoBoard(&list[i]))
	}
	return &v1.BoardList{List: out}, nil
}

func (s *BoardProtoService) Create(ctx context.Context, req *v1.BoardInput) (*v1.Board, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD", "name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	board, err := s.uc.CreateBoard(ctx, &biz.BoardInput{Name: req.Name, Description: req.Description})
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoBoard(board), nil
}

func (s *BoardProtoService) Get(ctx context.Context, req *v1.BoardIdRequest) (*v1.BoardDetail, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	detail, err := s.uc.GetBoardDetail(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoBoardDetail(detail), nil
}

func (s *BoardProtoService) Update(ctx context.Context, req *v1.BoardRequest) (*v1.BoardOk, error) {
	if req == nil || validID(req.Id) != nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD", "id 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.UpdateBoard(ctx, req.Id, &biz.BoardInput{Name: req.Name, Description: req.Description}); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) Delete(ctx context.Context, req *v1.BoardIdRequest) (*v1.BoardOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.DeleteBoard(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) CreateColumn(ctx context.Context, req *v1.ColumnInput) (*v1.BoardColumn, error) {
	if req == nil || validID(req.BoardId) != nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD_COLUMN", "boardId 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	column, err := s.uc.CreateColumn(ctx, req.BoardId, &biz.ColumnInput{Name: req.Name, Sort: int(req.Sort)})
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoBoardColumn(column, nil), nil
}

func (s *BoardProtoService) UpdateColumn(ctx context.Context, req *v1.ColumnRequest) (*v1.BoardOk, error) {
	if req == nil || validID(req.Id) != nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD_COLUMN", "id 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.UpdateColumn(ctx, req.Id, &biz.ColumnInput{Name: req.Name, Sort: int(req.Sort)}); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) DeleteColumn(ctx context.Context, req *v1.BoardIdRequest) (*v1.BoardOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.DeleteColumn(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) CreateTask(ctx context.Context, req *v1.TaskInput) (*v1.BoardTask, error) {
	if req == nil || validID(req.ColumnId) != nil || strings.TrimSpace(req.Title) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD_TASK", "columnId 和 title 必填")
	}
	in, err := boardTaskInput(req.Title, req.Payload, req.AssignedChainId, req.RetryMax, req.TimeoutSec)
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	task, err := s.uc.CreateTask(ctx, req.ColumnId, in)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoBoardTask(task), nil
}

func (s *BoardProtoService) UpdateTask(ctx context.Context, req *v1.TaskRequest) (*v1.BoardOk, error) {
	if req == nil || validID(req.Id) != nil || strings.TrimSpace(req.Title) == "" {
		return nil, kerrors.BadRequest("INVALID_BOARD_TASK", "id 和 title 必填")
	}
	in, err := boardTaskInput(req.Title, req.Payload, req.AssignedChainId, req.RetryMax, req.TimeoutSec)
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.UpdateTask(ctx, req.Id, in); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) DeleteTask(ctx context.Context, req *v1.BoardIdRequest) (*v1.BoardOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.DeleteTask(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) MoveTask(ctx context.Context, req *v1.MoveTaskRequest) (*v1.BoardOk, error) {
	if req == nil || validID(req.Id) != nil || validID(req.ToColumnId) != nil {
		return nil, kerrors.BadRequest("INVALID_BOARD_TASK", "id 和 toColumnId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	if err := s.uc.MoveTask(ctx, req.Id, req.ToColumnId, req.ToSort); err != nil {
		return nil, protoError(err)
	}
	return &v1.BoardOk{Ok: true}, nil
}

func (s *BoardProtoService) TriggerTask(ctx context.Context, req *v1.BoardIdRequest) (*v1.BoardTask, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("BOARD_UNAVAILABLE")
	}
	task, err := s.uc.TriggerTask(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	if s.auditor != nil {
		uid := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &uid, biz.AuditTaskTrigger, "task", task.AssignedChainID, ip, map[string]any{"taskId": req.Id, "title": task.Title, "status": task.Status})
	}
	return biz.ToProtoBoardTask(task), nil
}

// AuditProtoService 将管理员审计查询适配为结构化 JSON RPC。
type AuditProtoService struct {
	v1.UnimplementedAuditServiceServer
	uc *biz.AuditUsecase
}

func NewAuditProtoService(uc *biz.AuditUsecase) *AuditProtoService { return &AuditProtoService{uc: uc} }

func (s *AuditProtoService) List(ctx context.Context, req *v1.AuditLogListRequest) (*v1.AuditLogListResponse, error) {
	if s.uc == nil {
		return nil, unavailable("AUDIT_UNAVAILABLE")
	}
	if req == nil {
		req = &v1.AuditLogListRequest{}
	}
	page, pageSize := auditPage(req.Page)
	var userID *int64
	if req.UserId > 0 {
		userID = &req.UserId
	}
	list, total, err := s.uc.List(ctx, req.Action, userID, page, pageSize)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.AuditLog, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoAuditLog(&list[i]))
	}
	return &v1.AuditLogListResponse{List: out, Page: &v1.PageResponse{Total: total, Page: int32(page), PageSize: int32(pageSize)}}, nil
}

func mcpServerInput(req *v1.McpServerInput) (*biz.McpServerInput, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_MCP_SERVER", "name 必填")
	}
	return &biz.McpServerInput{Name: req.Name, Transport: req.Transport, Endpoint: req.Endpoint, Command: req.Command, Args: req.Args, Env: req.Env}, nil
}

func boardTaskInput(title, payload, chainID string, retryMax, timeoutSec int32) (*biz.TaskInput, error) {
	if payload != "" && !json.Valid([]byte(payload)) {
		return nil, kerrors.BadRequest("INVALID_BOARD_TASK", "payload 必须为 JSON")
	}
	return &biz.TaskInput{Title: title, Payload: json.RawMessage(payload), AssignedChainID: chainID, RetryMax: int(retryMax), TimeoutSec: int(timeoutSec)}, nil
}

func auditPage(in *v1.PageRequest) (int, int) {
	page, pageSize := 1, 20
	if in == nil {
		return page, pageSize
	}
	if in.Page > 0 {
		page = int(in.Page)
	}
	if in.PageSize > 0 && in.PageSize <= 200 {
		pageSize = int(in.PageSize)
	}
	return page, pageSize
}
