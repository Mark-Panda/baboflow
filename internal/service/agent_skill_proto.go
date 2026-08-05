package service

import (
	"context"
	"errors"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/wrapperspb"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
)

// AgentProtoService 将 Agent 配置与会话用例适配为结构化 JSON RPC。
type AgentProtoService struct {
	v1.UnimplementedAgentServiceServer
	uc *biz.AgentUsecase
}

func NewAgentProtoService(uc *biz.AgentUsecase) *AgentProtoService { return &AgentProtoService{uc: uc} }

func (s *AgentProtoService) List(ctx context.Context, req *v1.AgentListRequest) (*v1.AgentListResponse, error) {
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	if req == nil {
		req = &v1.AgentListRequest{}
	}
	list, err := s.uc.List(ctx, req.Keyword)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.Agent, 0, len(list))
	for i := range list {
		out = append(out, agentProto(&list[i]))
	}
	return &v1.AgentListResponse{List: out}, nil
}

func (s *AgentProtoService) Create(ctx context.Context, req *v1.AgentInput) (*v1.Agent, error) {
	if req == nil || strings.TrimSpace(req.Key) == "" || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT", "key 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	agent, err := s.uc.Create(ctx, req.Key, agentInput(req))
	if err != nil {
		return nil, protoError(err)
	}
	return agentProto(agent), nil
}

func (s *AgentProtoService) Get(ctx context.Context, req *v1.AgentKeyRequest) (*v1.Agent, error) {
	if req == nil || strings.TrimSpace(req.Key) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT_KEY", "key 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	agent, err := s.uc.Get(ctx, req.Key)
	if err != nil {
		return nil, protoError(err)
	}
	return agentProto(agent), nil
}

func (s *AgentProtoService) Update(ctx context.Context, req *v1.AgentUpdateRequest) (*v1.AgentOk, error) {
	if req == nil || strings.TrimSpace(req.Key) == "" || strings.TrimSpace(req.Name) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT", "key 和 name 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	if err := s.uc.Update(ctx, req.Key, agentInput(&v1.AgentInput{
		Name: req.Name, Instruction: req.Instruction, LlmModelId: req.LlmModelId, SkillIds: req.SkillIds,
		McpIds: req.McpIds, BuiltinTools: req.BuiltinTools, SubAgentIds: req.SubAgentIds, Enabled: req.Enabled,
	})); err != nil {
		return nil, protoError(err)
	}
	return &v1.AgentOk{Ok: true}, nil
}

func (s *AgentProtoService) Delete(ctx context.Context, req *v1.AgentKeyRequest) (*v1.AgentOk, error) {
	if req == nil || strings.TrimSpace(req.Key) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT_KEY", "key 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	if err := s.uc.Delete(ctx, req.Key); err != nil {
		return nil, protoError(err)
	}
	return &v1.AgentOk{Ok: true}, nil
}

func (s *AgentProtoService) ListSessions(ctx context.Context, req *v1.AgentSessionListRequest) (*v1.AgentSessionList, error) {
	if req == nil || strings.TrimSpace(req.AgentKey) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT_KEY", "agentKey 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	list, err := s.uc.ListSessions(ctx, req.AgentKey, currentUserID(ctx))
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.AgentSession, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoAgentSession(&list[i]))
	}
	return &v1.AgentSessionList{List: out}, nil
}

func (s *AgentProtoService) CreateSession(ctx context.Context, req *v1.CreateAgentSessionRequest) (*v1.AgentSession, error) {
	if req == nil || strings.TrimSpace(req.AgentKey) == "" {
		return nil, kerrors.BadRequest("INVALID_AGENT_KEY", "agentKey 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	chainID := ""
	if req.ChainId != nil {
		chainID = req.ChainId.Value
	}
	session, err := s.uc.CreateChainSession(ctx, req.AgentKey, req.Title, chainID, currentUserID(ctx))
	if err != nil {
		return nil, protoError(err)
	}
	return biz.ToProtoAgentSession(session), nil
}

func (s *AgentProtoService) DeleteSession(ctx context.Context, req *v1.AgentSessionRequest) (*v1.AgentOk, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, kerrors.BadRequest("INVALID_SESSION_ID", "sessionId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	if err := s.uc.DeleteSession(ctx, req.SessionId, currentUserID(ctx)); err != nil {
		return nil, protoError(err)
	}
	return &v1.AgentOk{Ok: true}, nil
}

func (s *AgentProtoService) ListMessages(ctx context.Context, req *v1.AgentSessionRequest) (*v1.AgentMessageList, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, kerrors.BadRequest("INVALID_SESSION_ID", "sessionId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("AGENT_UNAVAILABLE")
	}
	list, err := s.uc.ListMessages(ctx, req.SessionId, currentUserID(ctx))
	if err != nil {
		return nil, protoError(err)
	}
	out := make([]*v1.AgentMessage, 0, len(list))
	for i := range list {
		out = append(out, biz.ToProtoAgentMessage(&list[i]))
	}
	return &v1.AgentMessageList{List: out}, nil
}

// SkillProtoService 将文本 SKILL 与包内文件读取适配为结构化 JSON RPC。
type SkillProtoService struct {
	v1.UnimplementedSkillServiceServer
	uc      *biz.SkillUsecase
	auditor *biz.AuditUsecase
}

func NewSkillProtoService(uc *biz.SkillUsecase, auditor *biz.AuditUsecase) *SkillProtoService {
	return &SkillProtoService{uc: uc, auditor: auditor}
}

func (s *SkillProtoService) List(ctx context.Context, req *v1.SkillListRequest) (*v1.SkillListResponse, error) {
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	if req == nil {
		req = &v1.SkillListRequest{}
	}
	list, err := s.uc.List(ctx, req.Source, req.Keyword)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.Skill, 0, len(list))
	for i := range list {
		out = append(out, skillProto(&list[i]))
	}
	return &v1.SkillListResponse{List: out}, nil
}

func (s *SkillProtoService) Upload(ctx context.Context, req *v1.UploadSkillRequest) (*v1.Skill, error) {
	if req == nil || strings.TrimSpace(req.Content) == "" {
		return nil, kerrors.BadRequest("INVALID_SKILL", "content 必填")
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	skill, err := s.uc.Upload(ctx, req.Content, req.Source)
	if err != nil {
		return nil, skillProtoError(err)
	}
	return skillProto(skill), nil
}

func (s *SkillProtoService) Get(ctx context.Context, req *v1.SkillIdRequest) (*v1.Skill, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	skill, err := s.uc.Get(ctx, req.Id)
	if err != nil {
		return nil, skillProtoError(err)
	}
	return skillProto(skill), nil
}

func (s *SkillProtoService) Delete(ctx context.Context, req *v1.SkillIdRequest) (*v1.SkillOk, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	if err := s.uc.Delete(ctx, req.Id); err != nil {
		return nil, skillProtoError(err)
	}
	if s.auditor != nil {
		uid := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &uid, biz.AuditSkillDelete, "skill", "", ip, map[string]any{"skillId": req.Id})
	}
	return &v1.SkillOk{Ok: true}, nil
}

func (s *SkillProtoService) ListFiles(ctx context.Context, req *v1.SkillIdRequest) (*v1.SkillFileList, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	list, err := s.uc.ListFiles(ctx, req.Id)
	if err != nil {
		return nil, skillProtoError(err)
	}
	out := make([]*v1.SkillFile, 0, len(list))
	for i := range list {
		out = append(out, &v1.SkillFile{Path: list[i].Path, Size: list[i].Size, IsDir: list[i].IsDir})
	}
	return &v1.SkillFileList{List: out}, nil
}

func (s *SkillProtoService) ReadFile(ctx context.Context, req *v1.ReadSkillFileRequest) (*v1.ReadSkillFileResponse, error) {
	if req == nil || req.Id <= 0 || strings.TrimSpace(req.Path) == "" {
		return nil, kerrors.BadRequest("INVALID_SKILL_FILE", "id 和 path 必填")
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	content, err := s.uc.ReadFile(ctx, req.Id, req.Path)
	if err != nil {
		return nil, skillProtoError(err)
	}
	return &v1.ReadSkillFileResponse{Path: req.Path, Content: content}, nil
}

func (s *SkillProtoService) Generate(ctx context.Context, req *v1.GenerateSkillRequest) (*v1.Skill, error) {
	if req == nil || strings.TrimSpace(req.ChainId) == "" {
		return nil, kerrors.BadRequest("INVALID_CHAIN_ID", "chainId 必填")
	}
	if s.uc == nil {
		return nil, unavailable("SKILL_UNAVAILABLE")
	}
	skill, err := s.uc.GenerateFromChain(ctx, req.ChainId)
	if err != nil {
		return nil, skillProtoError(err)
	}
	return skillProto(skill), nil
}

func agentInput(req *v1.AgentInput) *biz.AgentInput {
	var modelID *int64
	if req.LlmModelId != nil {
		modelID = &req.LlmModelId.Value
	}
	var enabled *bool
	if req.Enabled != nil {
		enabled = &req.Enabled.Value
	}
	return &biz.AgentInput{Name: req.Name, Instruction: req.Instruction, LLMModelID: modelID, SkillIDs: req.SkillIds, McpIDs: req.McpIds, BuiltinTools: req.BuiltinTools, SubAgentIDs: req.SubAgentIds, Enabled: enabled}
}

func agentProto(in *biz.AgentView) *v1.Agent {
	out := &v1.Agent{Id: in.ID, Key: in.Key, Name: in.Name, Instruction: in.Instruction, SkillIds: in.SkillIDs, McpIds: in.McpIDs, BuiltinTools: in.BuiltinTools, SubAgentIds: in.SubAgentIDs, IsBuiltin: in.IsBuiltin, Enabled: wrapperspb.Bool(in.Enabled), UpdatedAt: in.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")}
	if in.LLMModelID != nil {
		out.LlmModelId = wrapperspb.Int64(*in.LLMModelID)
	}
	return out
}

func skillProto(in *biz.SkillView) *v1.Skill {
	return &v1.Skill{Id: in.ID, Name: in.Name, Description: in.Description, Source: in.Source, ChainId: in.ChainID, Frontmatter: jsonStruct(in.Frontmatter), Content: in.Content, HasFiles: in.HasFiles, CreatedAt: in.CreatedAt}
}

func skillProtoError(err error) error {
	var internalErr *biz.SkillInternalError
	if errors.As(err, &internalErr) {
		return internal(err)
	}
	return protoError(err)
}
