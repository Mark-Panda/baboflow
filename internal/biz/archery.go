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

// ArcheryRepo 是 archery_connection 表的存储抽象。
type ArcheryRepo interface {
	ListConnections(ctx context.Context) ([]po.ArcheryConnection, error)
	GetConnection(ctx context.Context, id int64) (*po.ArcheryConnection, error)
	GetConnectionByName(ctx context.Context, name string) (*po.ArcheryConnection, error)
	CreateConnection(ctx context.Context, c *po.ArcheryConnection) error
	UpdateConnection(ctx context.Context, c *po.ArcheryConnection) error
	DeleteConnection(ctx context.Context, id int64) error
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
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	Instance  string `json:"instance"`
	Username  string `json:"username"`
	Password  string `json:"password"` // 脱敏后的掩码
	Insecure  bool   `json:"insecure"`
	CACert    string `json:"caCert"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (uc *ArcheryUsecase) toView(c *po.ArcheryConnection) ConnectionView {
	pwd, _ := conf.Decrypt(uc.secret, c.PasswordEnc)
	return ConnectionView{
		ID: c.ID, Name: c.Name, Endpoint: c.Endpoint, Instance: c.Instance,
		Username: c.Username, Password: conf.Mask(pwd), Insecure: c.Insecure,
		CACert: c.CACert, Remark: c.Remark,
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
		out = append(out, uc.toView(&list[i]))
	}
	return out, nil
}

func (uc *ArcheryUsecase) GetConnection(ctx context.Context, id int64) (ConnectionView, error) {
	c, err := uc.repo.GetConnection(ctx, id)
	if err != nil {
		return ConnectionView{}, err
	}
	return uc.toView(c), nil
}

// ConnectionInput 创建/更新连接的入参；Password 更新时留空表示不修改。
type ConnectionInput struct {
	Name     string `json:"name" binding:"required"`
	Endpoint string `json:"endpoint" binding:"required"`
	Instance string `json:"instance" binding:"required"`
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
	if strings.TrimSpace(in.Instance) == "" {
		return errors.New("instance 不能为空")
	}
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
		Name: strings.TrimSpace(in.Name), Endpoint: in.Endpoint, Instance: strings.TrimSpace(in.Instance),
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
	c.Instance = strings.TrimSpace(in.Instance)
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

// NewClient 按连接 ID 解密凭据并构造一个 archery HTTP 客户端。
// 供 archery 规则链节点的 ConnectionResolver 注入（见 nodes.SetArcheryClientFactory）。
func (uc *ArcheryUsecase) NewClient(ctx context.Context, id int64) (*archeryclient.Client, error) {
	c, err := uc.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	pwd, err := conf.Decrypt(uc.secret, c.PasswordEnc)
	if err != nil {
		return nil, err
	}
	return archeryclient.New(archeryclient.Config{
		Endpoint: c.Endpoint, Instance: c.Instance,
		Username: c.Username, Password: pwd,
		Insecure: c.Insecure, CACert: c.CACert,
	})
}

// TestConnection 验证连接可用（登录 + 列库），供"测试连接"按钮。
func (uc *ArcheryUsecase) TestConnection(ctx context.Context, id int64) (map[string]any, error) {
	cli, err := uc.NewClient(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := cli.Login(); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	dbs, err := cli.Resource(archeryclient.ResDatabase, "", "", "")
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "instance": cli.Instance(), "databases": dbs}, nil
}
