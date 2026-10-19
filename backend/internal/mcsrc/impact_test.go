package mcsrc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCacheImpact 验证影响预览统计的分项与合计。
func TestCacheImpact(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 0, 0)

	// 直接落一个 jar / 索引 / 元数据文件，模拟已缓存版本。
	version, side := "1.21.1", "client"
	for _, p := range []string{s.jarPath(version, side), s.indexPath(version, side), s.metaPath(version, side)} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(p, []byte("0123456789"), 0o644); err != nil {
			t.Fatalf("写入文件失败: %v", err)
		}
	}

	impact, err := s.CacheImpact(version, side)
	if err != nil {
		t.Fatalf("CacheImpact 失败: %v", err)
	}
	if impact.TotalBytes != impact.JarBytes+impact.IndexBytes+impact.MetaBytes {
		t.Fatalf("合计与分项不一致: %+v", impact)
	}
	if impact.JarBytes != 10 || impact.IndexBytes != 10 || impact.MetaBytes != 10 {
		t.Fatalf("分项字节数异常: %+v", impact)
	}
	if len(impact.Files) != 3 {
		t.Fatalf("应列出 3 个待删文件，实际 %d", len(impact.Files))
	}
	if !impact.Rebuildable {
		t.Fatal("jar / 索引均为可重建派生数据，Rebuildable 应为 true")
	}
	if impact.Preparing {
		t.Fatal("未准备时 Preparing 应为 false")
	}
}

// TestCachePurgeRejectsPreparing 验证正在准备的版本禁止清理（gj.md §11）。
func TestCachePurgeRejectsPreparing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 0, 0)
	version, side := "1.20.1", "client"

	// 模拟进行中的准备：写入进度表但不真正下载。
	key := progKey(version, side)
	s.setProgress(key, progress{phase: PhaseDownloading, total: 100})
	defer s.clearProgress(key)

	if !s.IsPreparing(version, side) {
		t.Fatal("应识别为正在准备")
	}
	if err := s.Purge(version, side); !errors.Is(err, ErrCacheBusy) {
		t.Fatalf("正在准备时应返回 ErrCacheBusy，实际 %v", err)
	}
}

// TestCachePurgeAllSkipsPreparing 验证「清理全部」跳过正在准备的版本且不计为失败。
func TestCachePurgeAllSkipsPreparing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 0, 0)
	busyVersion, side := "1.19.4", "client"

	// 让 ListPrepared 能看到该条目：写一个元数据文件。
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := s.writeMeta(busyVersion, side, &entryMeta{Version: busyVersion, Side: side, Size: 1}); err != nil {
		t.Fatalf("写入元数据失败: %v", err)
	}
	key := progKey(busyVersion, side)
	s.setProgress(key, progress{phase: PhaseIndexing})
	defer s.clearProgress(key)

	purged, err := s.PurgeAll()
	if err != nil {
		t.Fatalf("PurgeAll 不应因忙碌条目报错: %v", err)
	}
	if purged != 0 {
		t.Fatalf("忙碌条目不应被清理，实际 purged=%d", purged)
	}
	if !s.IsPreparing(busyVersion, side) {
		t.Fatal("忙碌状态不应被清除")
	}
}
