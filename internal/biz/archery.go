package biz

import (
	"context"
	"errors"
	"strings"

	"baboflow/internal/biz/rulegokit/archeryclient"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// ArcheryRepo 是 archery_connection / archery_instance 表的存储抽象。
type ArcheryRepo interface {
	ListConnections(ctx context.Context) ([]po.ArcheryConnection, error)
	GetConnection(ctx context.Context, id int64) (*po.ArcheryConnection, error)
	GetConnectionByName(ctx context.Context, name string) (*po.ArcheryConnection, error)
	CreateConnection(ctx context.Context, c *po.ArcheryConnection) error
	UpdateConnection(ctx context.Context, c *po.ArcheryConnection) error
	DeleteConnection(ctx context.Context, id int64) error
	SetDefaultConnection(ctx context.Context, id int64) error
	ClearDefaultConnection(ctx context.Context, id int64) error
	GetDefaultConnection(ctx context.Context, tenantID int64) (*po.ArcheryConnection, error)

	ListInstances(ctx context.Context, connectionID int64) ([]po.ArcheryInstance, error)
	GetInstance(ctx context.Context, id int64) (*po.ArcheryInstance, error)
	UpsertInstance(ctx context.Context, in *po.ArcheryInstance) error
	DeleteInstancesNotIn(ctx context.Context, connectionID int64, keep []string) error
}

// ArcheryUsecase 管理 Archery 连接（凭据加密存库，同 LLMUsecase 模式），
// 并为 archery 规则链节点提供"按 ID 取解密连接并构造 HTTP 客户端"的能力。
type ArcheryUsecase struct {
	repo   ArcheryRepo
	secret string
}

func NewArcheryUsecase(repo ArcheryRepo, c *conf.Config) *ArcheryUsecase {
	return &ArcheryUsecase{repo: repo, secret: c.Secret}
}

// ConnectionView 连接的回显视图：密码只回脱敏形式。
type ConnectionView struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Endpoint      string `json:"endpoint"`
	Username      string `json:"username"`
	Password      string `json:"password"` // 脱敏后的掩码
	Insecure      bool   `json:"insecure"`
	CACert        string `json:"caCert"`
	Remark        string `json:"remark"`
	InstanceCount int    `json:"instanceCount"` // 该连接下已同步的实例数
	IsDefault     bool   `json:"isDefault"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func (uc *ArcheryUsecase) toView(c *po.ArcheryConnection, instanceCount int) ConnectionView {
	pwd, _ := conf.Decrypt(uc.secret, c.PasswordEnc)
	return ConnectionView{
		ID: c.ID, Name: c.Name, Endpoint: c.Endpoint,
		Username: c.Username, Password: conf.Mask(pwd), Insecure: c.Insecure,
		CACert: c.CACert, Remark: c.Remark, InstanceCount: instanceCount, IsDefault: c.IsDefault,
		CreatedAt: c.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func (uc *ArcheryUsecase) ListConnections(ctx context.Context) ([]ConnectionView, error) {
	list, err := uc.repo.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectionView, 0, len(list))
	for i := range list {
		ins, _ := uc.repo.ListInstances(ctx, list[i].ID)
		out = append(out, uc.toView(&list[i], len(ins)))
	}
	return out, nil
}

func (uc *ArcheryUsecase) GetConnection(ctx context.Context, id int64) (ConnectionView, error) {
	c, err := uc.repo.GetConnection(ctx, id)
	if err != nil {
		return ConnectionView{}, err
	}
	ins, _ := uc.repo.ListInstances(ctx, id)
	return uc.toView(c, len(ins)), nil
}

// ConnectionInput 创建/更新连接的入参；Password 更新时留空表示不修改。
// 连接只含地址+凭据；实例由「更新实例」从 Archery 拉取，不在此填写。
type ConnectionInput struct {
	Name     string `json:"name" binding:"required"`
	Endpoint string `json:"endpoint" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password"` // 创建必填；更新留空=不修改
	Insecure bool   `json:"insecure"`
	CACert   string `json:"caCert"`
	Remark   string `json:"remark"`
}

func validateConnectionInput(in *ConnectionInput) error {
	if strings.TrimSpace(in.Endpoint) == "" {
		return errors.New("endpoint 不能为空")
	}
	in.Endpoint = strings.TrimRight(strings.TrimSpace(in.Endpoint), "/")
	if strings.TrimSpace(in.Username) == "" {
		return errors.New("username 不能为空")
	}
	return nil
}

func (uc *ArcheryUsecase) CreateConnection(ctx context.Context, in *ConnectionInput) (*po.ArcheryConnection, error) {
	if err := validateConnectionInput(in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Password) == "" {
		return nil, errors.New("password 不能为空")
	}
	enc, err := conf.Encrypt(uc.secret, in.Password)
	if err != nil {
		return nil, err
	}
	c := &po.ArcheryConnection{
		Name: strings.TrimSpace(in.Name), Endpoint: in.Endpoint,
		Username: strings.TrimSpace(in.Username), PasswordEnc: enc, Insecure: in.Insecure,
		CACert: in.CACert, Remark: in.Remark,
	}
	if err := uc.repo.CreateConnection(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (uc *ArcheryUsecase) UpdateConnection(ctx context.Context, id int64, in *ConnectionInput) error {
	if err := validateConnectionInput(in); err != nil {
		return err
	}
	c, err := uc.repo.GetConnection(ctx, id)
	if err != nil {
		return err
	}
	c.Name = strings.TrimSpace(in.Name)
	c.Endpoint = in.Endpoint
	c.Username = strings.TrimSpace(in.Username)
	c.Insecure = in.Insecure
	c.CACert = in.CACert
	c.Remark = in.Remark
	if strings.TrimSpace(in.Password) != "" {
		enc, err := conf.Encrypt(uc.secret, in.Password)
		if err != nil {
			return err
		}
		c.PasswordEnc = enc
	}
	return uc.repo.UpdateConnection(ctx, c)
}

func (uc *ArcheryUsecase) DeleteConnection(ctx context.Context, id int64) error {
	return uc.repo.DeleteConnection(ctx, id)
}

func (uc *ArcheryUsecase) SetDefaultConnection(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetConnection(ctx, id); err != nil {
		return err
	}
	return uc.repo.SetDefaultConnection(ctx, id)
}

func (uc *ArcheryUsecase) ClearDefaultConnection(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetConnection(ctx, id); err != nil {
		return err
	}
	return uc.repo.ClearDefaultConnection(ctx, id)
}

// newSiteClient 解密连接凭据，构造一个面向整个站点（未绑定具体实例）的客户端。
// 用于登录 / 测试连接 / 拉取实例列表（这些操作不需要 instance_name）。
func (uc *ArcheryUsecase) newSiteClient(c *po.ArcheryConnection) (*archeryclient.Client, error) {
	pwd, err := conf.Decrypt(uc.secret, c.PasswordEnc)
	if err != nil {
		return nil, err
	}
	return archeryclient.New(archeryclient.Config{
		Endpoint: c.Endpoint, Username: c.Username, Password: pwd,
		Insecure: c.Insecure, CACert: c.CACert,
	})
}

// NewClientForInstance 按实例 ID 取其实例名 + 所属连接凭据，构造绑定该实例的查询客户端。
// 供 archery 规则链节点的 ClientFactory 注入（见 nodes.SetArcheryClientFactory）。
func (uc *ArcheryUsecase) NewClientForInstance(ctx context.Context, instanceID int64) (*archeryclient.Client, error) {
	in, err := uc.repo.GetInstance(ctx, instanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c, err := uc.repo.GetConnection(ctx, in.ConnectionID)
	if err != nil {
		return nil, err
	}
	pwd, err := conf.Decrypt(uc.secret, c.PasswordEnc)
	if err != nil {
		return nil, err
	}
	return archeryclient.New(archeryclient.Config{
		Endpoint: c.Endpoint, Instance: in.InstanceName, DBType: in.DBType,
		Username: c.Username, Password: pwd,
		Insecure: c.Insecure, CACert: c.CACert,
	})
}

func (uc *ArcheryUsecase) NewClientForDefaultConnection(ctx context.Context) (*archeryclient.Client, error) {
	c, err := uc.repo.GetDefaultConnection(ctx, 0)
	if err != nil {
		return nil, errors.New("未配置默认 Archery connection")
	}
	return uc.newSiteClient(c)
}

func (uc *ArcheryUsecase) ListDefaultInstances(ctx context.Context) ([]archeryclient.InstanceInfo, error) {
	c, err := uc.repo.GetDefaultConnection(ctx, 0)
	if err != nil {
		return nil, errors.New("未配置默认 Archery connection")
	}
	list, err := uc.SyncInstances(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]archeryclient.InstanceInfo, 0, len(list))
	for _, in := range list {
		out = append(out, archeryclient.InstanceInfo{ID: in.ID, InstanceName: in.InstanceName, DBType: in.DBType})
	}
	return out, nil
}

// InstanceView 实例回显。
type InstanceView struct {
	ID           int64  `json:"id"`
	ConnectionID int64  `json:"connectionId"`
	InstanceName string `json:"instanceName"`
	DBType       string `json:"dbType"`
}

func toInstanceView(in *po.ArcheryInstance) InstanceView {
	return InstanceView{ID: in.ID, ConnectionID: in.ConnectionID, InstanceName: in.InstanceName, DBType: in.DBType}
}

// ListInstances 返回某连接下已同步的实例。
func (uc *ArcheryUsecase) ListInstances(ctx context.Context, connectionID int64) ([]InstanceView, error) {
	list, err := uc.repo.ListInstances(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	out := make([]InstanceView, 0, len(list))
	for i := range list {
		out = append(out, toInstanceView(&list[i]))
	}
	return out, nil
}

// SyncInstances 从 Archery 拉取该连接（站点）下当前用户可访问的全部实例并 upsert：
// 新增/更新实例，删除 Archery 端已不存在的实例。返回同步后的实例列表。
func (uc *ArcheryUsecase) SyncInstances(ctx context.Context, connectionID int64) ([]InstanceView, error) {
	c, err := uc.repo.GetConnection(ctx, connectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	cli, err := uc.newSiteClient(c)
	if err != nil {
		return nil, err
	}
	remote, err := cli.ListInstances()
	if err != nil {
		return nil, err
	}
	keep := make([]string, 0, len(remote))
	for _, ri := range remote {
		name := strings.TrimSpace(ri.InstanceName)
		if name == "" {
			continue
		}
		keep = append(keep, name)
		if err := uc.repo.UpsertInstance(ctx, &po.ArcheryInstance{
			ConnectionID: connectionID, InstanceName: name, DBType: ri.DBType,
		}); err != nil {
			return nil, err
		}
	}
	if err := uc.repo.DeleteInstancesNotIn(ctx, connectionID, keep); err != nil {
		return nil, err
	}
	return uc.ListInstances(ctx, connectionID)
}

// TestConnection 验证连接可用（登录 + 拉取实例列表），供"测试连接"按钮。
func (uc *ArcheryUsecase) TestConnection(ctx context.Context, id int64) (map[string]any, error) {
	c, err := uc.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	cli, err := uc.newSiteClient(c)
	if err != nil {
		return nil, err
	}
	if err := cli.Login(); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	ins, err := cli.ListInstances()
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "instances": len(ins)}, nil
}
