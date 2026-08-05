package service

import (
	"context"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/biz/rulegokit"
)

// RuleChainProtoService 负责 RuleChain proto 请求与 usecase 的类型转换。
type RuleChainProtoService struct {
	v1.UnimplementedRuleChainServiceServer
	uc      *biz.RuleChainUsecase
	auditor *biz.AuditUsecase
}

func NewRuleChainProtoService(uc *biz.RuleChainUsecase, auditor *biz.AuditUsecase) *RuleChainProtoService {
	return &RuleChainProtoService{uc: uc, auditor: auditor}
}

func (s *RuleChainProtoService) List(ctx context.Context, req *v1.RuleChainListRequest) (*v1.RuleChainListResponse, error) {
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	page, pageSize := ruleChainPage(req.GetPage())
	list, total, err := s.uc.List(ctx, req.GetStatus(), req.GetKeyword(), page, pageSize)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.RuleChainListItem, 0, len(list))
	for _, item := range list {
		out = append(out, &v1.RuleChainListItem{Id: item.ID, Name: item.Name, Description: item.Description, Status: item.Status, Version: int32(item.Version), Source: item.Source, DebugMode: item.DebugMode, UpdatedAt: item.UpdatedAt.Format(timeLayout)})
	}
	return &v1.RuleChainListResponse{List: out, Page: &v1.PageResponse{Total: total, Page: int32(page), PageSize: int32(pageSize)}}, nil
}

func (s *RuleChainProtoService) Create(ctx context.Context, req *v1.RuleChainInput) (*v1.RuleChain, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_RULE_CHAIN", "name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	chain, err := s.uc.Create(ctx, ruleChainInput(req), currentUserID(ctx))
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoRuleChain(chain), nil
}

func (s *RuleChainProtoService) Get(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.RuleChain, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	chain, err := s.uc.Get(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoRuleChain(chain), nil
}

func (s *RuleChainProtoService) Update(ctx context.Context, req *v1.RuleChainUpdateRequest) (*v1.RuleChainOk, error) {
	if req == nil || validRuleChainID(req.Id) != nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_RULE_CHAIN", "id 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	if err := s.uc.Update(ctx, req.Id, ruleChainUpdateInput(req)); err != nil {
		return nil, protoError(err)
	}
	return &v1.RuleChainOk{Ok: true}, nil
}

func (s *RuleChainProtoService) Delete(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.RuleChainOk, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	if err := s.uc.Delete(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditChainDelete, req.Id, nil)
	return &v1.RuleChainOk{Ok: true}, nil
}

func (s *RuleChainProtoService) Validate(_ context.Context, req *v1.ValidateRuleChainRequest) (*v1.ValidateRuleChainResponse, error) {
	dsl := jsonBytes(nil)
	if req != nil {
		dsl = jsonBytes(req.Dsl)
	}
	if err := rulegokit.Validate(dsl); err != nil {
		return &v1.ValidateRuleChainResponse{Valid: false, Error: err.Error()}, nil
	}
	return &v1.ValidateRuleChainResponse{Valid: true}, nil
}

func (s *RuleChainProtoService) Publish(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.PublishRuleChainResponse, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	version, err := s.uc.Publish(ctx, req.Id, currentUserID(ctx))
	if err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditChainPublish, req.Id, map[string]any{"version": version})
	return &v1.PublishRuleChainResponse{Version: int32(version)}, nil
}

func (s *RuleChainProtoService) Offline(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.RuleChainOk, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	if err := s.uc.Offline(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditChainOffline, req.Id, nil)
	return &v1.RuleChainOk{Ok: true}, nil
}

func (s *RuleChainProtoService) ListVersions(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.RuleChainVersionList, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	list, err := s.uc.Versions(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	out := make([]*v1.RuleChainVersion, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoRuleChainVersion(&list[i]))
	}
	return &v1.RuleChainVersionList{List: out}, nil
}

func (s *RuleChainProtoService) Rollback(ctx context.Context, req *v1.RollbackRuleChainRequest) (*v1.RuleChainOk, error) {
	if req == nil || validRuleChainID(req.Id) != nil || req.Version <= 0 {
		return nil, kerrors.BadRequest("INVALID_RULE_CHAIN", "id 和 version 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	if err := s.uc.Rollback(ctx, req.Id, int(req.Version), currentUserID(ctx)); err != nil {
		return nil, protoError(err)
	}
	return &v1.RuleChainOk{Ok: true}, nil
}

func (s *RuleChainProtoService) Export(ctx context.Context, req *v1.RuleChainIdRequest) (*v1.RuleChainExport, error) {
	if err := validRuleChainID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	value, err := s.uc.Export(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return &v1.RuleChainExport{Name: value.Name, Description: value.Description, InputSchema: jsonStruct(value.InputSchema), Version: int32(value.Version), Dsl: jsonStruct(value.DSL)}, nil
}

func (s *RuleChainProtoService) Import(ctx context.Context, req *v1.RuleChainExport) (*v1.RuleChain, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_RULE_CHAIN", "name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	chain, err := s.uc.Import(ctx, &biz.ChainExport{Name: req.Name, Description: req.Description, InputSchema: jsonBytes(req.InputSchema), Version: int(req.Version), DSL: jsonBytes(req.Dsl)}, currentUserID(ctx))
	if err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditChainImport, chain.ID, map[string]any{"name": chain.Name})
	return biz.ToProtoRuleChain(chain), nil
}

func (s *RuleChainProtoService) Run(ctx context.Context, req *v1.RunRuleChainRequest) (*v1.ImmediateRunResult, error) {
	return s.run(ctx, req, false)
}

func (s *RuleChainProtoService) Debug(ctx context.Context, req *v1.RunRuleChainRequest) (*v1.ImmediateRunResult, error) {
	return s.run(ctx, req, true)
}

func (s *RuleChainProtoService) ListRuns(ctx context.Context, req *v1.RuleChainRunListRequest) (*v1.RuleChainRunListResponse, error) {
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	page, pageSize := ruleChainPage(req.GetPage())
	list, total, err := s.uc.ListRuns(ctx, req.GetChainId(), req.GetStatus(), page, pageSize)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.RuleChainRun, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoRuleChainRun(&list[i]))
	}
	return &v1.RuleChainRunListResponse{List: out, Page: &v1.PageResponse{Total: total, Page: int32(page), PageSize: int32(pageSize)}}, nil
}

func (s *RuleChainProtoService) GetRun(ctx context.Context, req *v1.RuleChainRunIdRequest) (*v1.RuleChainRun, error) {
	if req == nil || req.RunId <= 0 {
		return nil, kerrors.BadRequest("INVALID_RUN", "runId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	run, err := s.uc.GetRun(ctx, req.RunId)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoRuleChainRun(run), nil
}

func (s *RuleChainProtoService) run(ctx context.Context, req *v1.RunRuleChainRequest, debug bool) (*v1.ImmediateRunResult, error) {
	if req == nil || validRuleChainID(req.Id) != nil {
		return nil, kerrors.BadRequest("INVALID_RULE_CHAIN", "id 必填")
	}
	if s.uc == nil {
		return nil, unavailable("RULE_CHAIN_UNAVAILABLE")
	}
	input := &biz.RunInput{DataType: req.DataType, Data: req.Data, Metadata: req.Metadata}
	var result *biz.RunView
	var err error
	if debug {
		result, err = s.uc.Debug(ctx, req.Id, input)
	} else {
		result, err = s.uc.Run(ctx, req.Id, input, "manual")
	}
	if err != nil {
		return nil, protoError(err)
	}
	return immediateRunResult(result), nil
}

func (s *RuleChainProtoService) audit(ctx context.Context, action, targetID string, detail map[string]any) {
	if s.auditor != nil {
		userID := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &userID, action, "rule_chain", targetID, ip, detail)
	}
}

func ruleChainInput(req *v1.RuleChainInput) *biz.ChainInput {
	return &biz.ChainInput{Name: req.Name, Description: req.Description, InputSchema: jsonBytes(req.InputSchema), DSL: jsonBytes(req.Dsl), DebugMode: req.DebugMode, Source: req.Source}
}

func ruleChainUpdateInput(req *v1.RuleChainUpdateRequest) *biz.ChainInput {
	return &biz.ChainInput{Name: req.Name, Description: req.Description, InputSchema: jsonBytes(req.InputSchema), DSL: jsonBytes(req.Dsl), DebugMode: req.DebugMode, Source: req.Source}
}

func immediateRunResult(value *biz.RunView) *v1.ImmediateRunResult {
	return &v1.ImmediateRunResult{RunId: value.RunID, Status: value.Status, Output: value.Output, Error: value.Error, NodeTrace: biz.ProtoNodeTraces(value.Traces)}
}

func validRuleChainID(id string) error {
	if strings.TrimSpace(id) == "" {
		return kerrors.BadRequest("INVALID_RULE_CHAIN", "id 必填")
	}
	return nil
}

func ruleChainPage(req *v1.PageRequest) (int, int) {
	if req == nil {
		return 1, 20
	}
	page, pageSize := int(req.Page), int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return page, pageSize
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
