package conf

import (
	"os"
	"strconv"
	"strings"
)

// Config 聚合应用配置。全部经环境变量 / .env 注入（main 启动时加载 .env）。
type Config struct {
	HTTPAddr string // HTTP/WS 监听地址
	GRPCAddr string // gRPC 监听地址

	DatabaseDSN string // PostgreSQL DSN

	Secret string // BABO_SECRET: 32 字节，用于 apiKey AES-GCM 加密

	LangfuseHost       string
	LangfusePublicKey  string
	LangfuseSecretKey  string

	Workspace string // BABO_WORKSPACE: Agent 内置工具沙箱目录

	ExecutorWorkers int    // EXECUTOR_WORKERS: 规则链执行并发
	BashAllowlist   []string // BASH_ALLOWLIST: bash 命令白名单(逗号分隔), 空=黑名单模式

	EmbeddingDim     int    // EMBEDDING_DIM: 向量维度
	EmbeddingModelID int64  // EMBEDDING_MODEL_ID: 生成向量所用 llm_model.id

	ComponentSkillLLM bool // COMPONENT_SKILL_LLM: 组件 SKILL 是否用 LLM 润色

	AdminInitPassword string // ADMIN_INIT_PASSWORD: 首次启动 admin 初始密码

	MCPAuthToken string // MCP_AUTH_TOKEN: /mcp SSE 端点的 Bearer 令牌（外部 MCP 客户端用；空=仅接受已登录会话）

	// 飞书自建应用 OAuth 登录（直连 open.feishu.cn）。三者缺一则飞书登录不可用（密码登录不受影响）。
	FeishuAppID       string // FEISHU_APP_ID
	FeishuAppSecret   string // FEISHU_APP_SECRET（仅 env 注入，不入库、不打日志）
	FeishuRedirectURI string // FEISHU_REDIRECT_URI: 与飞书后台配置的重定向 URL 一致，如 https://<host>/api/v1/auth/feishu/callback
}

// Load 从环境变量读取配置（.env 由 main 先加载进环境）。
func Load() *Config {
	return &Config{
		HTTPAddr: getEnv("HTTP_ADDR", ":8000"),
		GRPCAddr: getEnv("GRPC_ADDR", ":9000"),

		DatabaseDSN: getEnv("DATABASE_DSN", "host=127.0.0.1 user=babo password=babo dbname=baboflow port=5432 sslmode=disable"),

		Secret: getEnv("BABO_SECRET", "baboflow-dev-secret-32bytes-pad!"),

		LangfuseHost:      getEnv("LANGFUSE_HOST", ""),
		LangfusePublicKey: getEnv("LANGFUSE_PUBLIC_KEY", ""),
		LangfuseSecretKey: getEnv("LANGFUSE_SECRET_KEY", ""),

		Workspace: getEnv("BABO_WORKSPACE", "./workspace"),

		ExecutorWorkers: getEnvInt("EXECUTOR_WORKERS", 8),
		BashAllowlist:   splitCSV(getEnv("BASH_ALLOWLIST", "")),

		EmbeddingDim:     getEnvInt("EMBEDDING_DIM", 1536),
		EmbeddingModelID: int64(getEnvInt("EMBEDDING_MODEL_ID", 0)),

		ComponentSkillLLM: getEnvBool("COMPONENT_SKILL_LLM", false),

		AdminInitPassword: getEnv("ADMIN_INIT_PASSWORD", "admin123"),

		MCPAuthToken: getEnv("MCP_AUTH_TOKEN", ""),

		FeishuAppID:       getEnv("FEISHU_APP_ID", ""),
		FeishuAppSecret:   getEnv("FEISHU_APP_SECRET", ""),
		FeishuRedirectURI: getEnv("FEISHU_REDIRECT_URI", ""),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	return def
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
