package conf

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 聚合应用配置。
//
// 解析优先级（高 → 低）：
//  1. 环境变量 / .env（main 启动时经 godotenv 注入环境）
//  2. 配置文件 config/default.yaml（可用 BABO_CONFIG 覆盖路径）
//  3. 代码内置默认值
//
// 注意：config/default.yaml 会随仓库提交，请勿写入任何真实密钥；
// 敏感项一律用环境变量 / .env 注入。
type Config struct {
	HTTPAddr string // HTTP/WS 监听地址
	GRPCAddr string // gRPC 监听地址

	DatabaseDSN string // PostgreSQL DSN

	Secret string // BABO_SECRET: 32 字节，用于 apiKey AES-GCM 加密

	LangfuseHost      string
	LangfusePublicKey string
	LangfuseSecretKey string

	MemoryEnabled        bool // MEMORY_ENABLED: 是否启用 Agent 长期记忆
	MemorySessionSummary bool // MEMORY_SESSION_SUMMARY: 是否启用会话摘要
	MemoryEventSearch    bool // MEMORY_EVENT_SEARCH: 是否启用事件检索
	MemoryLimit          int  // MEMORY_LIMIT: 每次检索注入的历史消息数

	Workspace string // BABO_WORKSPACE: Agent 内置工具沙箱目录

	ExecutorWorkers int      // EXECUTOR_WORKERS: 规则链执行并发
	BashAllowlist   []string // BASH_ALLOWLIST: bash 命令白名单(逗号分隔), 空=黑名单模式

	EmbeddingDim     int   // EMBEDDING_DIM: 向量维度
	EmbeddingModelID int64 // EMBEDDING_MODEL_ID: 生成向量所用 llm_model.id

	ComponentSkillLLM bool // COMPONENT_SKILL_LLM: 组件 SKILL 是否用 LLM 润色

	AdminInitPassword string // ADMIN_INIT_PASSWORD: 首次启动 admin 初始密码

	MCPAuthToken string // MCP_AUTH_TOKEN: /mcp SSE 端点的 Bearer 令牌（外部 MCP 客户端用；空=仅接受已登录会话）

	// 飞书自建应用 OAuth 登录（直连 open.feishu.cn）。三者缺一则飞书登录不可用（密码登录不受影响）。
	FeishuAppID       string // FEISHU_APP_ID
	FeishuAppSecret   string // FEISHU_APP_SECRET（仅 env 注入，不入库、不打日志）
	FeishuRedirectURI string // FEISHU_REDIRECT_URI: 与飞书后台配置的重定向 URL 一致，如 https://<host>/api/v1/auth/feishu/callback
}

// fileConfig 与 config/default.yaml 对应的文件配置。仅作为「环境变量缺省时的
// 中间层」；全部字段可空，空则继续回退到代码默认值。
type fileConfig struct {
	HTTPAddr    string `yaml:"httpAddr"`
	GRPCAddr    string `yaml:"grpcAddr"`
	DatabaseDSN string `yaml:"databaseDsn"`

	Secret string `yaml:"secret"`

	LangfuseHost      string `yaml:"langfuseHost"`
	LangfusePublicKey string `yaml:"langfusePublicKey"`
	LangfuseSecretKey string `yaml:"langfuseSecretKey"`

	MemoryEnabled        *bool `yaml:"memoryEnabled"`
	MemorySessionSummary *bool `yaml:"memorySessionSummary"`
	MemoryEventSearch    *bool `yaml:"memoryEventSearch"`
	MemoryLimit          int   `yaml:"memoryLimit"`

	Workspace string `yaml:"workspace"`

	ExecutorWorkers int `yaml:"executorWorkers"`
	// bashAllowlist 支持 YAML 列表（[]string）或逗号分隔字符串两种写法。
	BashAllowlist any `yaml:"bashAllowlist"`

	EmbeddingDim     int   `yaml:"embeddingDim"`
	EmbeddingModelID int64 `yaml:"embeddingModelId"`

	ComponentSkillLLM bool `yaml:"componentSkillLlm"`

	AdminInitPassword string `yaml:"adminInitPassword"`

	MCPAuthToken string `yaml:"mcpAuthToken"`

	FeishuAppID       string `yaml:"feishuAppId"`
	FeishuAppSecret   string `yaml:"feishuAppSecret"`
	FeishuRedirectURI string `yaml:"feishuRedirectUri"`
}

// DefaultConfigPath 默认配置文件路径（相对服务启动目录）。可用 BABO_CONFIG 覆盖。
const DefaultConfigPath = "config/default.yaml"

// loadFileConfig 读取并解析 YAML 配置文件；文件不存在/读取/解析失败时返回零值
// （对应项继续回退到代码默认值）。
func loadFileConfig(path string) fileConfig {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return fc
	}
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fileConfig{}
	}
	return fc
}

// Load 使用默认配置文件装配（等价 LoadWithFile(BABO_CONFIG 或 config/default.yaml)）。
// 环境变量优先，缺省读配置文件，再缺省用代码默认值。
func Load() *Config {
	path := os.Getenv("BABO_CONFIG")
	if path == "" {
		path = DefaultConfigPath
	}
	return LoadWithFile(path)
}

// LoadWithFile 装配配置：环境变量 > 指定 YAML 配置文件 > 代码默认值。
// path 为空或文件不可读时，退化为「环境变量 > 代码默认值」。
func LoadWithFile(path string) *Config {
	var fc fileConfig
	if path != "" {
		fc = loadFileConfig(path)
	}
	return &Config{
		HTTPAddr: pick("HTTP_ADDR", fc.HTTPAddr, ":8000"),
		GRPCAddr: pick("GRPC_ADDR", fc.GRPCAddr, ":9000"),

		DatabaseDSN: pick("DATABASE_DSN", fc.DatabaseDSN, "host=127.0.0.1 user=babo password=babo dbname=baboflow port=5432 sslmode=disable"),

		Secret: pick("BABO_SECRET", fc.Secret, "baboflow-dev-secret-32bytes-pad!"),

		LangfuseHost:      pick("LANGFUSE_HOST", fc.LangfuseHost, ""),
		LangfusePublicKey: pick("LANGFUSE_PUBLIC_KEY", fc.LangfusePublicKey, ""),
		LangfuseSecretKey: pick("LANGFUSE_SECRET_KEY", fc.LangfuseSecretKey, ""),

		MemoryEnabled:        pickBoolPtr("MEMORY_ENABLED", fc.MemoryEnabled, true),
		MemorySessionSummary: pickBoolPtr("MEMORY_SESSION_SUMMARY", fc.MemorySessionSummary, false),
		MemoryEventSearch:    pickBoolPtr("MEMORY_EVENT_SEARCH", fc.MemoryEventSearch, false),
		MemoryLimit:          pickInt("MEMORY_LIMIT", fc.MemoryLimit, 20),

		Workspace: pick("BABO_WORKSPACE", fc.Workspace, "./workspace"),

		ExecutorWorkers: pickInt("EXECUTOR_WORKERS", fc.ExecutorWorkers, 8),
		BashAllowlist:   pickCSV(fc.BashAllowlist),

		EmbeddingDim:     pickInt("EMBEDDING_DIM", fc.EmbeddingDim, 1536),
		EmbeddingModelID: pickInt64("EMBEDDING_MODEL_ID", fc.EmbeddingModelID, 0),

		ComponentSkillLLM: pickBool("COMPONENT_SKILL_LLM", fc.ComponentSkillLLM, false),

		AdminInitPassword: pick("ADMIN_INIT_PASSWORD", fc.AdminInitPassword, "admin123"),

		MCPAuthToken: pick("MCP_AUTH_TOKEN", fc.MCPAuthToken, ""),

		FeishuAppID:       pick("FEISHU_APP_ID", fc.FeishuAppID, ""),
		FeishuAppSecret:   pick("FEISHU_APP_SECRET", fc.FeishuAppSecret, ""),
		FeishuRedirectURI: pick("FEISHU_REDIRECT_URI", fc.FeishuRedirectURI, ""),
	}
}

// pick：环境变量 > 配置文件 > 默认值。
func pick(envKey, fileVal, def string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if fileVal != "" {
		return fileVal
	}
	return def
}

// pickInt：环境变量 > 配置文件 > 默认值。文件/默认值为 0 时视为「未配置」，继续回退。
func pickInt(envKey string, fileVal, def int) int {
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if fileVal != 0 {
		return fileVal
	}
	return def
}

// pickInt64 同 pickInt，针对 int64。
func pickInt64(envKey string, fileVal, def int64) int64 {
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	if fileVal != 0 {
		return fileVal
	}
	return def
}

// pickBool：环境变量 > 配置文件 > 默认值。
func pickBool(envKey string, fileVal, def bool) bool {
	if v := os.Getenv(envKey); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	if fileVal {
		return true
	}
	return def
}

func pickBoolPtr(envKey string, fileVal *bool, def bool) bool {
	if v := os.Getenv(envKey); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	if fileVal != nil {
		return *fileVal
	}
	return def
}

// pickCSV 解析 BASH_ALLOWLIST：环境变量（逗号分隔）优先，否则用文件配置
// （YAML 列表或逗号分隔字符串），都没有则为空（黑名单模式）。
func pickCSV(fileVal any) []string {
	if v := os.Getenv("BASH_ALLOWLIST"); v != "" {
		return splitCSV(v)
	}
	switch t := fileVal.(type) {
	case []string:
		return cleanList(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return cleanList(out)
	case string:
		return splitCSV(t)
	}
	return nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
