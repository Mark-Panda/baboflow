package data

import (
	"context"
	"testing"

	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

func TestAuditRepo_ListFilterAndPagination(t *testing.T) {
	db := newTestDB(t, &po.AuditLog{})
	repo := NewAuditRepo(db)
	ctx := context.Background()

	uid1 := int64(1)
	uid2 := int64(2)
	seed := []*po.AuditLog{
		{UserID: &uid1, Action: "login", TargetType: "auth", Detail: datatypes.JSON([]byte("{}"))},
		{UserID: &uid1, Action: "login", TargetType: "auth", Detail: datatypes.JSON([]byte("{}"))},
		{UserID: &uid1, Action: "task.trigger", TargetType: "task", Detail: datatypes.JSON([]byte("{}"))},
		{UserID: &uid2, Action: "login", TargetType: "auth", Detail: datatypes.JSON([]byte("{}"))},
	}
	for _, e := range seed {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("seed audit: %v", err)
		}
	}

	// 不过滤：全部 4 条
	list, total, err := repo.List(ctx, "", nil, 1, 10)
	if err != nil || total != 4 || len(list) != 4 {
		t.Fatalf("no-filter: total=%d len=%d err=%v", total, len(list), err)
	}
	// id DESC：最新(id 最大)在前
	if list[0].ID < list[len(list)-1].ID {
		t.Fatalf("expected id DESC order")
	}

	// 按 action 过滤
	_, total, err = repo.List(ctx, "task.trigger", nil, 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("action filter: total=%d err=%v", total, err)
	}

	// 按 userID 过滤
	_, total, err = repo.List(ctx, "login", &uid1, 1, 10)
	if err != nil || total != 2 {
		t.Fatalf("userID+action filter: total=%d err=%v", total, err)
	}

	// 分页：每页 2 条，第 2 页应有 2 条
	page2, total, err := repo.List(ctx, "", nil, 2, 2)
	if err != nil || total != 4 || len(page2) != 2 {
		t.Fatalf("pagination: total=%d len(page2)=%d err=%v", total, len(page2), err)
	}
	// 第 3 页为空
	page3, _, _ := repo.List(ctx, "", nil, 3, 2)
	if len(page3) != 0 {
		t.Fatalf("page3 should be empty, got %d", len(page3))
	}
}
