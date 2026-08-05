package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
)

type LLMProtoService struct {
	v1.UnimplementedLLMServiceServer
	uc      *biz.LLMUsecase
	auditor *biz.AuditUsecase
}

func NewLLMProtoService(uc *biz.LLMUsecase, auditor *biz.AuditUsecase) *LLMProtoService {
	return &LLMProtoService{uc: uc, auditor: auditor}
}

func (s *LLMProtoService) ListProviders(ctx context.Context, _ *v1.Empty) (*v1.ProviderList, error) {
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	list, err := s.uc.ListProviders(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.Provider, 0, len(list))
	for _, item := range list {
		out = append(out, &v1.Provider{Id: item.ID, Name: item.Name, Provider: item.Provider, BaseUrl: item.BaseURL, ApiKeyMasked: item.APIKeyMasked, Extra: jsonStruct(item.Extra), Remark: item.Remark, ModelCount: int32(item.ModelCount)})
	}
	return &v1.ProviderList{List: out}, nil
}

func (s *LLMProtoService) CreateProvider(ctx context.Context, req *v1.ProviderInput) (*v1.CreateIdResponse, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseUrl) == "" {
		return nil, kerrors.BadRequest("INVALID_PROVIDER", "name 和 baseUrl 必填")
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	provider, err := s.uc.CreateProvider(ctx, &biz.ProviderInput{Name: req.Name, Provider: req.Provider, BaseURL: req.BaseUrl, APIKey: req.ApiKey, Extra: jsonBytes(req.Extra), Remark: req.Remark})
	if err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMCreate, provider.ID, map[string]any{"name": req.Name, "type": "provider"})
	return &v1.CreateIdResponse{Id: provider.ID}, nil
}

func (s *LLMProtoService) UpdateProvider(ctx context.Context, req *v1.UpdateProviderRequest) (*v1.OkResponse, error) {
	if req == nil || validID(req.Id) != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseUrl) == "" {
		return nil, kerrors.BadRequest("INVALID_PROVIDER", "id、name 和 baseUrl 必填")
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	err := s.uc.UpdateProvider(ctx, req.Id, &biz.ProviderInput{Name: req.Name, Provider: req.Provider, BaseURL: req.BaseUrl, APIKey: req.ApiKey, Extra: jsonBytes(req.Extra), Remark: req.Remark})
	if err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMUpdate, req.Id, map[string]any{"name": req.Name, "type": "provider"})
	return &v1.OkResponse{Ok: true}, nil
}

func (s *LLMProtoService) DeleteProvider(ctx context.Context, req *v1.IdRequest) (*v1.OkResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	if err := s.uc.DeleteProvider(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMDelete, req.Id, map[string]any{"type": "provider"})
	return &v1.OkResponse{Ok: true}, nil
}

func (s *LLMProtoService) TestProvider(ctx context.Context, req *v1.IdRequest) (*v1.TestResult, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	result, err := s.uc.TestProvider(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return &v1.TestResult{Ok: result.OK, LatencyMs: result.LatencyMs, Message: result.Message}, nil
}

func (s *LLMProtoService) ListRemoteModels(ctx context.Context, req *v1.IdRequest) (*v1.StringList, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	list, err := s.uc.ProviderModels(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return &v1.StringList{Models: list}, nil
}

func (s *LLMProtoService) ListModels(ctx context.Context, req *v1.ProviderModelsRequest) (*v1.ModelList, error) {
	if err := validID(req.GetProviderId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	list, err := s.uc.ListModels(ctx, req.ProviderId)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.LLMModel, 0, len(list))
	for _, item := range list {
		out = append(out, biz.ToProtoLLMModel(item))
	}
	return &v1.ModelList{List: out}, nil
}

func (s *LLMProtoService) CreateModels(ctx context.Context, req *v1.CreateModelsRequest) (*v1.OkResponse, error) {
	if req == nil || validID(req.ProviderId) != nil || len(req.Models) == 0 {
		return nil, kerrors.BadRequest("INVALID_MODELS", "providerId 和 models 必填")
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	inputs := make([]biz.ModelInput, 0, len(req.Models))
	for _, item := range req.Models {
		if item == nil || strings.TrimSpace(item.Model) == "" {
			return nil, kerrors.BadRequest("INVALID_MODEL", "model 必填")
		}
		inputs = append(inputs, modelInput(item))
	}
	if err := s.uc.CreateModels(ctx, req.ProviderId, inputs); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMCreate, req.ProviderId, map[string]any{"type": "model", "count": len(inputs)})
	return &v1.OkResponse{Ok: true}, nil
}

func (s *LLMProtoService) UpdateModel(ctx context.Context, req *v1.UpdateModelRequest) (*v1.OkResponse, error) {
	if req == nil || validID(req.ModelId) != nil {
		return nil, kerrors.BadRequest("INVALID_MODEL", "modelId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	input := modelInput(&v1.ModelInput{Model: req.Model, Alias: req.Alias, Temperature: req.Temperature, MaxTokens: req.MaxTokens, IsDefault: req.IsDefault, Capability: req.Capability, Enabled: req.Enabled})
	if err := s.uc.UpdateModel(ctx, req.ModelId, &input); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMUpdate, req.ModelId, map[string]any{"type": "model", "model": req.Model})
	return &v1.OkResponse{Ok: true}, nil
}

func (s *LLMProtoService) DeleteModel(ctx context.Context, req *v1.ModelIdRequest) (*v1.OkResponse, error) {
	if err := validID(req.GetModelId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	if err := s.uc.DeleteModel(ctx, req.ModelId); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditLLMDelete, req.ModelId, map[string]any{"type": "model"})
	return &v1.OkResponse{Ok: true}, nil
}
func (s *LLMProtoService) SetDefaultModel(ctx context.Context, req *v1.ModelIdRequest) (*v1.OkResponse, error) {
	if err := validID(req.GetModelId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	if err := s.uc.SetDefaultModel(ctx, req.ModelId); err != nil {
		return nil, protoError(err)
	}
	return &v1.OkResponse{Ok: true}, nil
}
func (s *LLMProtoService) TestModel(ctx context.Context, req *v1.ModelIdRequest) (*v1.TestResult, error) {
	if err := validID(req.GetModelId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("LLM_UNAVAILABLE")
	}
	result, err := s.uc.TestModel(ctx, req.ModelId)
	if err != nil {
		return nil, protoError(err)
	}
	return &v1.TestResult{Ok: result.OK, LatencyMs: result.LatencyMs, Message: result.Message}, nil
}

func (s *LLMProtoService) audit(ctx context.Context, action string, targetID int64, detail map[string]any) {
	if s.auditor == nil {
		return
	}
	userID := currentUserID(ctx)
	ip, _ := ClientMetadataFromContext(ctx)
	s.auditor.Record(ctx, &userID, action, "llm", strconv.FormatInt(targetID, 10), ip, detail)
}

type ComponentProtoService struct {
	v1.UnimplementedComponentServiceServer
	repo biz.ComponentRepo
	sync *biz.ComponentSync
}

func NewComponentProtoService(repo biz.ComponentRepo, sync *biz.ComponentSync) *ComponentProtoService {
	return &ComponentProtoService{repo: repo, sync: sync}
}
func (s *ComponentProtoService) List(ctx context.Context, req *v1.ComponentListRequest) (*v1.ComponentListResponse, error) {
	if s.repo == nil {
		return nil, unavailable("COMPONENT_UNAVAILABLE")
	}
	if req == nil {
		req = &v1.ComponentListRequest{}
	}
	list, err := s.repo.SearchKeyword(ctx, req.Category, req.Keyword)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.ComponentMeta, 0, len(list))
	for _, item := range list {
		out = append(out, &v1.ComponentMeta{Type: item.Type, Name: item.Name, Category: item.Category, Description: item.Description, ConfigSchema: jsonStruct(item.ConfigSchema), Example: jsonStruct(item.Example)})
	}
	return &v1.ComponentListResponse{List: out}, nil
}
func (s *ComponentProtoService) GetSyncStatus(context.Context, *v1.Empty) (*v1.ComponentSyncResult, error) {
	if s.sync == nil {
		return nil, unavailable("COMPONENT_UNAVAILABLE")
	}
	return componentSyncResult(s.sync.Last()), nil
}
func (s *ComponentProtoService) Sync(ctx context.Context, _ *v1.Empty) (*v1.ComponentSyncResult, error) {
	if s.sync == nil {
		return nil, unavailable("COMPONENT_UNAVAILABLE")
	}
	result, err := s.sync.Run(ctx)
	if err != nil {
		return nil, internal(err)
	}
	return componentSyncResult(result), nil
}

type CronProtoService struct {
	v1.UnimplementedCronServiceServer
	uc *biz.CronUsecase
}

func NewCronProtoService(uc *biz.CronUsecase) *CronProtoService { return &CronProtoService{uc: uc} }
func (s *CronProtoService) List(ctx context.Context, _ *v1.Empty) (*v1.CronJobList, error) {
	if s.uc == nil {
		return nil, unavailable("CRON_UNAVAILABLE")
	}
	list, err := s.uc.List(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.CronJob, 0, len(list))
	for _, item := range list {
		out = append(out, biz.ToProtoCronJob(&item))
	}
	return &v1.CronJobList{List: out}, nil
}
func (s *CronProtoService) Create(ctx context.Context, req *v1.CronInput) (*v1.CronJob, error) {
	in, err := cronInput(req)
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("CRON_UNAVAILABLE")
	}
	job, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoCronJob(job), nil
}
func (s *CronProtoService) Update(ctx context.Context, req *v1.CronJobRequest) (*v1.CronOk, error) {
	if req == nil || validID(req.Id) != nil {
		return nil, kerrors.BadRequest("INVALID_CRON", "id 必填")
	}
	in, err := cronInput(&v1.CronInput{Name: req.Name, TargetType: req.TargetType, TargetId: req.TargetId, ScheduleType: req.ScheduleType, CronExpr: req.CronExpr, IntervalSec: req.IntervalSec, RunAt: req.RunAt, Payload: req.Payload})
	if err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("CRON_UNAVAILABLE")
	}
	if err := s.uc.Update(ctx, req.Id, in); err != nil {
		return nil, protoError(err)
	}
	return &v1.CronOk{Ok: true}, nil
}
func (s *CronProtoService) Delete(ctx context.Context, req *v1.CronJobIdRequest) (*v1.CronOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("CRON_UNAVAILABLE")
	}
	if err := s.uc.Delete(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	return &v1.CronOk{Ok: true}, nil
}
func (s *CronProtoService) Toggle(ctx context.Context, req *v1.CronJobIdRequest) (*v1.CronJob, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("CRON_UNAVAILABLE")
	}
	job, err := s.uc.Toggle(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoCronJob(job), nil
}

func modelInput(in *v1.ModelInput) biz.ModelInput {
	var enabled *bool
	if in.Enabled != nil {
		enabled = &in.Enabled.Value
	}
	return biz.ModelInput{Model: in.Model, Alias: in.Alias, Temperature: in.Temperature, MaxTokens: int(in.MaxTokens), IsDefault: in.IsDefault != nil && in.IsDefault.Value, Capability: jsonBytes(in.Capability), Enabled: enabled}
}
func jsonBytes(in *structpb.Struct) []byte {
	if in == nil {
		return nil
	}
	b, _ := json.Marshal(in.AsMap())
	return b
}
func jsonStruct(in []byte) *structpb.Struct {
	var value map[string]any
	if json.Unmarshal(in, &value) != nil {
		return nil
	}
	out, _ := structpb.NewStruct(value)
	return out
}
func componentSyncResult(in biz.SyncResult) *v1.ComponentSyncResult {
	return &v1.ComponentSyncResult{Added: int32(in.Added), Updated: int32(in.Updated), Removed: int32(in.Removed), Skipped: int32(in.Skipped), LastRunAt: in.LastRunAt.Format(time.RFC3339)}
}
func cronInput(in *v1.CronInput) (*biz.CronInput, error) {
	if in == nil || strings.TrimSpace(in.TargetType) == "" || strings.TrimSpace(in.TargetId) == "" {
		return nil, kerrors.BadRequest("INVALID_CRON", "targetType 和 targetId 必填")
	}
	out := &biz.CronInput{Name: in.Name, TargetType: in.TargetType, TargetID: in.TargetId, ScheduleType: in.ScheduleType, CronExpr: in.CronExpr, IntervalSec: int(in.IntervalSec), Payload: jsonBytes(in.Payload)}
	if in.RunAt != "" {
		value, err := time.Parse(time.RFC3339, in.RunAt)
		if err != nil {
			return nil, kerrors.BadRequest("INVALID_CRON", "runAt 必须为 RFC3339 时间")
		}
		out.RunAt = &value
	}
	return out, nil
}
