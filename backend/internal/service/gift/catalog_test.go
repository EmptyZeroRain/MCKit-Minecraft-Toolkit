package gift

import (
	"errors"
	"testing"
)

func TestRemainingStock(t *testing.T) {
	tests := []struct {
		name     string
		stock    int64
		redeemed int64
		wantNil  bool
		want     int64
	}{
		{name: "不限库存", stock: 0, redeemed: 10, wantNil: true},
		{name: "正常剩余", stock: 10, redeemed: 3, want: 7},
		{name: "刚好售罄", stock: 10, redeemed: 10, want: 0},
		{name: "超兑保护", stock: 10, redeemed: 12, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remainingStock(tt.stock, tt.redeemed)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("不限库存应返回 nil，实际 %v", *got)
				}
				return
			}
			if got == nil || *got != tt.want {
				t.Fatalf("剩余库存期望 %d，实际 %v", tt.want, got)
			}
		})
	}
}

func TestValidateStock(t *testing.T) {
	if err := validateStock(0); err != nil {
		t.Fatalf("0 应表示不限库存: %v", err)
	}
	if err := validateStock(maxGiftStock); err != nil {
		t.Fatalf("上限库存应合法: %v", err)
	}
	if err := validateStock(-1); !errors.Is(err, ErrGiftStockBad) {
		t.Fatalf("负库存应返回 ErrGiftStockBad，实际 %v", err)
	}
	if err := validateStock(maxGiftStock + 1); !errors.Is(err, ErrGiftStockBad) {
		t.Fatalf("超上限库存应返回 ErrGiftStockBad，实际 %v", err)
	}
}
