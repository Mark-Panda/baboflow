package agentkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// BuiltinTools 提供 bash/read/write/edit/grep 五个内置工具，沙箱限定在工作区目录。
type BuiltinTools struct {
	workspaceRoot string          // 工作区根（绝对路径）
	bashAllow     map[string]bool // bash 命令白名单（空 = 黑名单模式）
	bashTimeout   time.Duration
	maxOutput     int
}

func NewBuiltinTools(workspaceRoot string, allowlist []string) *BuiltinTools {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		abs = workspaceRoot
	}
	allow := map[string]bool{}
	for _, c := range allowlist {
		c = strings.TrimSpace(c)
		if c != "" {
			allow[c] = true
		}
	}
	return &BuiltinTools{
		workspaceRoot: abs,
		bashAllow:     allow,
		bashTimeout:   30 * time.Second,
		maxOutput:     100 * 1024,
	}
}

// resolve 把会话相对路径解析到沙箱内绝对路径，越界返回错误。
func (b *BuiltinTools) resolve(sessionID, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	base := filepath.Join(b.workspaceRoot, sessionID)
	// 相对路径基于会话目录；绝对路径若不在沙箱内则拒绝
	var full string
	if filepath.IsAbs(p) {
		full = filepath.Clean(p)
	} else {
		full = filepath.Clean(filepath.Join(base, p))
	}
	root := filepath.Clean(b.workspaceRoot)
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越出工作区沙箱: %s", p)
	}
	// 防符号链接逃逸：解析已存在的路径，确保真实落点仍在沙箱内。root 与目标都要先
	// EvalSymlinks（macOS 上 os.MkdirTemp 返回的 /var/... 是 /private/var 的软链）。
	// write/edit 可能针对尚不存在的新文件，EvalSymlinks 会失败，此时沿用上面的前缀校验即可。
	if resolved, err := filepath.EvalSymlinks(full); err == nil {
		realRoot := root
		if rr, rerr := filepath.EvalSymlinks(root); rerr == nil {
			realRoot = filepath.Clean(rr)
		}
		resolved = filepath.Clean(resolved)
		sep := string(os.PathSeparator)
		if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+sep) &&
			resolved != root && !strings.HasPrefix(resolved, root+sep) {
			return "", fmt.Errorf("路径经符号链接越出工作区沙箱: %s", p)
		}
	}
	return full, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + fmt.Sprintf("\n... [截断, 共 %d 字节]", len(s))
	}
	return s
}

// ---- bash ----

type bashInput struct {
	Command    string `json:"command" jsonschema:"description=要执行的 shell 命令,required"`
	TimeoutSec int    `json:"timeoutSec,omitempty" jsonschema:"description=超时秒数(默认30)"`
}

func (b *BuiltinTools) bashTool(sessionID string) (tool.InvokableTool, error) {
	return utils.InferTool("bash", "在工作区执行 shell 命令, 返回 stdout/stderr/exitCode。危险命令会被拦截。",
		func(ctx context.Context, in bashInput) (string, error) {
			if err := b.checkCommand(in.Command); err != nil {
				return "", err
			}
			dir, err := b.resolve(sessionID, ".")
			if err != nil {
				return "", err
			}
			_ = os.MkdirAll(dir, 0o755)
			timeout := b.bashTimeout
			if in.TimeoutSec > 0 {
				timeout = time.Duration(in.TimeoutSec) * time.Second
			}
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			cmd := exec.CommandContext(cctx, "sh", "-c", in.Command)
			cmd.Dir = dir
			// 沙箱：不向后代进程泄露服务端敏感环境变量（密钥/DSN 等）。
			cmd.Env = sanitizedEnv()
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()
			exitCode := 0
			if runErr != nil {
				if ee, ok := runErr.(*exec.ExitError); ok {
					exitCode = ee.ExitCode()
				} else if cctx.Err() == context.DeadlineExceeded {
					return fmt.Sprintf("命令超时(%v)已终止\nstdout:\n%s\nstderr:\n%s", timeout, truncate(stdout.String(), b.maxOutput), truncate(stderr.String(), b.maxOutput)), nil
				} else {
					return "", fmt.Errorf("执行失败: %w", runErr)
				}
			}
			return fmt.Sprintf("exitCode=%d\nstdout:\n%s\nstderr:\n%s", exitCode, truncate(stdout.String(), b.maxOutput), truncate(stderr.String(), b.maxOutput)), nil
		})
}

// envDenyPrefixes 是不会传给 bash 子进程的环境变量前缀（含密钥/连接串等）。
var envDenyPrefixes = []string{
	"BABO_SECRET", "DATABASE_DSN", "LANGFUSE_SECRET", "LANGFUSE_PUBLIC",
	"ADMIN_INIT_PASSWORD", "API_KEY", "SECRET", "PASSWORD", "TOKEN",
}

// sanitizedEnv 返回剥离了敏感变量后的进程环境，供沙箱内 bash 使用。
func sanitizedEnv() []string {
	out := make([]string, 0, len(os.Environ()))
loop:
	for _, kv := range os.Environ() {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		up := strings.ToUpper(name)
		for _, p := range envDenyPrefixes {
			if strings.HasPrefix(up, p) {
				continue loop
			}
		}
		out = append(out, kv)
	}
	return out
}

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`rm\s+-[a-zA-Z]*r[a-zA-Z]*f`),
	regexp.MustCompile(`rm\s+-[a-zA-Z]*f[a-zA-Z]*r`),
	regexp.MustCompile(`:\(\)\s*\{`),                    // fork 炸弹
	regexp.MustCompile(`mkfs`),                          // 格式化
	regexp.MustCompile(`dd\s+.*of=/dev/`),               // 写盘
	regexp.MustCompile(`>\s*/dev/sd`),                   // 写盘
	regexp.MustCompile(`shutdown|reboot|halt|poweroff`), // 关机
}

func (b *BuiltinTools) checkCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("命令不能为空")
	}
	// 白名单模式：首个词必须在白名单
	if len(b.bashAllow) > 0 {
		first := strings.Fields(cmd)[0]
		first = filepath.Base(first)
		if !b.bashAllow[first] {
			return fmt.Errorf("命令 %q 不在白名单内", first)
		}
		return nil
	}
	// 黑名单模式：拦截危险模式
	for _, re := range dangerousPatterns {
		if re.MatchString(cmd) {
			return fmt.Errorf("命令含危险操作已被拦截: %s", cmd)
		}
	}
	return nil
}

// ---- read ----

type readInput struct {
	Path   string `json:"path" jsonschema:"description=文件路径(相对工作区),required"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=起始行(从0开始)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=最多返回行数(默认全部)"`
}

func (b *BuiltinTools) readTool(sessionID string) (tool.InvokableTool, error) {
	return utils.InferTool("read", "读取文件文本内容(带行号)。大文件可用 offset/limit 分页。",
		func(ctx context.Context, in readInput) (string, error) {
			full, err := b.resolve(sessionID, in.Path)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("读取失败: %w", err)
			}
			// 简单二进制检测
			if isBinary(data) {
				return fmt.Sprintf("(二进制文件, %d 字节, 不支持文本读取)", len(data)), nil
			}
			lines := strings.Split(string(data), "\n")
			start := in.Offset
			if start < 0 {
				start = 0
			}
			if start >= len(lines) {
				return "(空)", nil
			}
			end := len(lines)
			if in.Limit > 0 && start+in.Limit < end {
				end = start + in.Limit
			}
			var sb strings.Builder
			for i := start; i < end; i++ {
				fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i])
			}
			if end < len(lines) {
				fmt.Fprintf(&sb, "... [共 %d 行, 已显示 %d-%d]\n", len(lines), start+1, end)
			}
			return truncate(sb.String(), b.maxOutput), nil
		})
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// ---- write ----

type writeInput struct {
	Path    string `json:"path" jsonschema:"description=文件路径(相对工作区),required"`
	Content string `json:"content" jsonschema:"description=要写入的完整内容,required"`
}

func (b *BuiltinTools) writeTool(sessionID string) (tool.InvokableTool, error) {
	return utils.InferTool("write", "写入(覆盖/新建)文件, 自动创建父目录; 已存在文件先备份为 .bak。",
		func(ctx context.Context, in writeInput) (string, error) {
			full, err := b.resolve(sessionID, in.Path)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return "", err
			}
			// 备份已存在文件
			if _, err := os.Stat(full); err == nil {
				_ = os.Rename(full, full+".bak")
			}
			if err := os.WriteFile(full, []byte(in.Content), 0o644); err != nil {
				return "", fmt.Errorf("写入失败: %w", err)
			}
			return fmt.Sprintf("已写入 %s (%d 字节)", in.Path, len(in.Content)), nil
		})
}

// ---- edit ----

type editInput struct {
	Path      string `json:"path" jsonschema:"description=文件路径(相对工作区),required"`
	OldString string `json:"oldString" jsonschema:"description=要被替换的原始字符串(须唯一匹配),required"`
	NewString string `json:"newString" jsonschema:"description=替换后的新字符串,required"`
}

func (b *BuiltinTools) editTool(sessionID string) (tool.InvokableTool, error) {
	return utils.InferTool("edit", "对文件做精确字符串替换(局部修改)。oldString 须在文件中唯一出现, 否则报错。",
		func(ctx context.Context, in editInput) (string, error) {
			full, err := b.resolve(sessionID, in.Path)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("读取失败: %w", err)
			}
			content := string(data)
			count := strings.Count(content, in.OldString)
			if count == 0 {
				return "", fmt.Errorf("未找到匹配字符串, 未做修改")
			}
			if count > 1 {
				return "", fmt.Errorf("oldString 在文件中出现 %d 次, 不唯一, 请提供更长的上下文", count)
			}
			newContent := strings.Replace(content, in.OldString, in.NewString, 1)
			if err := os.WriteFile(full, []byte(newContent), 0o644); err != nil {
				return "", fmt.Errorf("写入失败: %w", err)
			}
			return fmt.Sprintf("已修改 %s (1 处替换)", in.Path), nil
		})
}

// ---- grep ----

type grepInput struct {
	Pattern    string `json:"pattern" jsonschema:"description=正则表达式,required"`
	Path       string `json:"path,omitempty" jsonschema:"description=搜索目录(默认工作区根)"`
	IgnoreCase bool   `json:"ignoreCase,omitempty" jsonschema:"description=忽略大小写"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"description=最多返回匹配数(默认50)"`
}

func (b *BuiltinTools) grepTool(sessionID string) (tool.InvokableTool, error) {
	return utils.InferTool("grep", "在工作区内按正则搜索文件内容, 返回 file:line:content 匹配列表。",
		func(ctx context.Context, in grepInput) (string, error) {
			pat := in.Pattern
			if in.IgnoreCase {
				pat = "(?i)" + pat
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return "", fmt.Errorf("正则非法: %w", err)
			}
			dir := "."
			if in.Path != "" {
				dir = in.Path
			}
			root, err := b.resolve(sessionID, dir)
			if err != nil {
				return "", err
			}
			max := in.MaxResults
			if max <= 0 {
				max = 50
			}
			var matches []string
			_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || len(matches) >= max {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil || isBinary(data) {
					return nil
				}
				rel, _ := filepath.Rel(root, path)
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, i+1, strings.TrimSpace(line)))
						if len(matches) >= max {
							break
						}
					}
				}
				return nil
			})
			if len(matches) == 0 {
				return "(无匹配)", nil
			}
			return truncate(strings.Join(matches, "\n"), b.maxOutput), nil
		})
}

// Tools 返回该会话的内置工具集（按名称过滤）。
func (b *BuiltinTools) Tools(sessionID string, names []string) ([]tool.BaseTool, error) {
	factories := map[string]func(string) (tool.InvokableTool, error){
		"bash":  b.bashTool,
		"read":  b.readTool,
		"write": b.writeTool,
		"edit":  b.editTool,
		"grep":  b.grepTool,
	}
	use := names
	if len(use) == 0 {
		use = []string{"bash", "read", "write", "edit", "grep"}
	}
	allow := map[string]bool{}
	for _, n := range use {
		allow[n] = true
	}
	var out []tool.BaseTool
	for name, f := range factories {
		if !allow[name] {
			continue
		}
		t, err := f(sessionID)
		if err != nil {
			return nil, fmt.Errorf("构建工具 %s 失败: %w", name, err)
		}
		out = append(out, t)
	}
	return out, nil
}
