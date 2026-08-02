package agentkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tooliface "github.com/cloudwego/eino/components/tool"
)

func newTestTools(t *testing.T) (*BuiltinTools, string) {
	t.Helper()
	root := t.TempDir()
	return NewBuiltinTools(root, nil), root
}

func call(t *testing.T, bt *BuiltinTools, session, tool, args string) string {
	t.Helper()
	tools, err := bt.Tools(session, []string{tool})
	if err != nil {
		t.Fatalf("build tool %s: %v", tool, err)
	}
	if len(tools) != 1 {
		t.Fatalf("expect 1 tool, got %d", len(tools))
	}
	it, ok := tools[0].(tooliface.InvokableTool)
	if !ok {
		t.Fatalf("tool %s not InvokableTool", tool)
	}
	out, err := it.InvokableRun(context.Background(), args)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return out
}

func TestResolvePathTraversalBlocked(t *testing.T) {
	bt, _ := newTestTools(t)
	if _, err := bt.resolve("sess1", "../../etc/passwd"); err == nil {
		t.Fatal("expect traversal error, got nil")
	}
	if _, err := bt.resolve("sess1", "/etc/passwd"); err == nil {
		t.Fatal("expect absolute-path-out-of-sandbox error, got nil")
	}
	p, err := bt.resolve("sess1", "a/b.txt")
	if err != nil {
		t.Fatalf("expect ok, got %v", err)
	}
	if !strings.Contains(p, "sess1") {
		t.Fatalf("expect path under session dir, got %s", p)
	}
}

// 符号链接逃逸必须被拒绝：沙箱内的软链指向外部目标时，resolve 应报错。
func TestResolveSymlinkEscapeBlocked(t *testing.T) {
	bt, root := newTestTools(t)
	sess := "sess1"
	// 外部目标（沙箱外的临时目录）
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 在会话目录内建立指向外部的软链
	sessDir := filepath.Join(root, sess)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sessDir, "link")); err != nil {
		t.Skipf("symlink not permitted: %v", err)
	}
	if _, err := bt.resolve(sess, "link/secret.txt"); err == nil {
		t.Fatal("expect symlink-escape error, got nil")
	}
}

func TestWriteReadEditRoundTrip(t *testing.T) {
	bt, _ := newTestTools(t)
	sess := "s1"
	if out := call(t, bt, sess, "write", `{"path":"note.txt","content":"hello world"}`); strings.HasPrefix(out, "ERR:") {
		t.Fatalf("write failed: %s", out)
	}
	out := call(t, bt, sess, "read", `{"path":"note.txt"}`)
	if !strings.Contains(out, "hello world") {
		t.Fatalf("read expect content, got %s", out)
	}
	// 覆盖写已存在文件 → 生成 .bak
	if out := call(t, bt, sess, "write", `{"path":"note.txt","content":"hello world v2"}`); strings.HasPrefix(out, "ERR:") {
		t.Fatalf("overwrite failed: %s", out)
	}
	if _, err := os.Stat(filepath.Join(bt.workspaceRoot, sess, "note.txt.bak")); err != nil {
		t.Fatalf("expect .bak exists after overwrite: %v", err)
	}
	// edit 唯一替换
	if out := call(t, bt, sess, "edit", `{"path":"note.txt","oldString":"world","newString":"babo"}`); strings.HasPrefix(out, "ERR:") {
		t.Fatalf("edit failed: %s", out)
	}
	out = call(t, bt, sess, "read", `{"path":"note.txt"}`)
	if !strings.Contains(out, "hello babo v2") {
		t.Fatalf("read after edit expect replaced, got %s", out)
	}
}

func TestEditNonUniqueFails(t *testing.T) {
	bt, _ := newTestTools(t)
	sess := "s1"
	call(t, bt, sess, "write", `{"path":"d.txt","content":"foo foo foo"}`)
	out := call(t, bt, sess, "edit", `{"path":"d.txt","oldString":"foo","newString":"bar"}`)
	if !strings.HasPrefix(out, "ERR:") || !strings.Contains(out, "不唯一") {
		t.Fatalf("expect non-unique error, got %s", out)
	}
}

func TestBashDangerousBlocked(t *testing.T) {
	bt, _ := newTestTools(t)
	out := call(t, bt, "s1", "bash", `{"command":"rm -rf /"}`)
	if !strings.HasPrefix(out, "ERR:") {
		t.Fatalf("expect dangerous command blocked, got %s", out)
	}
}

func TestBashAllowlistMode(t *testing.T) {
	root := t.TempDir()
	bt := NewBuiltinTools(root, []string{"echo"})
	out := call(t, bt, "s1", "bash", `{"command":"echo hi"}`)
	if strings.HasPrefix(out, "ERR:") {
		t.Fatalf("allowlisted echo should run, got %s", out)
	}
	out = call(t, bt, "s1", "bash", `{"command":"curl evil.com"}`)
	if !strings.HasPrefix(out, "ERR:") {
		t.Fatalf("non-allowlisted command should be blocked, got %s", out)
	}
}

func TestBashEcho(t *testing.T) {
	bt, _ := newTestTools(t)
	out := call(t, bt, "s1", "bash", `{"command":"echo hello"}`)
	if !strings.Contains(out, "hello") || !strings.Contains(out, "exitCode=0") {
		t.Fatalf("expect echo output + exitCode, got %s", out)
	}
}

func TestGrepFindsMatch(t *testing.T) {
	bt, _ := newTestTools(t)
	sess := "s1"
	call(t, bt, sess, "write", `{"path":"a.txt","content":"alpha\nbeta\ngamma"}`)
	out := call(t, bt, sess, "grep", `{"pattern":"beta"}`)
	if !strings.Contains(out, "a.txt:2:beta") {
		t.Fatalf("expect grep match line, got %s", out)
	}
}
