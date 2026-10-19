// 缓存运维：列出本地缓存占用与清理（后台「MC 源码缓存」页使用）。
//
// jar 与索引都是「可重建的派生数据」：清理只意味着下次准备需要重新下载 /
// 重新建索引，不会丢任何业务数据，因此允许后台一键清理。
//
// 只管理本 Store 约定的三类文件（.jar / .index.json / .meta.json），
// 不做目录递归删除，避免误删运维放在同一目录下的其他文件。
package mcsrc

import (
	"errors"
	"os"
)

// CacheEntry 单条缓存的磁盘占用明细。
type CacheEntry struct {
	Status
	// IndexSize 索引文件字节数（未建索引时为 0）。
	IndexSize int64 `json:"index_size"`
	// MetaSize 元数据文件字节数。
	MetaSize int64 `json:"meta_size"`
	// TotalSize jar + 索引 + 元数据。
	TotalSize int64 `json:"total_size"`
}

// CacheUsage 缓存占用汇总。
type CacheUsage struct {
	// Entries 已缓存的 version+side 条目数。
	Entries int `json:"entries"`
	// JarBytes / IndexBytes / MetaBytes 分项占用。
	JarBytes   int64 `json:"jar_bytes"`
	IndexBytes int64 `json:"index_bytes"`
	MetaBytes  int64 `json:"meta_bytes"`
	// TotalBytes 合计占用。
	TotalBytes int64 `json:"total_bytes"`
}

// ListCache 列出全部本地缓存及其磁盘占用。
func (s *Store) ListCache() ([]CacheEntry, CacheUsage, error) {
	sts, err := s.ListPrepared()
	if err != nil {
		return nil, CacheUsage{}, err
	}
	out := make([]CacheEntry, 0, len(sts))
	var usage CacheUsage
	for _, st := range sts {
		e := CacheEntry{Status: st}
		e.IndexSize = fileSize(s.indexPath(st.Version, st.Side))
		e.MetaSize = fileSize(s.metaPath(st.Version, st.Side))
		e.TotalSize = st.Size + e.IndexSize + e.MetaSize
		usage.JarBytes += st.Size
		usage.IndexBytes += e.IndexSize
		usage.MetaBytes += e.MetaSize
		out = append(out, e)
	}
	usage.Entries = len(out)
	usage.TotalBytes = usage.JarBytes + usage.IndexBytes + usage.MetaBytes
	return out, usage, nil
}

// Purge 删除单个 version+side 的本地缓存（jar / 索引 / 元数据），
// 并失效对应的内存关系索引缓存。幂等：未缓存时返回 nil。
//
// 与 Prepare 共用同一把单飞锁：清理不会与「正在下载 / 建索引」交叉执行。
func (s *Store) Purge(version, side string) error {
	v, sd, err := normalize(version, side)
	if err != nil {
		return err
	}
	// 正在下载 / 建索引时禁止清理（gj.md §11）：快速失败而非排队等待，
	// 否则会在构建完成后立刻删掉刚生成的数据。
	if _, ok := s.progressOf(progKey(v, sd)); ok {
		return ErrCacheBusy
	}
	lock := s.lockFor(progKey(v, sd))
	lock.Lock()
	defer lock.Unlock()

	var firstErr error
	for _, p := range []string{s.jarPath(v, sd), s.indexPath(v, sd), s.metaPath(v, sd)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	s.dropRelations(v, sd)
	s.clearProgress(progKey(v, sd))
	return firstErr
}

// PurgeAll 清空全部本地缓存，返回成功清理的条目数与首个错误。
// 逐个清理而非删目录：即使中途失败，已清理的部分仍然生效。
func (s *Store) PurgeAll() (int, error) {
	sts, err := s.ListPrepared()
	if err != nil {
		return 0, err
	}
	purged := 0
	var firstErr error
	for _, st := range sts {
		if err := s.Purge(st.Version, st.Side); err != nil {
			// 正在准备的版本跳过，不计为失败（清理全部不应中断进行中的构建）。
			if errors.Is(err, ErrCacheBusy) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		purged++
	}
	return purged, firstErr
}

// fileSize 返回文件字节数；不存在或读取失败返回 0。
func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return 0
	}
	return fi.Size()
}
