package mcsrc

import "errors"

// ErrCacheBusy 目标版本正在下载 / 解压 / 建索引，禁止清理。
//
// gj.md §11：避免删除正在下载、解压或索引的目录——清理会破坏进行中的构建，
// 因此这里选择「快速失败」而不是等待构建结束。
var ErrCacheBusy = errors.New("该版本正在准备（下载 / 建索引），请稍后再清理")

// CacheImpact 清理某版本缓存的「影响预览」（gj.md §11）。
//
// 供前端在执行破坏性清理前展示：目标版本、预计释放空间、影响范围与可逆性。
type CacheImpact struct {
	Version string `json:"version"`
	Side    string `json:"side"`
	// Cached 是否已缓存；未缓存时清理无影响。
	Cached bool `json:"cached"`
	// JarBytes / IndexBytes / MetaBytes 分项占用。
	JarBytes   int64 `json:"jar_bytes"`
	IndexBytes int64 `json:"index_bytes"`
	MetaBytes  int64 `json:"meta_bytes"`
	// TotalBytes 预计释放的磁盘空间。
	TotalBytes int64 `json:"total_bytes"`
	// Preparing 是否正在下载 / 建索引；为 true 时清理会被拒绝。
	Preparing bool `json:"preparing"`
	// Rebuildable 恒为 true：jar 与索引都是可重建的派生数据，不涉及业务数据。
	Rebuildable bool `json:"rebuildable"`
	// Files 将被删除的文件清单（便于二次确认与审计）。
	Files []string `json:"files"`
}

// IsPreparing 判断某版本是否正在下载 / 建索引。
func (s *Store) IsPreparing(version, side string) bool {
	v, sd, err := normalize(version, side)
	if err != nil {
		return false
	}
	_, ok := s.progressOf(progKey(v, sd))
	return ok
}

// CacheImpact 计算清理某版本缓存的影响（不执行删除）。
func (s *Store) CacheImpact(version, side string) (CacheImpact, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return CacheImpact{}, err
	}
	st, err := s.Status(v, sd)
	if err != nil {
		return CacheImpact{}, err
	}

	jarPath := s.jarPath(v, sd)
	indexPath := s.indexPath(v, sd)
	metaPath := s.metaPath(v, sd)

	// jar 实际占用优先取磁盘文件大小；文件缺失时回退到元数据记录值，
	// 保证「预计释放空间」尽量贴近真实值（gj.md §11）。
	jarBytes := fileSize(jarPath)
	if jarBytes == 0 {
		jarBytes = st.Size
	}

	impact := CacheImpact{
		Version:     v,
		Side:        sd,
		Cached:      st.Cached,
		JarBytes:    jarBytes,
		IndexBytes:  fileSize(indexPath),
		MetaBytes:   fileSize(metaPath),
		Preparing:   s.IsPreparing(v, sd),
		Rebuildable: true,
	}
	impact.TotalBytes = impact.JarBytes + impact.IndexBytes + impact.MetaBytes

	files := make([]string, 0, 3)
	for _, p := range []string{jarPath, indexPath, metaPath} {
		if fileSize(p) > 0 {
			files = append(files, p)
		}
	}
	impact.Files = files
	return impact, nil
}
