package conf

import (
	"os"
	"path/filepath"
	"testing"
)

// 写一份临时 YAML 配置并返回其路径。
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "default.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return p
}

func TestLoadWithFile_FileValueUsed(t *testing.T) {
	// 环境变量未设置 → 取配置文件值。
	p := writeTempConfig(t, `
httpAddr: ":18000"
executorWorkers: 32
feishuAppId: "cli_from_file"
`)
	cfg := LoadWithFile(p)
	if cfg.HTTPAddr != ":18000" {
		t.Fatalf("expected httpAddr from file, got %q", cfg.HTTPAddr)
	}
	if cfg.ExecutorWorkers != 32 {
		t.Fatalf("expected executorWorkers=32, got %d", cfg.ExecutorWorkers)
	}
	if cfg.FeishuAppID != "cli_from_file" {
		t.Fatalf("expected feishuAppId from file, got %q", cfg.FeishuAppID)
	}
}

func TestLoadWithFile_EnvOverridesFile(t *testing.T) {
	// 环境变量优先于配置文件。
	p := writeTempConfig(t, `httpAddr: ":18000"`)
	t.Setenv("HTTP_ADDR", ":29999")
	cfg := LoadWithFile(p)
	if cfg.HTTPAddr != ":29999" {
		t.Fatalf("expected env to override file, got %q", cfg.HTTPAddr)
	}
}

func TestLoadWithFile_FallsBackToDefault(t *testing.T) {
	// 环境变量与配置文件都没有 → 用代码默认值。
	p := writeTempConfig(t, `httpAddr: ":18000"`)
	cfg := LoadWithFile(p)
	if cfg.GRPCAddr != ":9000" {
		t.Fatalf("expected default grpcAddr :9000, got %q", cfg.GRPCAddr)
	}
	if cfg.EmbeddingDim != 1536 {
		t.Fatalf("expected default embeddingDim 1536, got %d", cfg.EmbeddingDim)
	}
	if cfg.AdminInitPassword != "admin123" {
		t.Fatalf("expected default adminInitPassword, got %q", cfg.AdminInitPassword)
	}
}

func TestLoadWithFile_MissingFile_UsesDefaults(t *testing.T) {
	// 文件不存在 → 全部回退默认值。
	cfg := LoadWithFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if cfg.HTTPAddr != ":8000" || cfg.DatabaseDSN == "" {
		t.Fatalf("expected defaults when file missing, got %+v", cfg)
	}
}

func TestLoadWithFile_BashAllowlistForms(t *testing.T) {
	// YAML 列表形式。
	pl := writeTempConfig(t, "bashAllowlist:\n  - ls\n  - cat\n")
	if got := LoadWithFile(pl).BashAllowlist; len(got) != 2 || got[0] != "ls" || got[1] != "cat" {
		t.Fatalf("list form wrong: %#v", got)
	}
	// 逗号分隔字符串形式。
	ps := writeTempConfig(t, `bashAllowlist: "ls, cat , grep"`)
	if got := LoadWithFile(ps).BashAllowlist; len(got) != 3 || got[2] != "grep" {
		t.Fatalf("string form wrong: %#v", got)
	}
	// 环境变量优先。
	t.Setenv("BASH_ALLOWLIST", "onlyenv")
	if got := LoadWithFile(pl).BashAllowlist; len(got) != 1 || got[0] != "onlyenv" {
		t.Fatalf("env should override file: %#v", got)
	}
}

func TestLoad_DefaultPathEnvOverride(t *testing.T) {
	// BABO_CONFIG 指向自定义文件时，Load() 读它。
	p := writeTempConfig(t, `httpAddr: ":17000"`)
	t.Setenv("BABO_CONFIG", p)
	if got := Load().HTTPAddr; got != ":17000" {
		t.Fatalf("expected BABO_CONFIG path to be honored, got %q", got)
	}
}

func TestLoad_MemoryConfig(t *testing.T) {
	cfg := LoadWithFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if !cfg.MemoryEnabled || cfg.MemorySessionSummary || cfg.MemoryEventSearch || cfg.MemoryLimit != 20 {
		t.Fatalf("unexpected memory defaults: %+v", cfg)
	}

	t.Setenv("MEMORY_ENABLED", "false")
	t.Setenv("MEMORY_SESSION_SUMMARY", "false")
	t.Setenv("MEMORY_EVENT_SEARCH", "true")
	t.Setenv("MEMORY_LIMIT", "8")
	cfg = LoadWithFile("")
	if cfg.MemoryEnabled || cfg.MemorySessionSummary || !cfg.MemoryEventSearch || cfg.MemoryLimit != 8 {
		t.Fatalf("environment variables should override memory config: %+v", cfg)
	}

	p := writeTempConfig(t, "memoryEnabled: false\nmemorySessionSummary: false\nmemoryEventSearch: true\n")
	t.Setenv("MEMORY_ENABLED", "")
	t.Setenv("MEMORY_SESSION_SUMMARY", "")
	t.Setenv("MEMORY_EVENT_SEARCH", "")
	cfg = LoadWithFile(p)
	if cfg.MemoryEnabled || cfg.MemorySessionSummary || !cfg.MemoryEventSearch {
		t.Fatalf("explicit false values from file should be honored: %+v", cfg)
	}
}
