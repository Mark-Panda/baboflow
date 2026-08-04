package biz

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// memSkillRepo 内存版 SkillDataRepo（按 name/id 存整行）。
type memSkillRepo struct {
	byName map[string]*po.Skill
	byID   map[int64]*po.Skill
	nextID int64
}

func newMemSkillRepo() *memSkillRepo {
	return &memSkillRepo{byName: map[string]*po.Skill{}, byID: map[int64]*po.Skill{}, nextID: 1}
}

func (m *memSkillRepo) List(ctx context.Context, source, keyword string) ([]po.Skill, error) {
	out := make([]po.Skill, 0, len(m.byID))
	for _, s := range m.byID {
		out = append(out, *s)
	}
	return out, nil
}
func (m *memSkillRepo) GetByID(ctx context.Context, id int64) (*po.Skill, error) {
	if s, ok := m.byID[id]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memSkillRepo) GetByName(ctx context.Context, name string) (*po.Skill, error) {
	if s, ok := m.byName[name]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memSkillRepo) Create(ctx context.Context, s *po.Skill) error {
	s.ID = m.nextID
	m.nextID++
	s.CreatedAt = time.Now()
	m.byID[s.ID] = s
	m.byName[s.Name] = s
	return nil
}
func (m *memSkillRepo) Update(ctx context.Context, s *po.Skill) error {
	m.byID[s.ID] = s
	m.byName[s.Name] = s
	return nil
}
func (m *memSkillRepo) Delete(ctx context.Context, id int64) error {
	if s, ok := m.byID[id]; ok {
		delete(m.byName, s.Name)
		delete(m.byID, id)
	}
	return nil
}

func skillUsecaseForTest(t *testing.T) (*SkillUsecase, *memSkillRepo, string) {
	t.Helper()
	ws := t.TempDir()
	repo := newMemSkillRepo()
	uc := NewSkillUsecase(repo, nil, &conf.Config{Workspace: ws})
	return uc, repo, ws
}

// buildSkillZip 在内存构造一个技能包 zip。entries 为 包内路径→内容。
func buildSkillZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("写 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

const testSkillMD = "---\nname: pdf-tools\ndescription: PDF 处理\n---\n\n# PDF Tools\n用法说明。\n"

func TestUploadPackage_RootSkillMD(t *testing.T) {
	uc, repo, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":            testSkillMD,
		"references/usage.md": "# 用法参考\n",
		"scripts/run.sh":      "#!/bin/sh\necho hi\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "upload")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	if !view.HasFiles {
		t.Fatal("expected HasFiles=true")
	}
	if view.Name != "pdf-tools" {
		t.Fatalf("expected name=pdf-tools, got %q", view.Name)
	}
	// 磁盘已落盘到 workspace/skills/pdf-tools。
	dir := filepath.Join(ws, "skills", "pdf-tools")
	for _, rel := range []string{"SKILL.md", "references/usage.md", "scripts/run.sh"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected file extracted %s: %v", rel, err)
		}
	}
	// DB 行：FilePath 指向目录、Package 非空。
	s := repo.byName["pdf-tools"]
	if s.FilePath != dir {
		t.Fatalf("expected FilePath=%q, got %q", dir, s.FilePath)
	}
	if len(s.Package) == 0 {
		t.Fatal("expected Package stored in DB")
	}
}

func TestUploadPackage_PreservesExistingMetadata(t *testing.T) {
	uc, repo, _ := skillUsecaseForTest(t)
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	existing := &po.Skill{
		ID: 7, Name: "pdf-tools", TenantID: 42, ChainID: "chain-1", CreatedAt: createdAt,
	}
	repo.byID[existing.ID] = existing
	repo.byName[existing.Name] = existing

	z := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD})
	if _, err := uc.UploadPackage(context.Background(), z, ""); err != nil {
		t.Fatal(err)
	}
	got := repo.byID[existing.ID]
	if got.TenantID != 42 || got.ChainID != "chain-1" || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("existing metadata was overwritten: tenant=%d chain=%q created=%v", got.TenantID, got.ChainID, got.CreatedAt)
	}
}

func TestUploadPackage_NestedSkillMD(t *testing.T) {
	uc, repo, ws := skillUsecaseForTest(t)
	// SKILL.md 嵌套一层目录（"多包一层"）。
	z := buildSkillZip(t, map[string]string{
		"pdf-tools/SKILL.md":            testSkillMD,
		"pdf-tools/references/usage.md": "# 用法\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatalf("UploadPackage nested error: %v", err)
	}
	if !view.HasFiles {
		t.Fatal("expected HasFiles=true for nested package")
	}
	dir := filepath.Join(ws, "skills", "pdf-tools")
	// 嵌套的包应以 SKILL.md 所在目录为根，落到 skills/pdf-tools 下（相对根平铺）。
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md at skill root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "references/usage.md")); err != nil {
		t.Fatalf("expected references/usage.md at skill root: %v", err)
	}
	if repo.byName["pdf-tools"].FilePath != dir {
		t.Fatalf("FilePath mismatch: %q", repo.byName["pdf-tools"].FilePath)
	}
}

func TestUploadPackage_RejectsDuplicateEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, content := range []string{testSkillMD, testSkillMD} {
		w, err := zw.Create("SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	uc, _, _ := skillUsecaseForTest(t)
	if _, err := uc.UploadPackage(context.Background(), buf.Bytes(), ""); err == nil ||
		!strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}
}

func TestUploadPackage_MissingSkillMD(t *testing.T) {
	uc, _, _ := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{"README.md": "no skill here"})
	if _, err := uc.UploadPackage(context.Background(), z, ""); err == nil ||
		!strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("expected missing SKILL.md error, got %v", err)
	}
}

func TestUploadPackage_RejectsPathTraversal(t *testing.T) {
	uc, _, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":      testSkillMD,
		"../escape.txt": "evil",
	})
	if _, err := uc.UploadPackage(context.Background(), z, ""); err == nil {
		t.Fatal("expected path traversal rejection")
	}
	// 确保没有文件逃逸到 skills 根之外。
	if _, err := os.Stat(filepath.Join(ws, "escape.txt")); err == nil {
		t.Fatal("escape.txt should not be written outside skill dir")
	}
}

// TestExtractSkillPackage_BlocksTraversal 直接对解压函数验证：即便某条目
// 名字面含 ".."（未经读取器规范化），也绝不落到 destDir 之外。
func TestExtractSkillPackage_BlocksTraversal(t *testing.T) {
	dest := t.TempDir()
	z := buildSkillZip(t, map[string]string{
		"pdf-tools/SKILL.md":   testSkillMD,
		"pdf-tools/../evil.md": "evil", // 规范化后会逃逸出技能根
	})
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		t.Fatal(err)
	}
	// 不报错也罢（条目可能被规范化后落到根外被跳过），关键是 dest 之外无 evil.md。
	_ = extractSkillPackage(zr, "pdf-tools", dest)
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.md")); err == nil {
		t.Fatal("evil.md must not escape destDir")
	}
}

func TestUploadPackage_InvalidZip(t *testing.T) {
	uc, _, _ := skillUsecaseForTest(t)
	if _, err := uc.UploadPackage(context.Background(), []byte("not a zip"), ""); err == nil {
		t.Fatal("expected invalid zip error")
	}
}

func TestSkillListFilesAndReadFile(t *testing.T) {
	uc, _, _ := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":            testSkillMD,
		"references/usage.md": "# 用法参考\n",
		"scripts/run.sh":      "echo hi\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	files, err := uc.ListFiles(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("ListFiles error: %v", err)
	}
	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	for _, want := range []string{"SKILL.md", "references/usage.md", "scripts/run.sh"} {
		if !paths[want] {
			t.Fatalf("expected file %s in list, got %v", want, paths)
		}
	}
	// 读文本文件。
	content, err := uc.ReadFile(context.Background(), view.ID, "references/usage.md")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(content, "用法参考") {
		t.Fatalf("unexpected content: %q", content)
	}
	// 路径逃逸拒绝。
	if _, err := uc.ReadFile(context.Background(), view.ID, "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejection in ReadFile")
	}
	// 不存在文件。
	if _, err := uc.ReadFile(context.Background(), view.ID, "nope.md"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestSkillReadFile_RejectsBinary(t *testing.T) {
	uc, _, _ := skillUsecaseForTest(t)
	// 构造含二进制（NUL 字节）文件的包。
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w1, _ := zw.Create("SKILL.md")
	w1.Write([]byte(testSkillMD))
	w2, _ := zw.Create("bin.dat")
	w2.Write([]byte{0x00, 0x01, 0x02, 0x00})
	zw.Close()
	view, err := uc.UploadPackage(context.Background(), buf.Bytes(), "")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	if _, err := uc.ReadFile(context.Background(), view.ID, "bin.dat"); err == nil ||
		!strings.Contains(err.Error(), "二进制") {
		t.Fatalf("expected binary rejection, got %v", err)
	}
}

func TestSkillEnsureExtracted_LazyRebuild(t *testing.T) {
	uc, repo, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":            testSkillMD,
		"references/usage.md": "# 用法\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	dir := filepath.Join(ws, "skills", "pdf-tools")
	// 模拟卷被清：删掉落盘目录。
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("expected dir removed")
	}
	// EnsureExtracted 应从 DB 归档重建。
	s := repo.byName["pdf-tools"]
	got, err := uc.EnsureExtracted(context.Background(), s)
	if err != nil {
		t.Fatalf("EnsureExtracted error: %v", err)
	}
	if got != dir {
		t.Fatalf("expected dir %q, got %q", dir, got)
	}
	if _, err := os.Stat(filepath.Join(dir, "references/usage.md")); err != nil {
		t.Fatalf("expected references/usage.md rebuilt: %v", err)
	}
	_ = view
}

func TestSkillEnsureExtracted_RebuildsIncompleteDir(t *testing.T) {
	uc, _, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":            testSkillMD,
		"references/usage.md": "# 用法\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws, "skills", "pdf-tools")
	if err := os.Remove(filepath.Join(dir, ".baboflow.complete")); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.EnsureExtracted(context.Background(), &po.Skill{
		Name: view.Name, HasFiles: true, FilePath: dir, Package: z,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".baboflow.complete")); err != nil {
		t.Fatalf("expected incomplete directory to be rebuilt: %v", err)
	}
}

func TestSkillEnsureExtracted_RebuildsTamperedFile(t *testing.T) {
	uc, _, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{
		"SKILL.md":            testSkillMD,
		"references/usage.md": "# 原始内容\n",
	})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(ws, "skills", "pdf-tools")
	target := filepath.Join(dir, "references", "usage.md")
	if err := os.WriteFile(target, []byte("被篡改\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.EnsureExtracted(context.Background(), &po.Skill{
		Name: view.Name, HasFiles: true, FilePath: dir, Package: z,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# 原始内容\n" {
		t.Fatalf("expected tampered file to be rebuilt, got %q", got)
	}
}

func TestSkillEnsureExtractedPropagatesMetadataUpdateError(t *testing.T) {
	uc, repo, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(ws, "skills", "pdf-tools")); err != nil {
		t.Fatal(err)
	}
	failing := *repo
	uc.repo = &failingSkillRepo{memSkillRepo: &failing, err: gorm.ErrInvalidData}
	_, err = uc.EnsureExtracted(context.Background(), &po.Skill{
		ID: view.ID, Name: view.Name, HasFiles: true, FilePath: "", Package: z,
	})
	if err == nil {
		t.Fatal("expected metadata update error")
	}
}

func TestSkillDelete_RemovesPackageDir(t *testing.T) {
	uc, _, ws := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	dir := filepath.Join(ws, "skills", "pdf-tools")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected dir exists: %v", err)
	}
	if err := uc.Delete(context.Background(), view.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("expected package dir removed after Delete")
	}
}

func TestSkillDownloadPackage(t *testing.T) {
	uc, _, _ := skillUsecaseForTest(t)
	z := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD})
	view, err := uc.UploadPackage(context.Background(), z, "")
	if err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	name, data, err := uc.DownloadPackage(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("DownloadPackage error: %v", err)
	}
	if name != "pdf-tools.zip" {
		t.Fatalf("expected pdf-tools.zip, got %q", name)
	}
	if !bytes.Equal(data, z) {
		t.Fatal("expected download bytes == uploaded zip")
	}
}

func TestReadZipEntryRejectsActualOversize(t *testing.T) {
	z := buildSkillZip(t, map[string]string{"large.txt": "1234567890"})
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		t.Fatal(err)
	}
	// 模拟恶意 ZIP：声明的解压大小小于实际内容。
	zr.File[0].UncompressedSize64 = 1
	if _, err := readZipEntry(zr.File[0], 4); err == nil {
		t.Fatal("expected actual content size limit error")
	}
}

func TestUploadPackage_DBFailureKeepsExistingPackageDir(t *testing.T) {
	uc, repo, ws := skillUsecaseForTest(t)
	oldZip := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD, "old.txt": "old"})
	if _, err := uc.UploadPackage(context.Background(), oldZip, ""); err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(ws, "skills", "pdf-tools")
	oldContent, err := os.ReadFile(filepath.Join(oldDir, "old.txt"))
	if err != nil {
		t.Fatal(err)
	}

	failing := *repo
	failing.byName = repo.byName
	failing.byID = repo.byID
	// 通过替换 Update 行为模拟数据库更新失败。
	uc.repo = &failingSkillRepo{memSkillRepo: &failing, err: gorm.ErrInvalidData}
	newZip := buildSkillZip(t, map[string]string{"SKILL.md": testSkillMD, "new.txt": "new"})
	if _, err := uc.UploadPackage(context.Background(), newZip, ""); err == nil {
		t.Fatal("expected database update error")
	}
	got, err := os.ReadFile(filepath.Join(oldDir, "old.txt"))
	if err != nil {
		t.Fatalf("old package directory should remain: %v", err)
	}
	if !bytes.Equal(got, oldContent) {
		t.Fatalf("old package content changed: %q", got)
	}
}

type failingSkillRepo struct {
	*memSkillRepo
	err error
}

func (r *failingSkillRepo) Update(ctx context.Context, s *po.Skill) error {
	return r.err
}
