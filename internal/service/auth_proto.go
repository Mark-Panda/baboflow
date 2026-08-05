package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport"
	"gorm.io/gorm"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
)

// AuthProtoService 将已认证的 Kratos context 和 AuthUsecase 适配为 proto 接口。
type AuthProtoService struct {
	v1.UnimplementedAuthServiceServer
	auth    *biz.AuthUsecase
	auditor *biz.AuditUsecase
}

func NewAuthProtoService(auth *biz.AuthUsecase, auditor *biz.AuditUsecase) *AuthProtoService {
	return &AuthProtoService{auth: auth, auditor: auditor}
}

func (s *AuthProtoService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.User, error) {
	if req == nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		return nil, kerrors.BadRequest("INVALID_LOGIN", "用户名和密码不能为空")
	}
	if s.auth == nil {
		return nil, kerrors.InternalServer("AUTH_UNAVAILABLE", "认证服务未就绪")
	}
	ip, userAgent := ClientMetadataFromContext(ctx)
	result, err := s.auth.Login(ctx, req.Username, req.Password, ip, userAgent)
	if err != nil {
		s.auditRecord(ctx, nil, biz.AuditLoginFailed, req.Username, map[string]any{"reason": err.Error()})
		return nil, kerrors.Unauthorized("BAD_CREDENTIAL", err.Error())
	}
	if tr, ok := transport.FromServerContext(ctx); ok {
		setTransportCookie(ctx, tr.ReplyHeader(), result.SessionID, 7*24*3600)
	}
	s.auditRecord(ctx, &result.UserID, biz.AuditLogin, req.Username, nil)
	return &v1.User{UserId: result.UserID, Username: result.Username, DisplayName: result.DisplayName, MustChangePwd: result.MustChangePwd}, nil
}

func (s *AuthProtoService) Logout(ctx context.Context, _ *v1.Empty) (*v1.LogoutResponse, error) {
	if s.auth == nil {
		return nil, kerrors.InternalServer("AUTH_UNAVAILABLE", "认证服务未就绪")
	}
	_ = s.auth.Logout(ctx, sessionID(ctx))
	uid := currentUserID(ctx)
	s.auditRecord(ctx, &uid, biz.AuditLogout, "", nil)
	if tr, ok := transport.FromServerContext(ctx); ok {
		setTransportCookie(ctx, tr.ReplyHeader(), "", -1)
	}
	return &v1.LogoutResponse{Ok: true}, nil
}

func (s *AuthProtoService) GetMe(ctx context.Context, _ *v1.Empty) (*v1.User, error) {
	if s.auth == nil {
		return nil, kerrors.InternalServer("AUTH_UNAVAILABLE", "认证服务未就绪")
	}
	user, err := s.auth.Me(ctx, currentUserID(ctx))
	if err != nil {
		return nil, kerrors.Unauthorized("AUTH_INVALID", "会话无效")
	}
	return &v1.User{UserId: user.ID, Username: user.Username, DisplayName: user.DisplayName, MustChangePwd: user.MustChangePwd, Avatar: user.Avatar, Email: user.Email}, nil
}

func (s *AuthProtoService) ChangePassword(ctx context.Context, req *v1.ChangePasswordRequest) (*v1.LogoutResponse, error) {
	if req == nil || req.OldPassword == "" || len(req.NewPassword) < 6 {
		return nil, kerrors.BadRequest("INVALID_PASSWORD", "旧密码必填且新密码至少 6 位")
	}
	if s.auth == nil {
		return nil, kerrors.InternalServer("AUTH_UNAVAILABLE", "认证服务未就绪")
	}
	if err := s.auth.ChangePassword(ctx, currentUserID(ctx), req.OldPassword, req.NewPassword, sessionID(ctx)); err != nil {
		return nil, kerrors.BadRequest("CHANGE_PASSWORD_FAILED", err.Error())
	}
	uid := currentUserID(ctx)
	s.auditRecord(ctx, &uid, biz.AuditChangePassword, "", nil)
	return &v1.LogoutResponse{Ok: true}, nil
}

func (s *AuthProtoService) auditRecord(ctx context.Context, userID *int64, action, targetID string, detail map[string]any) {
	if s.auditor != nil {
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, userID, action, "auth", targetID, ip, detail)
	}
}

func setTransportCookie(ctx context.Context, header transport.Header, value string, maxAge int) {
	cookie := (&http.Cookie{
		Name:     biz.SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   secureCookie(ctx),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}).String()
	header.Add("Set-Cookie", cookie)
}

// ArcheryProtoService 仅负责参数校验与 proto/biz 类型转换。
type ArcheryProtoService struct {
	v1.UnimplementedArcheryServiceServer
	uc      *biz.ArcheryUsecase
	auditor *biz.AuditUsecase
}

func NewArcheryProtoService(uc *biz.ArcheryUsecase, auditor *biz.AuditUsecase) *ArcheryProtoService {
	return &ArcheryProtoService{uc: uc, auditor: auditor}
}

func (s *ArcheryProtoService) ListConnections(ctx context.Context, _ *v1.Empty) (*v1.ArcheryConnectionList, error) {
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	list, err := s.uc.ListConnections(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]*v1.ArcheryConnection, 0, len(list))
	for _, item := range list {
		out = append(out, archeryConnection(item))
	}
	return &v1.ArcheryConnectionList{List: out}, nil
}

func (s *ArcheryProtoService) CreateConnection(ctx context.Context, req *v1.ArcheryConnectionInput) (*v1.ArcheryCreateResponse, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Endpoint) == "" || strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		return nil, kerrors.BadRequest("INVALID_CONNECTION", "name、endpoint、username 和 password 必填")
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	conn, err := s.uc.CreateConnection(ctx, archeryInput(req.Name, req.Endpoint, req.Username, req.Password, req.Insecure, req.CaCert, req.Remark))
	if err != nil {
		return nil, badRequest(err)
	}
	s.audit(ctx, biz.AuditArcheryCreate, conn.ID, map[string]any{"name": req.Name})
	return &v1.ArcheryCreateResponse{Id: conn.ID}, nil
}

func (s *ArcheryProtoService) GetConnection(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryConnection, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	conn, err := s.uc.GetConnection(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	return archeryConnection(conn), nil
}

func (s *ArcheryProtoService) UpdateConnection(ctx context.Context, req *v1.ArcheryConnectionRequest) (*v1.ArcheryOkResponse, error) {
	if req == nil || validID(req.Id) != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Endpoint) == "" || strings.TrimSpace(req.Username) == "" {
		return nil, kerrors.BadRequest("INVALID_CONNECTION", "id、name、endpoint 和 username 必填")
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	if err := s.uc.UpdateConnection(ctx, req.Id, archeryInput(req.Name, req.Endpoint, req.Username, req.Password, req.Insecure, req.CaCert, req.Remark)); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditArcheryUpdate, req.Id, map[string]any{"name": req.Name})
	return &v1.ArcheryOkResponse{Ok: true}, nil
}

func (s *ArcheryProtoService) DeleteConnection(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryOkResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	if err := s.uc.DeleteConnection(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditArcheryDelete, req.Id, nil)
	return &v1.ArcheryOkResponse{Ok: true}, nil
}

func (s *ArcheryProtoService) SetDefaultConnection(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryOkResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	if err := s.uc.SetDefaultConnection(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditArcheryUpdate, req.Id, map[string]any{"default": true})
	return &v1.ArcheryOkResponse{Ok: true}, nil
}

func (s *ArcheryProtoService) ClearDefaultConnection(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryOkResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	if err := s.uc.ClearDefaultConnection(ctx, req.Id); err != nil {
		return nil, protoError(err)
	}
	s.audit(ctx, biz.AuditArcheryUpdate, req.Id, map[string]any{"default": false})
	return &v1.ArcheryOkResponse{Ok: true}, nil
}

func (s *ArcheryProtoService) TestConnection(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryTestResult, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	result, err := s.uc.TestConnection(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	out := &v1.ArcheryTestResult{}
	if ok, _ := result["ok"].(bool); ok {
		out.Ok = true
	}
	if count, ok := result["instances"].(int); ok {
		out.Instances = int32(count)
	}
	if message, ok := result["error"].(string); ok {
		out.Error = message
	}
	return out, nil
}

func (s *ArcheryProtoService) ListInstances(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryInstanceList, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	list, err := s.uc.ListInstances(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	out := make([]*v1.ArcheryInstance, 0, len(list))
	for _, item := range list {
		out = append(out, &v1.ArcheryInstance{Id: item.ID, ConnectionId: item.ConnectionID, InstanceName: item.InstanceName, DbType: item.DBType})
	}
	return &v1.ArcheryInstanceList{List: out}, nil
}

func (s *ArcheryProtoService) SyncInstances(ctx context.Context, req *v1.ArcheryIdRequest) (*v1.ArcheryInstanceList, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if s.uc == nil {
		return nil, unavailable("ARCHERY_UNAVAILABLE")
	}
	list, err := s.uc.SyncInstances(ctx, req.Id)
	if err != nil {
		return nil, protoError(err)
	}
	out := make([]*v1.ArcheryInstance, 0, len(list))
	for _, item := range list {
		out = append(out, &v1.ArcheryInstance{Id: item.ID, ConnectionId: item.ConnectionID, InstanceName: item.InstanceName, DbType: item.DBType})
	}
	s.audit(ctx, biz.AuditArcheryUpdate, req.Id, map[string]any{"syncInstances": len(list)})
	return &v1.ArcheryInstanceList{List: out}, nil
}

func (s *ArcheryProtoService) audit(ctx context.Context, action string, targetID int64, detail map[string]any) {
	if s.auditor != nil {
		userID := currentUserID(ctx)
		ip, _ := ClientMetadataFromContext(ctx)
		s.auditor.Record(ctx, &userID, action, "archery", strconv.FormatInt(targetID, 10), ip, detail)
	}
}

func archeryInput(name, endpoint, username, password string, insecure bool, caCert, remark string) *biz.ConnectionInput {
	return &biz.ConnectionInput{Name: name, Endpoint: endpoint, Username: username, Password: password, Insecure: insecure, CACert: caCert, Remark: remark}
}

func archeryConnection(value biz.ConnectionView) *v1.ArcheryConnection {
	return &v1.ArcheryConnection{Id: value.ID, Name: value.Name, Endpoint: value.Endpoint, Username: value.Username, Password: value.Password, Insecure: value.Insecure, CaCert: value.CACert, Remark: value.Remark, InstanceCount: int32(value.InstanceCount), IsDefault: value.IsDefault, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func currentUserID(ctx context.Context) int64 { id, _ := ctx.Value(ctxUserID).(int64); return id }
func sessionID(ctx context.Context) string    { id, _ := ctx.Value(ctxSession).(string); return id }
func validID(id int64) error {
	if id <= 0 {
		return kerrors.BadRequest("INVALID_ID", "id 必须大于 0")
	}
	return nil
}
func unavailable(reason string) error { return kerrors.InternalServer(reason, "服务未就绪") }
func internal(err error) error        { return kerrors.InternalServer("INTERNAL_ERROR", err.Error()) }
func badRequest(err error) error      { return kerrors.BadRequest("INVALID_REQUEST", err.Error()) }
func protoError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, biz.ErrNotFound) {
		return kerrors.NotFound("NOT_FOUND", "资源不存在")
	}
	if errors.Is(err, biz.ErrReferenced) || errors.Is(err, biz.ErrChainPublished) || errors.Is(err, biz.ErrChainNotLoaded) {
		return kerrors.Conflict("CONFLICT", err.Error())
	}
	return kerrors.BadRequest("BUSINESS_ERROR", err.Error())
}
