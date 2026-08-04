package biz

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gorm.io/datatypes"

	"baboflow/internal/biz/agentkit"
	"baboflow/internal/data/po"
)

// 技能包（ZIP）限制，防 zip bomb / 资源耗尽。
const (
	skillPkgMaxBytes     = 20 << 20 // 上传包体上限 20MB（service 亦校验）
	skillPkgMaxEntries   = 512      // 包内条目上限
	skillPkgMaxFileBytes = 8 << 20  // 单文件上限 8MB
	skillPkgMaxTotal     = 64 << 20 // 解压后总大小上限 64MB
	skillPackageComplete = ".baboflow.complete"
)

// SkillFileItem 技能包内一个条目（相对技能根的路径）。
type SkillFileItem struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"isDir"`
}

// skillsBaseDir 技能包落盘根目录：<workspaceRoot>/skills。
func (uc *SkillUsecase) skillsBaseDir() (string, error) {
	if strings.TrimSpace(uc.workspaceRoot) == "" {
		return "", errors.New("未配置工作区（BABO_WORKSPACE），无法落盘技能包")
	}
	abs, err := filepath.Abs(uc.workspaceRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, "skills"), nil
}

// pkgDir 某技能的落盘目录：<workspaceRoot>/skills/<name>。name 经 Clean 防逃逸。
func (uc *SkillUsecase) pkgDir(name string) (string, error) {
	base, err := uc.skillsBaseDir()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.ContainsRune(clean, os.PathSeparator) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("非法技能名 %q", name)
	}
	return filepath.Join(base, clean), nil
}

// removePackageDir 删除技能包落盘目录（幂等，忽略错误）。
func (uc *SkillUsecase) removePackageDir(name string) {
	if dir, err := uc.pkgDir(name); err == nil && dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// UploadPackage 上传技能包（ZIP 多文件）：递归定位 SKILL.md → 解析 frontmatter →
// 安全解压落盘到 workspace/skills/<name>/ → 落库（zip 归档为权威源，随 DB 持久化）。
// 幂等：同名覆盖。
func (uc *SkillUsecase) UploadPackage(ctx context.Context, zipBytes []byte, source string) (*SkillView, error) {
	uc.packageMu.Lock()
	defer uc.packageMu.Unlock()

	if len(zipBytes) == 0 {
		return nil, errors.New("空文件")
	}
	if len(zipBytes) > skillPkgMaxBytes {
		return nil, fmt.Errorf("技能包超过大小上限 %dMB", skillPkgMaxBytes>>20)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("非法 ZIP 包: %w", err)
	}
	if len(zr.File) > skillPkgMaxEntries {
		return nil, fmt.Errorf("技能包条目超过上限 %d", skillPkgMaxEntries)
	}

	// 先全量校验：任何条目名含 ".." 段一律拒包（zip slip 防御，宁可拒包也不静默改写路径）。
	// 必须与下方 SKILL.md 定位分开遍历——后者找到即 break，会漏检排在其后的恶意条目。
	seen := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		normalized := strings.ReplaceAll(f.Name, "\\", "/")
		for _, seg := range strings.Split(normalized, "/") {
			if seg == ".." {
				return nil, fmt.Errorf("技能包含非法路径 %q", f.Name)
			}
		}
		normalized = path.Clean(normalized)
		if normalized == "." || path.IsAbs(normalized) {
			return nil, fmt.Errorf("技能包含非法路径 %q", f.Name)
		}
		if _, exists := seen[normalized]; exists {
			return nil, fmt.Errorf("技能包含重复条目 %q", f.Name)
		}
		seen[normalized] = struct{}{}
	}

	// 递归定位第一个 SKILL.md（以其所在目录为技能根，对"多包一层"宽容）。
	var skillEntry *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(path.Base(f.Name), "SKILL.md") {
			skillEntry = f
			break
		}
	}
	if skillEntry == nil {
		return nil, errors.New("技能包内未找到 SKILL.md")
	}
	skillRoot := path.Dir(skillEntry.Name) // 包内技能根前缀（"." 表示包根）
	if skillRoot == "." {
		skillRoot = "" // 归一：包根用空前缀
	}

	skillMD, err := readZipEntry(skillEntry, skillPkgMaxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 SKILL.md 失败: %w", err)
	}
	fm, _, err := agentkit.ParseSkillMarkdown(string(skillMD))
	if err != nil {
		return nil, err
	}
	name, _ := fm["name"].(string)
	desc, _ := fm["description"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("SKILL.md 缺少 frontmatter name")
	}
	if source == "" {
		source = "upload"
	}

	// 安全解压到 workspace/skills/<name>/。
	dir, err := uc.pkgDir(name)
	if err != nil {
		return nil, err
	}
	base, err := uc.skillsBaseDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, &SkillInternalError{Err: err}
	}
	staged, err := os.MkdirTemp(base, ".skill-staging-*")
	if err != nil {
		return nil, &SkillInternalError{Err: err}
	}
	defer os.RemoveAll(staged)
	if err := extractSkillPackage(zr, skillRoot, staged); err != nil {
		return nil, wrapSkillStorageError(err)
	}

	fmJSON, _ := json.Marshal(fm)
	s := &po.Skill{
		Name: name, Description: desc, Source: source,
		Frontmatter: datatypes.JSON(fmJSON), Content: string(skillMD),
		FilePath: dir, Package: zipBytes, HasFiles: true,
	}
	rollback, commit, err := installSkillDir(staged, dir)
	if err != nil {
		return nil, &SkillInternalError{Err: err}
	}
	if existing, err := uc.repo.GetByName(ctx, name); err == nil {
		s.ID = existing.ID
		s.ChainID = existing.ChainID
		s.TenantID = existing.TenantID
		s.Embedding = existing.Embedding
		s.CreatedAt = existing.CreatedAt
		if desc == "" {
			s.Description = existing.Description
		}
		if err := uc.repo.Update(ctx, s); err != nil {
			rollback()
			return nil, err
		}
	} else {
		if err := uc.repo.Create(ctx, s); err != nil {
			rollback()
			return nil, err
		}
	}
	commit()
	return toSkillView(s, true), nil
}

// EnsureExtracted 确保含包技能已落盘（幂等）。目录缺失时用 DB 归档重建（自愈），返回目录路径。
// 纯文本技能（HasFiles=false）返回其 FilePath（通常空）。
func (uc *SkillUsecase) EnsureExtracted(ctx context.Context, s *po.Skill) (string, error) {
	uc.packageMu.Lock()
	defer uc.packageMu.Unlock()

	if s == nil || !s.HasFiles {
		if s != nil {
			return s.FilePath, nil
		}
		return "", nil
	}
	dir, err := uc.pkgDir(s.Name)
	if err != nil {
		return "", err
	}
	// 已落盘且与记录一致 → 直接用。
	if st, err := os.Stat(dir); err == nil && st.IsDir() && s.FilePath == dir {
		if markerErr := verifySkillPackageDir(dir); markerErr == nil {
			return dir, nil
		}
	}
	// 缺失/不一致：从 DB 归档重建。
	if len(s.Package) == 0 {
		return "", fmt.Errorf("技能 %q 无包归档可重建", s.Name)
	}
	zr, err := zip.NewReader(bytes.NewReader(s.Package), int64(len(s.Package)))
	if err != nil {
		return "", fmt.Errorf("重建解压失败: %w", err)
	}
	skillRoot := skillRootPrefix(zr) // 已归一："."→""
	if err := extractSkillPackage(zr, skillRoot, dir); err != nil {
		return "", err
	}
	// 回填 FilePath（自愈后保持一致）。
	if s.FilePath != dir {
		s.FilePath = dir
		if err := uc.repo.Update(ctx, s); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// ListFiles 列出技能包内文件清单。读 DB 归档（权威源）在内存解析，不依赖磁盘。
func (uc *SkillUsecase) ListFiles(ctx context.Context, id int64) ([]SkillFileItem, error) {
	s, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if !s.HasFiles || len(s.Package) == 0 {
		return []SkillFileItem{}, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(s.Package), int64(len(s.Package)))
	if err != nil {
		return nil, fmt.Errorf("解析技能包失败: %w", err)
	}
	skillRoot := skillRootPrefix(zr)
	out := make([]SkillFileItem, 0, len(zr.File))
	for _, f := range zr.File {
		rel, ok := relToSkillRoot(f.Name, skillRoot)
		if !ok || rel == "" {
			continue
		}
		isDir := f.FileInfo().IsDir()
		if isDir {
			rel = strings.TrimSuffix(rel, "/")
			if rel == "" {
				continue
			}
		}
		out = append(out, SkillFileItem{Path: rel, Size: int64(f.UncompressedSize64), IsDir: isDir})
	}
	return out, nil
}

// ReadFile 读取技能包内单个文本文件内容（path 相对技能根）。二进制/越界/过大拒绝。
func (uc *SkillUsecase) ReadFile(ctx context.Context, id int64, relPath string) (string, error) {
	s, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return "", ErrNotFound
	}
	if !s.HasFiles || len(s.Package) == 0 {
		return "", errors.New("该技能不含技能包")
	}
	clean := path.Clean(strings.TrimPrefix(relPath, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("非法路径 %q", relPath)
	}
	zr, err := zip.NewReader(bytes.NewReader(s.Package), int64(len(s.Package)))
	if err != nil {
		return "", fmt.Errorf("解析技能包失败: %w", err)
	}
	skillRoot := skillRootPrefix(zr)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel, ok := relToSkillRoot(f.Name, skillRoot)
		if !ok || rel != clean {
			continue
		}
		data, err := readZipEntry(f, skillPkgMaxFileBytes)
		if err != nil {
			return "", err
		}
		if !isTextContent(data) {
			return "", fmt.Errorf("文件 %q 为二进制，不支持在线查看", relPath)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("文件 %q 不存在", relPath)
}

// DownloadPackage 返回技能包 zip 归档与建议下载文件名。
func (uc *SkillUsecase) DownloadPackage(ctx context.Context, id int64) (string, []byte, error) {
	s, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return "", nil, ErrNotFound
	}
	if !s.HasFiles || len(s.Package) == 0 {
		return "", nil, errors.New("该技能不含技能包")
	}
	return s.Name + ".zip", s.Package, nil
}

// ---- 内部工具 ----

// skillRootPrefix 返回包内技能根前缀（SKILL.md 所在目录）。"." 归一为 ""。
func skillRootPrefix(zr *zip.Reader) string {
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && strings.EqualFold(path.Base(f.Name), "SKILL.md") {
			d := path.Dir(f.Name)
			if d == "." {
				return ""
			}
			return d
		}
	}
	return ""
}

// relToSkillRoot 把包内路径转成相对技能根的路径；不在技能根下则 ok=false。
// 不做静默 Clean（"../x"→"x" 会掩盖逃逸）；显式拒绝含 ".."/绝对路径的条目（ok=false）。
func relToSkillRoot(name, skillRoot string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	if name == "" || path.IsAbs(name) {
		return "", false
	}
	// 显式拒绝任何含 ".." 段的路径（防逃逸）。
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", false
		}
	}
	if skillRoot == "" {
		return name, true
	}
	if name == skillRoot {
		return "", true
	}
	prefix := skillRoot + "/"
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix), true
	}
	return "", false
}

// readZipEntry 读取 zip 条目内容（限大小）。
func readZipEntry(f *zip.File, maxBytes int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > maxBytes {
		return nil, fmt.Errorf("文件 %q 超过大小上限", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件 %q 超过大小上限", f.Name)
	}
	return data, nil
}

// extractSkillPackage 把 zip 中技能根前缀下的条目安全解压到 destDir（先清空）。
// 全程路径校验防逃逸/符号链接/越界，限条目/单文件/总大小。
func extractSkillPackage(zr *zip.Reader, skillRoot, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	cleanDest := filepath.Clean(destDir)
	var total int64
	manifest := make(map[string]string)
	for _, f := range zr.File {
		rel, ok := relToSkillRoot(f.Name, skillRoot)
		if !ok {
			continue
		}
		if rel == "" {
			continue // 技能根目录本身
		}
		if rel == skillPackageComplete {
			continue
		}
		// 防逃逸：拒绝 ..、绝对路径。
		if strings.HasPrefix(rel, "../") || rel == ".." || path.IsAbs(rel) {
			return fmt.Errorf("技能包含非法路径 %q", f.Name)
		}
		// 防符号链接。
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			continue
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(rel))
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("技能包路径越界 %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		data, err := readZipEntry(f, skillPkgMaxFileBytes)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > skillPkgMaxTotal {
			return fmt.Errorf("技能包解压后总大小超过上限 %dMB", skillPkgMaxTotal>>20)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		manifest[rel] = hex.EncodeToString(sum[:])
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, skillPackageComplete), manifestData, 0o644); err != nil {
		return err
	}
	return nil
}

func verifySkillPackageDir(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, skillPackageComplete))
	if err != nil {
		return err
	}
	var expected map[string]string
	if err := json.Unmarshal(data, &expected); err != nil {
		return err
	}
	actual := make(map[string]string, len(expected))
	err = filepath.WalkDir(dir, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == skillPackageComplete {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		actual[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return errors.New("技能包目录文件数量不一致")
	}
	for name, sum := range expected {
		if actual[name] != sum {
			return fmt.Errorf("技能包文件 %q 校验失败", name)
		}
	}
	return nil
}

// installSkillDir 用临时目录替换技能目录，并返回回滚/提交操作。
// 回滚用于数据库写入失败时恢复旧目录，避免单次请求内 DB 与磁盘不一致。
func installSkillDir(staged, dest string) (rollback func(), commit func(), err error) {
	base := filepath.Dir(dest)
	var backup string
	if _, statErr := os.Stat(dest); statErr == nil {
		backup, err = os.MkdirTemp(base, ".skill-backup-*")
		if err != nil {
			return nil, nil, err
		}
		if err = os.RemoveAll(backup); err != nil {
			return nil, nil, err
		}
		if err = os.Rename(dest, backup); err != nil {
			return nil, nil, err
		}
	} else if !os.IsNotExist(statErr) {
		return nil, nil, statErr
	}
	if err = os.Rename(staged, dest); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return nil, nil, err
	}
	rollback = func() {
		_ = os.RemoveAll(dest)
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
	}
	commit = func() {
		if backup != "" {
			_ = os.RemoveAll(backup)
		}
	}
	return rollback, commit, nil
}

// isTextContent 粗略判断是否为可在线查看的文本（UTF-8 且无 NUL）。
func isTextContent(b []byte) bool {
	const sniff = 512
	n := len(b)
	if n > sniff {
		n = sniff
	}
	chunk := b[:n]
	if bytes.IndexByte(chunk, 0) >= 0 {
		return false
	}
	return utf8.Valid(chunk)
}
