package po

import (
	"time"

	"github.com/pgvector/pgvector-go"
	"gorm.io/datatypes"
)

// 说明：字段标签尽量与设计文档 backend-design.md 对齐。
// jsonb 统一用 datatypes.JSON；向量用 pgvector.Vector；多租户预留 TenantID。

type AdminUser struct {
	ID           int64      `gorm:"primaryKey"`
	Username     string     `gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string     `gorm:"size:255;not null"`
	DisplayName  string     `gorm:"size:64;not null;default:管理员"`
	MustChangePwd bool      `gorm:"not null;default:false"` // 首次登录强制改密
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (AdminUser) TableName() string { return "admin_user" }

type Session struct {
	ID        string `gorm:"primaryKey;size:64"`
	UserID    int64  `gorm:"index;not null"`
	IP        string `gorm:"size:64;not null;default:''"`
	UserAgent string `gorm:"size:255;not null;default:''"`
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Session) TableName() string { return "session" }

type LLMProvider struct {
	ID        int64 `gorm:"primaryKey"`
	TenantID  int64 `gorm:"index;not null;default:0"`
	Name      string `gorm:"size:64;not null"`
	Provider  string `gorm:"size:32;not null;default:openai"`
	BaseURL   string `gorm:"size:255;not null"`
	APIKeyEnc string `gorm:"type:text;not null"`
	Extra     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Remark    string `gorm:"size:255;not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Models    []LLMModel `gorm:"foreignKey:ProviderID"`
}

func (LLMProvider) TableName() string { return "llm_provider" }

type LLMModel struct {
	ID          int64 `gorm:"primaryKey" json:"id"`
	TenantID    int64 `gorm:"index;not null;default:0" json:"tenantId"`
	ProviderID  int64 `gorm:"index;not null" json:"providerId"`
	Model       string `gorm:"size:128;not null" json:"model"`
	Alias       string `gorm:"size:64;not null;default:''" json:"alias"`
	Temperature float64 `gorm:"type:numeric(3,2);not null;default:0.7" json:"temperature"`
	MaxTokens   int    `gorm:"not null;default:4096" json:"maxTokens"`
	IsDefault   bool   `gorm:"not null;default:false" json:"isDefault"`
	Capability  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"capability"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (LLMModel) TableName() string { return "llm_model" }

type ComponentMeta struct {
	ID          int64 `gorm:"primaryKey"`
	Type        string `gorm:"size:128;uniqueIndex;not null"`
	Name        string `gorm:"size:128;not null"`
	Category    string `gorm:"size:32;index;not null"`
	Description string `gorm:"type:text;not null;default:''"`
	ConfigSchema datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Example     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Fingerprint string `gorm:"size:64;not null;default:''"`
	Embedding   *pgvector.Vector `gorm:"type:vector"` // 可空：未配置 embedding 时为 NULL
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ComponentMeta) TableName() string { return "component_meta" }

type Skill struct {
	ID          int64 `gorm:"primaryKey"`
	TenantID    int64 `gorm:"index;not null;default:0"`
	Name        string `gorm:"size:128;uniqueIndex;not null"`
	Description string `gorm:"size:512;not null;default:''"`
	Source      string `gorm:"size:16;not null;default:upload"` // upload/chain/builtin/component
	ChainID     string `gorm:"size:64;not null;default:''"`
	Frontmatter datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Content     string `gorm:"type:text;not null"`
	FilePath    string `gorm:"size:255;not null;default:''"`
	Embedding   *pgvector.Vector `gorm:"type:vector"` // 可空
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Skill) TableName() string { return "skill" }

type Agent struct {
	ID            int64 `gorm:"primaryKey"`
	TenantID      int64 `gorm:"index;not null;default:0"`
	Key           string `gorm:"size:64;uniqueIndex;not null"`
	Name          string `gorm:"size:128;not null"`
	Instruction   string `gorm:"type:text;not null;default:''"`
	LLMModelID    *int64
	MemoryBackend string `gorm:"size:16;not null;default:builtin"`
	SkillIDs      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	McpIDs        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	BuiltinTools  datatypes.JSON `gorm:"type:jsonb;not null;default:'[\"bash\",\"read\",\"write\",\"edit\",\"grep\"]'"`
	IsBuiltin     bool   `gorm:"not null;default:false"`
	Enabled       bool   `gorm:"not null;default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Agent) TableName() string { return "agent" }

type AgentSubAgent struct {
	ID          int64 `gorm:"primaryKey"`
	ParentID    int64 `gorm:"uniqueIndex:idx_parent_child;not null"`
	ChildID     int64 `gorm:"uniqueIndex:idx_parent_child;not null"`
	Description string `gorm:"size:512;not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AgentSubAgent) TableName() string { return "agent_sub_agent" }

// ---- Agent 会话 / 消息 / 附件（M5）----

type AgentSession struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"` // uuid
	TenantID  int64  `gorm:"index;not null;default:0" json:"tenantId"`
	AgentKey  string `gorm:"size:64;index;not null" json:"agentKey"`
	UserID    *int64 `gorm:"index" json:"userId,omitempty"`
	Title     string `gorm:"size:255;not null;default:''" json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (AgentSession) TableName() string { return "agent_session" }

type AgentMessage struct {
	ID         int64  `gorm:"primaryKey" json:"id"`
	SessionID  string `gorm:"size:64;index;not null" json:"sessionId"`
	Role       string `gorm:"size:16;index;not null" json:"role"` // user/assistant/tool/system
	Content    string `gorm:"type:text;not null;default:''" json:"content"`
	ToolCalls  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"toolCalls"`  // [{name,input,output,status}]
	Attachment datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"attachment"` // [{assetId,name,mime,url}]
	SubAgent   string `gorm:"size:64;not null;default:''" json:"subAgent,omitempty"` // 由哪个 subAgent 产出
	TraceID    string `gorm:"size:64;not null;default:''" json:"traceId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (AgentMessage) TableName() string { return "agent_message" }

type Asset struct {
	ID        int64  `gorm:"primaryKey" json:"id"`
	TenantID  int64  `gorm:"index;not null;default:0" json:"tenantId"`
	Name      string `gorm:"size:255;not null" json:"name"`
	Mime      string `gorm:"size:128;not null;default:''" json:"mime"`
	Size      int64  `gorm:"not null;default:0" json:"size"`
	Path      string `gorm:"size:512;not null" json:"path"` // 本地存储相对路径
	SessionID string `gorm:"size:64;index;not null;default:''" json:"sessionId,omitempty"`
	CreatedBy *int64 `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func (Asset) TableName() string { return "asset" }


type AuditLog struct {
	ID         int64          `gorm:"primaryKey" json:"id"`
	TenantID   int64          `gorm:"index;not null;default:0" json:"tenantId"`
	UserID     *int64         `json:"userId,omitempty"`
	Action     string         `gorm:"size:64;index;not null" json:"action"`
	TargetType string         `gorm:"size:32;not null" json:"targetType"`
	TargetID   string         `gorm:"size:64;not null;default:''" json:"targetId"`
	Detail     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"detail"`
	IP         string         `gorm:"size:64;not null;default:''" json:"ip"`
	CreatedAt  time.Time      `gorm:"index" json:"createdAt"`
}

func (AuditLog) TableName() string { return "audit_log" }

// ---- 规则链（M2）----

type RuleChain struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	TenantID    int64          `gorm:"index;not null;default:0" json:"tenantId"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Description string         `gorm:"size:512;not null;default:''" json:"description"`
	// InputSchema 规则链入参 JSON Schema（可选），供 MCP 暴露/SKILL 生成向调用方说明如何传参。
	InputSchema datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"inputSchema"`
	DSL         datatypes.JSON `gorm:"type:jsonb;not null" json:"dsl"`
	Status      string         `gorm:"size:16;index;not null;default:draft" json:"status"` // draft/published/archived
	Version     int            `gorm:"not null;default:0" json:"version"`
	DebugMode   bool           `gorm:"not null;default:false" json:"debugMode"`
	Source      string         `gorm:"size:16;not null;default:manual" json:"source"` // manual/agent
	CreatedBy   *int64         `json:"createdBy,omitempty"`
	DeletedAt   *time.Time     `gorm:"index" json:"-"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func (RuleChain) TableName() string { return "rule_chain" }

type RuleChainVersion struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	ChainID     string         `gorm:"size:64;uniqueIndex:idx_chain_version;not null" json:"chainId"`
	Version     int            `gorm:"uniqueIndex:idx_chain_version;not null" json:"version"`
	DSL         datatypes.JSON `gorm:"type:jsonb;not null" json:"dsl"`
	PublishedBy *int64         `json:"publishedBy,omitempty"`
	PublishedAt time.Time      `gorm:"not null;default:now()" json:"publishedAt"`
}

func (RuleChainVersion) TableName() string { return "rule_chain_version" }

type ChainRun struct {
	ID         int64          `gorm:"primaryKey" json:"id"`
	TenantID   int64          `gorm:"index;not null;default:0" json:"tenantId"`
	ChainID    string         `gorm:"size:64;index;not null" json:"chainId"`
	TaskID     *int64         `json:"taskId,omitempty"`
	Trigger    string         `gorm:"size:16;not null;default:manual" json:"trigger"` // manual/task/mcp/cron
	Status     string         `gorm:"size:16;index;not null;default:running" json:"status"` // running/success/failure/timeout
	Input      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"input"`
	Output     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"output"`
	Error      string         `gorm:"type:text;not null;default:''" json:"error"`
	NodeTrace  datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"nodeTrace"`
	StartedAt  time.Time      `gorm:"not null;default:now()" json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

func (ChainRun) TableName() string { return "chain_run" }

// ---- MCP（M6）----

type McpServer struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	TenantID    int64          `gorm:"index;not null;default:0" json:"tenantId"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Transport   string         `gorm:"size:16;not null;default:sse" json:"transport"` // stdio/sse/streamable-http
	Endpoint    string         `gorm:"size:255;not null;default:''" json:"endpoint"`  // sse/http 地址
	Command     string         `gorm:"size:255;not null;default:''" json:"command"`   // stdio 启动命令
	Args        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"args"`
	Env         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"env"`
	Status      string         `gorm:"size:16;not null;default:disabled" json:"status"` // enabled/disabled/error
	LastCheckAt *time.Time     `json:"lastCheckAt,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func (McpServer) TableName() string { return "mcp_server" }

type McpExposure struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	TenantID    int64          `gorm:"index;not null;default:0" json:"tenantId"`
	ChainID     string         `gorm:"size:64;index;not null" json:"chainId"`
	ToolName    string         `gorm:"size:128;uniqueIndex;not null" json:"toolName"`
	Description string         `gorm:"size:512;not null;default:''" json:"description"`
	InputSchema datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"inputSchema"`
	Enabled     bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func (McpExposure) TableName() string { return "mcp_exposure" }

// ---- 看板（M6）----

type Board struct {
	ID          int64      `gorm:"primaryKey" json:"id"`
	TenantID    int64      `gorm:"index;not null;default:0" json:"tenantId"`
	Name        string     `gorm:"size:128;not null" json:"name"`
	Description string     `gorm:"size:512;not null;default:''" json:"description"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (Board) TableName() string { return "board" }

type BoardColumn struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	BoardID   int64     `gorm:"index;not null" json:"boardId"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Sort      int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (BoardColumn) TableName() string { return "board_column" }

type Task struct {
	ID              int64          `gorm:"primaryKey" json:"id"`
	TenantID        int64          `gorm:"index;not null;default:0" json:"tenantId"`
	ColumnID        int64          `gorm:"index;not null" json:"columnId"`
	Title           string         `gorm:"size:255;not null" json:"title"`
	Payload         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	Status          string         `gorm:"size:16;not null;default:pending" json:"status"` // pending/running/success/failure
	AssignedChainID string         `gorm:"size:64;not null;default:''" json:"assignedChainId"`
	RunID           *int64         `json:"runId,omitempty"`
	Result          datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"result"`
	RetryMax        int            `gorm:"not null;default:0" json:"retryMax"`
	RetryCount      int            `gorm:"not null;default:0" json:"retryCount"`
	TimeoutSec      int            `gorm:"not null;default:300" json:"timeoutSec"`
	Sort            int            `gorm:"not null;default:0" json:"sort"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

func (Task) TableName() string { return "task" }

// ---- Cron 定时任务（需求1, M7）----

type CronJob struct {
	ID           int64          `gorm:"primaryKey" json:"id"`
	TenantID     int64          `gorm:"index;not null;default:0" json:"tenantId"`
	Name         string         `gorm:"size:128;not null;default:''" json:"name"`
	TargetType   string         `gorm:"size:16;not null" json:"targetType"`   // chain / agent
	TargetID     string         `gorm:"size:64;not null" json:"targetId"`     // rule_chain.id 或 agent.key
	ScheduleType string         `gorm:"size:16;not null;default:cron" json:"scheduleType"` // once/interval/cron
	CronExpr     string         `gorm:"size:64;not null;default:''" json:"cronExpr"`
	IntervalSec  int            `gorm:"not null;default:0" json:"intervalSec"`
	RunAt        *time.Time     `json:"runAt,omitempty"`
	Payload      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	Enabled      bool           `gorm:"not null;default:true" json:"enabled"`
	LastRunAt    *time.Time     `json:"lastRunAt,omitempty"`
	LastStatus   string         `gorm:"size:16;not null;default:''" json:"lastStatus"` // success/failure
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

func (CronJob) TableName() string { return "cron_job" }

// ---- Archery 连接（archery 节点凭据，密码 AES-GCM 加密，同 LLMProvider.APIKeyEnc）----

// ArcheryConnection 描述一个 Archery（hhyo/Archery）平台站点的连接：只含地址+登录凭据，
// 密码密文存库（conf.Encrypt），接口回显一律脱敏，明文永不进 DSL/日志。
// 其下可查询的实例见 ArcheryInstance（点「更新实例」时从 /group/user_all_instances/ 拉取）。
type ArcheryConnection struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	TenantID    int64     `gorm:"index;not null;default:0" json:"tenantId"`
	Name        string    `gorm:"size:128;uniqueIndex;not null" json:"name"`   // 便于人工识别的唯一名
	Endpoint    string    `gorm:"size:255;not null" json:"endpoint"`           // 如 https://archery.example.com
	Username    string    `gorm:"size:128;not null" json:"username"`           // 登录用户名
	PasswordEnc string    `gorm:"size:512;not null;default:''" json:"-"`       // 密码密文（不输出）
	Insecure    bool      `gorm:"not null;default:false" json:"insecure"`      // 跳过 TLS 校验（不安全）
	CACert      string    `gorm:"type:text;not null;default:''" json:"caCert"` // 额外信任的 CA（PEM 文本，可空）
	Remark      string    `gorm:"size:512;not null;default:''" json:"remark"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (ArcheryConnection) TableName() string { return "archery_connection" }

// ArcheryInstance 是某 Archery 连接（站点）下一个可查询实例。规则链节点按其实例 ID 引用；
// 由「更新实例」从 Archery 拉取（upsert），instance_name 即节点查询时的 instance_name。
type ArcheryInstance struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	ConnectionID int64     `gorm:"index;not null" json:"connectionId"`                                  // 所属 archery_connection
	InstanceName string    `gorm:"size:128;not null;uniqueIndex:uniq_conn_instance" json:"instanceName"` // Archery 实例名
	DBType       string    `gorm:"size:32;not null;default:''" json:"dbType"`                            // mysql/postgresql/...
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (ArcheryInstance) TableName() string { return "archery_instance" }
