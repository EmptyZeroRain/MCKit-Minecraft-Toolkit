package handler

import (
	"strings"
	"testing"

	mcstructure "mckit/backend/internal/mcstructure"
)

// TestStructureNote 三种版本支持情况的提示文案。
func TestStructureNote(t *testing.T) {
	// 结构尚未加入该版本：提示改选版本。
	if note := structureNote(false, true); !strings.Contains(note, "尚未加入") {
		t.Errorf("应提示结构未加入该版本，实际 %q", note)
	}
	// 旧版本分布算法不一致：提示不可作为依据。
	if note := structureNote(true, false); !strings.Contains(note, "不可作为依据") {
		t.Errorf("应提示算法差异，实际 %q", note)
	}
	// 正常情况：附群系可行性说明。
	if note := structureNote(true, true); !strings.Contains(note, "feasible") {
		t.Errorf("应包含群系可行性说明，实际 %q", note)
	}
}

// TestBiomeSupported 群系查询仅 1.18+ 支持。
func TestBiomeSupported(t *testing.T) {
	if !biomeSupported(mcstructure.Ver1_18Plus) {
		t.Error("1.18+ 应支持群系查询")
	}
	for _, v := range []mcstructure.Version{mcstructure.Ver1_8_1_12, mcstructure.Ver1_13_1_15, mcstructure.Ver1_16_1_17} {
		if biomeSupported(v) {
			t.Errorf("%s 不应支持群系查询", v)
		}
	}
}
