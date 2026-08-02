package data

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"baboflow/internal/data/po"
)

// newTestDB 打开一个共享内存 SQLite，并迁移指定的模型。
// SQLite 与 Postgres 在 jsonb/vector 等类型上有差异，但本测试覆盖的模型
// （AdminUser/Session/LLMProvider/LLMModel/Agent）均可在 SQLite 上建表。
func newTestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	// 每个测试用独立的命名内存库，避免并发互相污染。
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// ---- AuthRepo ----

func TestAuthRepo_UserCRUD(t *testing.T) {
	db := newTestDB(t, &po.AdminUser{}, &po.Session{})
	repo := NewAuthRepo(db)
	ctx := context.Background()

	u := &po.AdminUser{Username: "admin", PasswordHash: "hash1", DisplayName: "管理员", MustChangePwd: true}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	got, err := repo.FindUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("FindUserByUsername: %v", err)
	}
	if got.ID != u.ID || !got.MustChangePwd {
		t.Fatalf("unexpected user: %+v", got)
	}

	// 改密后 mustChangePwd 应被清除
	if err := repo.UpdateUserPassword(ctx, u.ID, "hash2", false); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	got2, err := repo.FindUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindUserByID: %v", err)
	}
	if got2.PasswordHash != "hash2" || got2.MustChangePwd {
		t.Fatalf("password not updated: %+v", got2)
	}

	// 记录最后登录时间
	if err := repo.TouchLastLogin(ctx, u.ID); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}
	got3, _ := repo.FindUserByID(ctx, u.ID)
	if got3.LastLoginAt == nil {
		t.Fatal("LastLoginAt should be set")
	}
}

func TestAuthRepo_SessionLifecycle(t *testing.T) {
	db := newTestDB(t, &po.AdminUser{}, &po.Session{})
	repo := NewAuthRepo(db)
	ctx := context.Background()

	exp := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second)
	s := &po.Session{ID: "sid-abc", UserID: 1, IP: "127.0.0.1", UserAgent: "test", ExpiresAt: exp}
	if err := repo.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := repo.FindSession(ctx, "sid-abc")
	if err != nil {
		t.Fatalf("FindSession: %v", err)
	}
	if got.UserID != 1 {
		t.Fatalf("unexpected session: %+v", got)
	}

	// 滑动续期
	newExp := exp.Add(time.Hour)
	if err := repo.TouchSession(ctx, "sid-abc", newExp); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	got2, _ := repo.FindSession(ctx, "sid-abc")
	if !got2.ExpiresAt.After(exp) {
		t.Fatalf("expires not extended: %v vs %v", got2.ExpiresAt, exp)
	}

	if err := repo.DeleteSession(ctx, "sid-abc"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := repo.FindSession(ctx, "sid-abc"); err == nil {
		t.Fatal("session should be gone after delete")
	}
}

// ---- LLMRepo ----

func TestLLMRepo_ProviderAndModels(t *testing.T) {
	db := newTestDB(t, &po.LLMProvider{}, &po.LLMModel{}, &po.Agent{})
	repo := NewLLMRepo(db)
	ctx := context.Background()

	p := &po.LLMProvider{Name: "Kimi", Provider: "openai", BaseURL: "https://x/v1", APIKeyEnc: "enc"}
	if err := repo.CreateProvider(ctx, p); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	m1 := &po.LLMModel{ProviderID: p.ID, Model: "m-a", Alias: "a"}
	m2 := &po.LLMModel{ProviderID: p.ID, Model: "m-b", Alias: "b"}
	if err := repo.CreateModel(ctx, m1); err != nil {
		t.Fatalf("CreateModel m1: %v", err)
	}
	if err := repo.CreateModel(ctx, m2); err != nil {
		t.Fatalf("CreateModel m2: %v", err)
	}

	// ListProviders 预加载 Models
	list, err := repo.ListProviders(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListProviders: %v len=%d", err, len(list))
	}
	if len(list[0].Models) != 2 {
		t.Fatalf("expected 2 preloaded models, got %d", len(list[0].Models))
	}

	// SetDefaultModel 应互斥：只有一个 default
	if err := repo.SetDefaultModel(ctx, p.ID, m1.ID); err != nil {
		t.Fatalf("SetDefaultModel m1: %v", err)
	}
	if err := repo.SetDefaultModel(ctx, p.ID, m2.ID); err != nil {
		t.Fatalf("SetDefaultModel m2: %v", err)
	}
	models, _ := repo.ListModels(ctx, p.ID)
	defaults := 0
	for _, m := range models {
		if m.IsDefault {
			defaults++
			if m.ID != m2.ID {
				t.Fatalf("expected m2 default, got m%d", m.ID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly 1 default, got %d", defaults)
	}
}

func TestLLMRepo_DeleteProviderCascadesModels(t *testing.T) {
	db := newTestDB(t, &po.LLMProvider{}, &po.LLMModel{}, &po.Agent{})
	repo := NewLLMRepo(db)
	ctx := context.Background()

	p := &po.LLMProvider{Name: "P", Provider: "openai", BaseURL: "https://x", APIKeyEnc: "e"}
	if err := repo.CreateProvider(ctx, p); err != nil {
		t.Fatal(err)
	}
	m := &po.LLMModel{ProviderID: p.ID, Model: "m"}
	if err := repo.CreateModel(ctx, m); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteProvider(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if _, err := repo.GetProvider(ctx, p.ID); err == nil {
		t.Fatal("provider should be deleted")
	}
	models, _ := repo.ListModels(ctx, p.ID)
	if len(models) != 0 {
		t.Fatalf("models should cascade-delete, got %d", len(models))
	}
}

func TestLLMRepo_CountAgentByModel(t *testing.T) {
	db := newTestDB(t, &po.LLMProvider{}, &po.LLMModel{}, &po.Agent{})
	repo := NewLLMRepo(db)
	ctx := context.Background()

	mid := int64(42)
	a1 := &po.Agent{Key: "a1", Name: "A1", LLMModelID: &mid}
	a2 := &po.Agent{Key: "a2", Name: "A2", LLMModelID: &mid}
	a3 := &po.Agent{Key: "a3", Name: "A3"} // 无模型
	for _, a := range []*po.Agent{a1, a2, a3} {
		if err := db.Create(a).Error; err != nil {
			t.Fatalf("seed agent: %v", err)
		}
	}
	n, err := repo.CountAgentByModel(ctx, mid)
	if err != nil {
		t.Fatalf("CountAgentByModel: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 agents on model, got %d", n)
	}
}
