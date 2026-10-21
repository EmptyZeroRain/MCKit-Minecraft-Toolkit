package accountadmin

import (
	"context"
	"testing"

	"mckit/backend/internal/domain"
)

// TestDetailIncludesMakeupCardsIntegration 验证账号详情返回补签卡持有量。
func TestDetailIncludesMakeupCardsIntegration(t *testing.T) {
	gdb := setupAccountAdminDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	created, err := svc.Create(ctx, CreateInput{
		Username: "it-detail-cards", Password: "user-pass-123", Role: domain.RoleUser,
	})
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := gdb.Exec(`INSERT INTO gift_makeup_cards (user_id, quantity, updated_at)
		VALUES (?, 3, now())`, created.ID).Error; err != nil {
		t.Fatalf("写入补签卡库存失败: %v", err)
	}

	detail, err := svc.Detail(ctx, created.ID)
	if err != nil {
		t.Fatalf("查询账号详情失败: %v", err)
	}
	if detail.MakeupCards != 3 {
		t.Fatalf("补签卡数量期望 3，实际 %d", detail.MakeupCards)
	}
}
