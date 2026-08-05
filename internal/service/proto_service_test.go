package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/peer"
	"gorm.io/datatypes"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

func TestAuthProtoServiceRejectsBlankLogin(t *testing.T) {
	service := NewAuthProtoService(nil, nil)

	_, err := service.Login(context.Background(), &v1.LoginRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected BadRequest for blank login, got %v", err)
	}
}

func TestAuthProtoServiceLoginStoresClientMetadataAndSetsCookie(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &stubAuthRepo{
		sessions: map[string]*po.Session{},
		users:    map[int64]*po.AdminUser{7: {ID: 7, Username: "admin", DisplayName: "管理员", PasswordHash: string(hash)}},
	}
	header := testHeader{}
	header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	header.Set("User-Agent", "baboflow-test")
	replyHeader := testHeader{}
	ctx := transport.NewServerContext(context.Background(), testTransport{
		kind: transport.KindHTTP, header: header, replyHeader: replyHeader,
	})
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 12345}})
	service := NewAuthProtoService(biz.NewAuthUsecase(repo), nil)

	user, err := service.Login(ctx, &v1.LoginRequest{Username: "admin", Password: "secret"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.UserId != 7 || user.Username != "admin" {
		t.Fatalf("unexpected login response: %#v", user)
	}
	if len(repo.sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(repo.sessions))
	}
	for _, session := range repo.sessions {
		if session.IP != "203.0.113.7" || session.UserAgent != "baboflow-test" {
			t.Fatalf("unexpected session metadata: %#v", session)
		}
	}
	if len(replyHeader.Values("Set-Cookie")) != 1 {
		t.Fatalf("expected session cookie, got %v", replyHeader.Values("Set-Cookie"))
	}
}

func TestArcheryProtoServiceRejectsInvalidConnectionID(t *testing.T) {
	service := NewArcheryProtoService(nil, nil)

	_, err := service.GetConnection(context.Background(), &v1.ArcheryIdRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected BadRequest for invalid connection ID, got %v", err)
	}
}

func TestArcheryProtoServiceListsConnections(t *testing.T) {
	uc := biz.NewArcheryUsecase(stubArcheryRepo{
		connections: []po.ArcheryConnection{{ID: 1, Name: "primary", Endpoint: "https://archery.example.com", Username: "admin"}},
	}, &conf.Config{Secret: "test"})
	service := NewArcheryProtoService(uc, nil)

	response, err := service.ListConnections(context.Background(), &v1.Empty{})
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if len(response.List) != 1 || response.List[0].Id != 1 || response.List[0].Name != "primary" {
		t.Fatalf("unexpected connection response: %#v", response)
	}
}

func TestArcheryProtoServiceRecordsSuccessfulMutations(t *testing.T) {
	secret := "test"
	passwordEnc, err := conf.Encrypt(secret, "secret")
	if err != nil {
		t.Fatal(err)
	}
	archery := fakeArcheryServer(t)
	repo := stubArcheryRepo{connections: []po.ArcheryConnection{{
		ID: 1, Name: "primary", Endpoint: archery.URL, Username: "admin", PasswordEnc: passwordEnc,
	}}, instances: []po.ArcheryInstance{{ID: 1, ConnectionID: 1, InstanceName: "mysql-primary", DBType: "mysql"}}}
	audits := make(chan *po.AuditLog, 1)
	service := NewArcheryProtoService(
		biz.NewArcheryUsecase(repo, &conf.Config{Secret: secret}),
		biz.NewAuditUsecase(recordingAuditRepo{created: audits}),
	)
	header := testHeader{}
	header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	ctx := context.WithValue(context.Background(), ctxUserID, int64(7))
	ctx = transport.NewServerContext(ctx, testTransport{kind: transport.KindHTTP, header: header})
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 12345}})

	tests := []struct {
		name     string
		call     func() error
		action   string
		targetID string
		detail   string
	}{
		{name: "create", call: func() error {
			_, err := service.CreateConnection(ctx, &v1.ArcheryConnectionInput{Name: "created", Endpoint: archery.URL, Username: "admin", Password: "secret"})
			return err
		}, action: biz.AuditArcheryCreate, targetID: "2", detail: `{"name":"created"}`},
		{name: "update", call: func() error {
			_, err := service.UpdateConnection(ctx, &v1.ArcheryConnectionRequest{Id: 1, Name: "updated", Endpoint: archery.URL, Username: "admin"})
			return err
		}, action: biz.AuditArcheryUpdate, targetID: "1", detail: `{"name":"updated"}`},
		{name: "delete", call: func() error {
			_, err := service.DeleteConnection(ctx, &v1.ArcheryIdRequest{Id: 1})
			return err
		}, action: biz.AuditArcheryDelete, targetID: "1"},
		{name: "set default", call: func() error {
			_, err := service.SetDefaultConnection(ctx, &v1.ArcheryIdRequest{Id: 1})
			return err
		}, action: biz.AuditArcheryUpdate, targetID: "1", detail: `{"default":true}`},
		{name: "clear default", call: func() error {
			_, err := service.ClearDefaultConnection(ctx, &v1.ArcheryIdRequest{Id: 1})
			return err
		}, action: biz.AuditArcheryUpdate, targetID: "1", detail: `{"default":false}`},
		{name: "sync instances", call: func() error {
			_, err := service.SyncInstances(ctx, &v1.ArcheryIdRequest{Id: 1})
			return err
		}, action: biz.AuditArcheryUpdate, targetID: "1", detail: `{"syncInstances":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("mutation failed: %v", err)
			}
			select {
			case audit := <-audits:
				if audit.UserID == nil || *audit.UserID != 7 || audit.IP != "203.0.113.9" || audit.Action != tt.action || audit.TargetType != "archery" || audit.TargetID != tt.targetID || string(audit.Detail) != tt.detail {
					t.Fatalf("unexpected audit: %#v", audit)
				}
			case <-time.After(time.Second):
				t.Fatal("expected audit record")
			}
		})
	}
}

func TestRuleChainPageFallsBackForOversizedPageSize(t *testing.T) {
	page, pageSize := ruleChainPage(&v1.PageRequest{Page: -1, PageSize: 201})
	if page != 1 || pageSize != 20 {
		t.Fatalf("expected page=1 pageSize=20, got page=%d pageSize=%d", page, pageSize)
	}
}

func TestLLMProtoServiceListsProviders(t *testing.T) {
	service := NewLLMProtoService(biz.NewLLMUsecase(stubLLMRepo{}, &conf.Config{Secret: "test"}), nil)
	response, err := service.ListProviders(context.Background(), &v1.Empty{})
	if err != nil || len(response.List) != 1 || response.List[0].Name != "local" {
		t.Fatalf("unexpected response: %#v, %v", response, err)
	}
	_, err = service.CreateProvider(context.Background(), &v1.ProviderInput{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestComponentProtoServiceListsComponents(t *testing.T) {
	service := NewComponentProtoService(stubComponentRepo{}, nil)
	response, err := service.List(context.Background(), &v1.ComponentListRequest{})
	if err != nil || len(response.List) != 1 || response.List[0].Type != "test" {
		t.Fatalf("unexpected response: %#v, %v", response, err)
	}
	_, err = service.GetSyncStatus(context.Background(), &v1.Empty{})
	if kerrors.Code(err) != 500 {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestCronProtoServiceRejectsInvalidCreate(t *testing.T) {
	service := NewCronProtoService(nil)
	_, err := service.Create(context.Background(), &v1.CronInput{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
	_, err = service.List(context.Background(), &v1.Empty{})
	if kerrors.Code(err) != 500 {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestCronProtoServiceCreatesAndTogglesJob(t *testing.T) {
	repo := &stubCronRepo{}
	service := NewCronProtoService(biz.NewCronUsecase(repo, nil))

	created, err := service.Create(context.Background(), &v1.CronInput{
		Name:         "定时执行",
		TargetType:   "chain",
		TargetId:     "chain_1",
		ScheduleType: "interval",
		IntervalSec:  60,
	})
	if err != nil {
		t.Fatalf("create cron job: %v", err)
	}
	if created.Id != 1 || created.Name != "定时执行" || created.TargetType != "chain" ||
		created.TargetId != "chain_1" || created.ScheduleType != "interval" ||
		created.IntervalSec != 60 || !created.Enabled {
		t.Fatalf("unexpected created cron job: %#v", created)
	}
	if repo.job == nil || string(repo.job.Payload) != "{}" {
		t.Fatalf("unexpected persisted cron job: %#v", repo.job)
	}

	toggled, err := service.Toggle(context.Background(), &v1.CronJobIdRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("toggle cron job: %v", err)
	}
	if toggled.Id != created.Id || toggled.Enabled || repo.updateCalls != 1 || repo.job.Enabled {
		t.Fatalf("unexpected toggled cron job: %#v, updateCalls=%d, persisted=%#v", toggled, repo.updateCalls, repo.job)
	}
}

func TestAgentProtoServiceListsAgentsAndRejectsBlankKey(t *testing.T) {
	now := time.Now().UTC()
	uc := biz.NewAgentUsecase(stubAgentRepo{agents: []po.Agent{{ID: 1, Key: "agent_1", Name: "示例", UpdatedAt: now}}}, nil, nil, nil, nil)
	service := NewAgentProtoService(uc)

	response, err := service.List(context.Background(), &v1.AgentListRequest{})
	if err != nil || len(response.List) != 1 || response.List[0].Key != "agent_1" {
		t.Fatalf("unexpected agent response: %#v, %v", response, err)
	}
	_, err = service.Get(context.Background(), &v1.AgentKeyRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestSkillProtoServiceListsSkillsAndRejectsBlankContent(t *testing.T) {
	uc := biz.NewSkillUsecase(stubSkillRepo{skills: []po.Skill{{ID: 1, Name: "example", Source: "upload"}}}, nil, nil)
	service := NewSkillProtoService(uc, nil)

	response, err := service.List(context.Background(), &v1.SkillListRequest{})
	if err != nil || len(response.List) != 1 || response.List[0].Name != "example" {
		t.Fatalf("unexpected skill response: %#v, %v", response, err)
	}
	_, err = service.Upload(context.Background(), &v1.UploadSkillRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestRuleChainProtoServiceListsChainsAndRuns(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	service := NewRuleChainProtoService(biz.NewRuleChainUsecase(stubRuleChainRepo{
		chains: []po.RuleChain{{
			ID: "chain_1", Name: "示例", Description: "说明", InputSchema: datatypes.JSON(`{"type":"object"}`),
			Status: "draft", Version: 2, Source: "manual", DebugMode: true, UpdatedAt: now,
		}},
		runs: []po.ChainRun{{
			ID: 7, ChainID: "chain_1", Trigger: "manual", Status: "success",
			Input: datatypes.JSON(`{"data":"hello"}`), Output: datatypes.JSON(`{"result":"ok"}`),
			NodeTrace: datatypes.JSON(`[{"nodeId":"node_1","durationMs":12,"input":{"msg":"in","metadata":{"x":"y"}}}]`),
			StartedAt: now,
		}},
	}, nil), nil)

	chains, err := service.List(context.Background(), &v1.RuleChainListRequest{})
	if err != nil || len(chains.List) != 1 || chains.List[0].Name != "示例" {
		t.Fatalf("unexpected chain response: %#v, %v", chains, err)
	}
	chain, err := service.Get(context.Background(), &v1.RuleChainIdRequest{Id: "chain_1"})
	if err != nil || chain.GetInputSchema().AsMap()["type"] != "object" {
		t.Fatalf("unexpected chain detail: %#v, %v", chain, err)
	}
	runs, err := service.ListRuns(context.Background(), &v1.RuleChainRunListRequest{})
	if err != nil || len(runs.List) != 1 || len(runs.List[0].NodeTrace) != 1 || runs.List[0].TaskId != nil {
		t.Fatalf("unexpected run response: %#v, %v", runs, err)
	}
}

func TestRuleChainProtoServiceRejectsInvalidRequests(t *testing.T) {
	service := NewRuleChainProtoService(nil, nil)
	_, err := service.Get(context.Background(), &v1.RuleChainIdRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
	response, err := service.Validate(context.Background(), &v1.ValidateRuleChainRequest{})
	if err != nil || response.Valid || response.Error == "" {
		t.Fatalf("unexpected validation response: %#v, %v", response, err)
	}
}

func TestMcpProtoServiceListsServersAndRejectsBlankName(t *testing.T) {
	service := NewMcpProtoService(biz.NewMcpUsecase(stubMcpRepo{}, nil), nil, nil)
	response, err := service.ListServers(context.Background(), &v1.Empty{})
	if err != nil || len(response.List) != 1 || response.List[0].Name != "example" {
		t.Fatalf("unexpected MCP response: %#v, %v", response, err)
	}
	_, err = service.CreateServer(context.Background(), &v1.McpServerInput{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestBoardProtoServiceListsBoardsAndRejectsInvalidID(t *testing.T) {
	service := NewBoardProtoService(biz.NewBoardUsecase(stubBoardRepo{}, nil), nil)
	response, err := service.List(context.Background(), &v1.Empty{})
	if err != nil || len(response.List) != 1 || response.List[0].Name != "example" {
		t.Fatalf("unexpected board response: %#v, %v", response, err)
	}
	_, err = service.Get(context.Background(), &v1.BoardIdRequest{})
	if kerrors.Code(err) != 400 {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestAuditProtoServiceListsLogsAndNormalizesPagination(t *testing.T) {
	service := NewAuditProtoService(biz.NewAuditUsecase(stubAuditRepo{}))
	response, err := service.List(context.Background(), &v1.AuditLogListRequest{})
	if err != nil || len(response.List) != 1 || response.Page.Page != 1 || response.Page.PageSize != 20 {
		t.Fatalf("unexpected audit response: %#v, %v", response, err)
	}
	_, err = NewAuditProtoService(nil).List(context.Background(), &v1.AuditLogListRequest{})
	if kerrors.Code(err) != 500 {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

type stubLLMRepo struct{}

func (stubLLMRepo) ListProviders(context.Context) ([]po.LLMProvider, error) {
	return []po.LLMProvider{{ID: 1, Name: "local", Provider: "openai", BaseURL: "http://example.test"}}, nil
}
func (stubLLMRepo) GetProvider(context.Context, int64) (*po.LLMProvider, error) { return nil, nil }
func (stubLLMRepo) CreateProvider(context.Context, *po.LLMProvider) error       { return nil }
func (stubLLMRepo) UpdateProvider(context.Context, *po.LLMProvider) error       { return nil }
func (stubLLMRepo) DeleteProvider(context.Context, int64) error                 { return nil }
func (stubLLMRepo) ListModels(context.Context, int64) ([]po.LLMModel, error)    { return nil, nil }
func (stubLLMRepo) GetModel(context.Context, int64) (*po.LLMModel, error)       { return nil, nil }
func (stubLLMRepo) CreateModel(context.Context, *po.LLMModel) error             { return nil }
func (stubLLMRepo) UpdateModel(context.Context, *po.LLMModel) error             { return nil }
func (stubLLMRepo) DeleteModel(context.Context, int64) error                    { return nil }
func (stubLLMRepo) SetDefaultModel(context.Context, int64, int64) error         { return nil }
func (stubLLMRepo) CountAgentByModel(context.Context, int64) (int64, error)     { return 0, nil }

type stubComponentRepo struct{}

func (stubComponentRepo) ListAll(context.Context) ([]po.ComponentMeta, error)  { return nil, nil }
func (stubComponentRepo) Upsert(context.Context, *po.ComponentMeta) error      { return nil }
func (stubComponentRepo) MarkMissing(context.Context, []string) (int64, error) { return 0, nil }
func (stubComponentRepo) SearchKeyword(context.Context, string, string) ([]po.ComponentMeta, error) {
	return []po.ComponentMeta{{Type: "test", Name: "Test", ConfigSchema: datatypes.JSON(`{}`), Example: datatypes.JSON(`{}`)}}, nil
}

type stubCronRepo struct {
	job         *po.CronJob
	updateCalls int
}

func (s *stubCronRepo) List(context.Context) ([]po.CronJob, error) {
	if s.job == nil {
		return nil, nil
	}
	return []po.CronJob{*s.job}, nil
}
func (s *stubCronRepo) ListEnabled(context.Context) ([]po.CronJob, error) {
	if s.job == nil || !s.job.Enabled {
		return nil, nil
	}
	return []po.CronJob{*s.job}, nil
}
func (s *stubCronRepo) GetByID(_ context.Context, id int64) (*po.CronJob, error) {
	if s.job == nil || s.job.ID != id {
		return nil, biz.ErrNotFound
	}
	return s.job, nil
}
func (s *stubCronRepo) Create(_ context.Context, job *po.CronJob) error {
	now := time.Now().UTC().Truncate(time.Second)
	job.ID = 1
	job.CreatedAt = now
	job.UpdatedAt = now
	s.job = job
	return nil
}
func (s *stubCronRepo) Update(_ context.Context, job *po.CronJob) error {
	s.updateCalls++
	s.job = job
	return nil
}
func (s *stubCronRepo) Delete(context.Context, int64) error { return nil }

type stubMcpRepo struct{}

func (stubMcpRepo) ListServers(context.Context) ([]po.McpServer, error) {
	return []po.McpServer{{ID: 1, Name: "example", Args: datatypes.JSON(`[]`), Env: datatypes.JSON(`{}`)}}, nil
}
func (stubMcpRepo) GetServer(context.Context, int64) (*po.McpServer, error) {
	return nil, biz.ErrNotFound
}
func (stubMcpRepo) CreateServer(context.Context, *po.McpServer) error       { return nil }
func (stubMcpRepo) UpdateServer(context.Context, *po.McpServer) error       { return nil }
func (stubMcpRepo) DeleteServer(context.Context, int64) error               { return nil }
func (stubMcpRepo) ListExposures(context.Context) ([]po.McpExposure, error) { return nil, nil }
func (stubMcpRepo) ListEnabledExposures(context.Context) ([]po.McpExposure, error) {
	return nil, nil
}
func (stubMcpRepo) GetExposure(context.Context, int64) (*po.McpExposure, error) {
	return nil, biz.ErrNotFound
}
func (stubMcpRepo) GetExposureByTool(context.Context, string) (*po.McpExposure, error) {
	return nil, biz.ErrNotFound
}
func (stubMcpRepo) CreateExposure(context.Context, *po.McpExposure) error { return nil }
func (stubMcpRepo) UpdateExposure(context.Context, *po.McpExposure) error { return nil }
func (stubMcpRepo) DeleteExposure(context.Context, int64) error           { return nil }

type stubBoardRepo struct{}

func (stubBoardRepo) ListBoards(context.Context) ([]po.Board, error) {
	return []po.Board{{ID: 1, Name: "example"}}, nil
}
func (stubBoardRepo) GetBoard(context.Context, int64) (*po.Board, error) { return nil, biz.ErrNotFound }
func (stubBoardRepo) CreateBoard(context.Context, *po.Board) error       { return nil }
func (stubBoardRepo) UpdateBoard(context.Context, *po.Board) error       { return nil }
func (stubBoardRepo) DeleteBoard(context.Context, int64) error           { return nil }
func (stubBoardRepo) ListColumns(context.Context, int64) ([]po.BoardColumn, error) {
	return nil, nil
}
func (stubBoardRepo) GetColumn(context.Context, int64) (*po.BoardColumn, error) {
	return nil, biz.ErrNotFound
}
func (stubBoardRepo) CreateColumn(context.Context, *po.BoardColumn) error { return nil }
func (stubBoardRepo) UpdateColumn(context.Context, *po.BoardColumn) error { return nil }
func (stubBoardRepo) DeleteColumn(context.Context, int64) error           { return nil }
func (stubBoardRepo) ListTasksByBoard(context.Context, int64) ([]po.Task, error) {
	return nil, nil
}
func (stubBoardRepo) GetTask(context.Context, int64) (*po.Task, error) { return nil, biz.ErrNotFound }
func (stubBoardRepo) CreateTask(context.Context, *po.Task) error       { return nil }
func (stubBoardRepo) UpdateTask(context.Context, *po.Task) error       { return nil }
func (stubBoardRepo) DeleteTask(context.Context, int64) error          { return nil }

type stubAuditRepo struct{}

func (stubAuditRepo) Create(context.Context, *po.AuditLog) error { return nil }
func (stubAuditRepo) List(context.Context, string, *int64, int, int) ([]po.AuditLog, int64, error) {
	return []po.AuditLog{{ID: 1, Action: "auth.login", Detail: datatypes.JSON(`{}`)}}, 1, nil
}

type recordingAuditRepo struct {
	created chan<- *po.AuditLog
}

func (r recordingAuditRepo) Create(_ context.Context, audit *po.AuditLog) error {
	r.created <- audit
	return nil
}
func (recordingAuditRepo) List(context.Context, string, *int64, int, int) ([]po.AuditLog, int64, error) {
	return nil, 0, nil
}

type stubArcheryRepo struct {
	connections []po.ArcheryConnection
	instances   []po.ArcheryInstance
}

func (s stubArcheryRepo) ListConnections(context.Context) ([]po.ArcheryConnection, error) {
	return s.connections, nil
}
func (s stubArcheryRepo) GetConnection(_ context.Context, id int64) (*po.ArcheryConnection, error) {
	for i := range s.connections {
		if s.connections[i].ID == id {
			return &s.connections[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
func (stubArcheryRepo) GetConnectionByName(context.Context, string) (*po.ArcheryConnection, error) {
	return nil, biz.ErrNotFound
}
func (stubArcheryRepo) CreateConnection(_ context.Context, connection *po.ArcheryConnection) error {
	connection.ID = 2
	return nil
}
func (stubArcheryRepo) UpdateConnection(context.Context, *po.ArcheryConnection) error { return nil }
func (stubArcheryRepo) DeleteConnection(context.Context, int64) error                 { return nil }
func (stubArcheryRepo) SetDefaultConnection(context.Context, int64) error             { return nil }
func (stubArcheryRepo) ClearDefaultConnection(context.Context, int64) error           { return nil }
func (stubArcheryRepo) GetDefaultConnection(context.Context, int64) (*po.ArcheryConnection, error) {
	return nil, biz.ErrNotFound
}
func (s stubArcheryRepo) ListInstances(context.Context, int64) ([]po.ArcheryInstance, error) {
	return s.instances, nil
}
func (stubArcheryRepo) GetInstance(context.Context, int64) (*po.ArcheryInstance, error) {
	return nil, biz.ErrNotFound
}
func (stubArcheryRepo) UpsertInstance(context.Context, *po.ArcheryInstance) error { return nil }
func (stubArcheryRepo) DeleteInstancesNotIn(context.Context, int64, []string) error {
	return nil
}

func fakeArcheryServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf", Path: "/"})
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "session", Path: "/"})
		_, _ = w.Write([]byte(`{"status":0}`))
	})
	mux.HandleFunc("/group/user_all_instances/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sessionid"); err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"status":0,"data":[{"instance_name":"mysql-primary","db_type":"mysql"}]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

type stubRuleChainRepo struct {
	chains []po.RuleChain
	runs   []po.ChainRun
}

type stubAgentRepo struct {
	agents []po.Agent
}

func (s stubAgentRepo) ListAgents(context.Context, string) ([]po.Agent, error) { return s.agents, nil }
func (s stubAgentRepo) GetAgentByKey(_ context.Context, key string) (*po.Agent, error) {
	for i := range s.agents {
		if s.agents[i].Key == key {
			return &s.agents[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
func (stubAgentRepo) CreateAgent(context.Context, *po.Agent) error            { return nil }
func (stubAgentRepo) UpdateAgent(context.Context, *po.Agent) error            { return nil }
func (stubAgentRepo) DeleteAgent(context.Context, int64) error                { return nil }
func (stubAgentRepo) SetSubAgents(context.Context, int64, []int64) error      { return nil }
func (stubAgentRepo) ListSubAgentIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (stubAgentRepo) CreateSession(context.Context, *po.AgentSession) error   { return nil }
func (stubAgentRepo) GetSession(context.Context, string) (*po.AgentSession, error) {
	return nil, biz.ErrNotFound
}
func (stubAgentRepo) ListSessions(context.Context, string, int64) ([]po.AgentSession, error) {
	return nil, nil
}
func (stubAgentRepo) UpdateSessionTitle(context.Context, string, string) error { return nil }
func (stubAgentRepo) DeleteSession(context.Context, string) error              { return nil }
func (stubAgentRepo) CreateMessage(context.Context, *po.AgentMessage) error    { return nil }
func (stubAgentRepo) ListMessages(context.Context, string, int) ([]po.AgentMessage, error) {
	return nil, nil
}
func (stubAgentRepo) CreateAsset(context.Context, *po.Asset) error       { return nil }
func (stubAgentRepo) GetAsset(context.Context, int64) (*po.Asset, error) { return nil, biz.ErrNotFound }

type stubSkillRepo struct {
	skills []po.Skill
}

func (s stubSkillRepo) List(context.Context, string, string) ([]po.Skill, error) {
	return s.skills, nil
}
func (s stubSkillRepo) GetByID(_ context.Context, id int64) (*po.Skill, error) {
	for i := range s.skills {
		if s.skills[i].ID == id {
			return &s.skills[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
func (s stubSkillRepo) GetByName(_ context.Context, name string) (*po.Skill, error) {
	for i := range s.skills {
		if s.skills[i].Name == name {
			return &s.skills[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
func (stubSkillRepo) Create(context.Context, *po.Skill) error { return nil }
func (stubSkillRepo) Update(context.Context, *po.Skill) error { return nil }
func (stubSkillRepo) Delete(context.Context, int64) error     { return nil }

func (s stubRuleChainRepo) Create(context.Context, *po.RuleChain) error { return nil }
func (s stubRuleChainRepo) Update(context.Context, *po.RuleChain) error { return nil }
func (s stubRuleChainRepo) Get(_ context.Context, id string) (*po.RuleChain, error) {
	for i := range s.chains {
		if s.chains[i].ID == id {
			return &s.chains[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
func (s stubRuleChainRepo) List(context.Context, string, string, int, int) ([]po.RuleChain, int64, error) {
	return s.chains, int64(len(s.chains)), nil
}
func (s stubRuleChainRepo) Delete(context.Context, string) error { return nil }
func (s stubRuleChainRepo) CreateVersion(context.Context, *po.RuleChainVersion) error {
	return nil
}
func (s stubRuleChainRepo) ListVersions(context.Context, string) ([]po.RuleChainVersion, error) {
	return nil, nil
}
func (s stubRuleChainRepo) GetVersion(context.Context, string, int) (*po.RuleChainVersion, error) {
	return nil, biz.ErrNotFound
}
func (s stubRuleChainRepo) CreateRun(context.Context, *po.ChainRun) error { return nil }
func (s stubRuleChainRepo) UpdateRun(context.Context, *po.ChainRun) error { return nil }
func (s stubRuleChainRepo) ListRuns(context.Context, string, string, int, int) ([]po.ChainRun, int64, error) {
	return s.runs, int64(len(s.runs)), nil
}
func (s stubRuleChainRepo) GetRun(_ context.Context, id int64) (*po.ChainRun, error) {
	for i := range s.runs {
		if s.runs[i].ID == id {
			return &s.runs[i], nil
		}
	}
	return nil, biz.ErrNotFound
}
