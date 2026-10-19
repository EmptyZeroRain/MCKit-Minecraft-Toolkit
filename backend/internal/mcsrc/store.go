// Package mcsrc 提供「MC 源码浏览器」的后端支持：
// 服务端按版本下载并缓存 Minecraft jar，并基于 jar 条目构建类索引，
// 供前端按版本浏览 / 搜索类，避免每个访客各自从 Mojang 下载数百 MB。
//
// 设计说明：
//   - 文件布局（均在 MCSRCCacheDir 下，扁平命名，版本号经白名单校验防路径穿越）：
//     <version>-<side>.jar         原始 jar
//     <version>-<side>.index.json  类索引
//     <version>-<side>.meta.json   元数据（jar 摘要 + 索引摘要）
//   - 下载先写临时文件，校验 sha1 后再原子改名，避免半成品被当成缓存；
//   - 同一 version+side 的下载 / 建索引串行化（单飞），防止并发重复下载数百 MB；
//   - 一切体积与数量都有上限，避免异常上游数据耗尽磁盘或内存。
package mcsrc

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mckit/backend/internal/mcversion"
)

// 侧别。
const (
	SideClient = "client"
	SideServer = "server"
)

// defaultDownloadTimeout jar 体积可达数百 MB，需较长的整体超时。
const defaultDownloadTimeout = 10 * time.Minute

var (
	// ErrDisabled 源码库未启用。
	ErrDisabled = errors.New("源码库未启用")
	// ErrBadVersion 版本号非法（版本号会拼进文件路径，必须严格校验）。
	ErrBadVersion = errors.New("版本号非法")
	// ErrBadSide 侧别非法。
	ErrBadSide = errors.New("侧别非法，仅支持 client / server")
	// ErrNoDownload 该版本缺少对应侧别的下载信息。
	ErrNoDownload = errors.New("该版本没有对应侧别的下载信息")
	// ErrTooLarge jar 体积超出上限。
	ErrTooLarge = errors.New("jar 体积超出上限")
	// ErrChecksum jar 校验失败（sha1 与官方清单不一致）。
	ErrChecksum = errors.New("jar 校验失败（sha1 与官方清单不一致）")
	// ErrNotFound 尚未准备该版本（未缓存 jar）。
	ErrNotFound = errors.New("尚未准备该版本")
	// ErrNotIndexed 已缓存 jar 但尚未建立类索引。
	ErrNotIndexed = errors.New("该版本尚未建立类索引")
)

// versionPattern 版本号白名单：字母数字开头，仅含字母、数字、点、下划线、连字符。
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// entryMeta 单条缓存的元数据（jar + 索引摘要）。
type entryMeta struct {
	Version   string     `json:"version"`
	Side      string     `json:"side"`
	SHA1      string     `json:"sha1"`
	Size      int64      `json:"size"`
	URL       string     `json:"url"`
	FetchedAt time.Time  `json:"fetched_at"`
	Index     *indexMeta `json:"index,omitempty"`
}

// indexMeta 类索引摘要。
type indexMeta struct {
	BuiltAt    time.Time `json:"built_at"`
	ClassCount int       `json:"class_count"`
	Truncated  bool      `json:"truncated"`
}

// Status 单个 version+side 的准备状态。
// 时间字段用指针 + omitempty：零值时间会被编码成 `0001-01-01T00:00:00Z`，
// 前端会误渲染成无效日期，因此未缓存 / 未建索引时直接省略。
type Status struct {
	Version      string     `json:"version"`
	Side         string     `json:"side"`
	Cached       bool       `json:"cached"`
	Size         int64      `json:"size"`
	SHA1         string     `json:"sha1"`
	FetchedAt    *time.Time `json:"fetched_at,omitempty"`
	Indexed      bool       `json:"indexed"`
	ClassCount   int        `json:"class_count"`
	Truncated    bool       `json:"truncated"`
	IndexBuiltAt *time.Time `json:"index_built_at,omitempty"`

	// 进行中的准备进度（仅内存，未在准备时为零值）。
	// 前端据此渲染进度条：downloading 阶段有 total 时为确定进度，否则为不确定进度。
	Preparing  bool   `json:"preparing"`
	Phase      string `json:"phase,omitempty"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}

// 准备阶段（供前端区分「下载中（可算百分比）」与「建索引中（不确定进度）」）。
const (
	// PhaseDownloading 正在下载 jar。
	PhaseDownloading = "downloading"
	// PhaseIndexing 正在建立类索引。
	PhaseIndexing = "indexing"
)

// progress 单个目标的准备进度。
type progress struct {
	phase      string
	downloaded int64
	total      int64
}

// Store jar 与索引的磁盘缓存。
type Store struct {
	dir        string
	maxBytes   int64
	maxClasses int
	http       *http.Client
	locks      sync.Map // key(version-side) -> *sync.Mutex，串行化同一目标的下载与建索引

	// relMu / rel 继承关系索引的内存缓存（key = version-side，值记录 jar 摘要用于失效）。
	// 构建需全量扫描 jar 内所有 class，故只缓存最近若干个版本（见 relationsFor）。
	relMu sync.Mutex
	rel   map[string]relCache

	// refMu / ref 反向引用索引的内存缓存（key = version-side，值记录 jar 摘要用于失效）。
	// 构建需全量扫描 jar 内所有 class 的常量池与指令，故只保留最近若干个版本（见 referencesFor）。
	refMu sync.Mutex
	ref   map[string]refCache

	// progMu / prog 进行中的准备进度（key = version-side）。
	// 只服务于进度展示，不落盘；进程重启后由下一次 Prepare 重新填充。
	progMu sync.Mutex
	prog   map[string]progress
}

// NewStore 创建缓存。maxBytes 为单个 jar 体积上限，maxClasses 为建索引的类数量上限。
func NewStore(dir string, maxBytes int64, maxClasses int) *Store {
	if maxBytes <= 0 {
		maxBytes = 512 << 20
	}
	if maxClasses <= 0 {
		maxClasses = 60000
	}
	return &Store{
		dir:        dir,
		maxBytes:   maxBytes,
		maxClasses: maxClasses,
		http:       &http.Client{Timeout: defaultDownloadTimeout},
		prog:       map[string]progress{},
		ref:        map[string]refCache{},
	}
}

// WithHTTP 注入自定义 HTTP 客户端（测试用）。
func (s *Store) WithHTTP(h *http.Client) *Store {
	if h != nil {
		s.http = h
	}
	return s
}

// lockFor 取同一目标的互斥锁（单飞）。
func (s *Store) lockFor(key string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// progKey 进度 / 单飞锁的键（与 lockFor 的键保持一致）。
func progKey(version, side string) string { return version + "-" + side }

// setProgress 记录某目标的准备进度。
func (s *Store) setProgress(key string, p progress) {
	s.progMu.Lock()
	s.prog[key] = p
	s.progMu.Unlock()
}

// clearProgress 结束某目标的进度跟踪。
func (s *Store) clearProgress(key string) {
	s.progMu.Lock()
	delete(s.prog, key)
	s.progMu.Unlock()
}

// progressOf 读取某目标的准备进度。
func (s *Store) progressOf(key string) (progress, bool) {
	s.progMu.Lock()
	p, ok := s.prog[key]
	s.progMu.Unlock()
	return p, ok
}

// progressWriter 把已写入的字节数累加到进度表。
// io.Copy 以 32KB 分块调用 Write，对进度条而言足够平滑，无需额外节流。
type progressWriter struct {
	store *Store
	key   string
}

func (w progressWriter) Write(p []byte) (int, error) {
	w.store.progMu.Lock()
	if cur, ok := w.store.prog[w.key]; ok {
		cur.downloaded += int64(len(p))
		w.store.prog[w.key] = cur
	}
	w.store.progMu.Unlock()
	return len(p), nil
}

func (s *Store) jarPath(version, side string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-%s.jar", version, side))
}

func (s *Store) indexPath(version, side string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-%s.index.json", version, side))
}

func (s *Store) metaPath(version, side string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-%s.meta.json", version, side))
}

// normalize 校验并归一化版本号与侧别。
func normalize(version, side string) (string, string, error) {
	v := strings.TrimSpace(version)
	if !versionPattern.MatchString(v) {
		return "", "", ErrBadVersion
	}
	sd := strings.ToLower(strings.TrimSpace(side))
	if sd == "" {
		sd = SideClient
	}
	if sd != SideClient && sd != SideServer {
		return "", "", ErrBadSide
	}
	return v, sd, nil
}

// Status 返回指定 version+side 的准备状态（不触发任何下载）。
func (s *Store) Status(version, side string) (Status, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return Status{}, err
	}
	return s.statusLocked(v, sd), nil
}

// ListPrepared 列出所有已缓存的 version+side（按版本降序、侧别升序）。
func (s *Store) ListPrepared() ([]Status, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Status{}, nil
		}
		return nil, err
	}
	out := make([]Status, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		meta, err := s.readMeta(filepath.Join(s.dir, name))
		if err != nil || meta == nil {
			continue
		}
		out = append(out, meta.toStatus())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version > out[j].Version
		}
		return out[i].Side < out[j].Side
	})
	return out, nil
}

// OpenJar 打开已缓存的 jar 文件，供下载 / 读取使用，调用方负责关闭。
// 未缓存时返回 ErrNotFound（与 Status.Cached=false 语义一致，不触发下载）。
func (s *Store) OpenJar(version, side string) (*os.File, os.FileInfo, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(s.jarPath(v, sd))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, fi, nil
}

// Prepare 确保 jar 已缓存且已建索引：必要时下载 jar（校验 sha1）并构建类索引。
func (s *Store) Prepare(ctx context.Context, version, side string, dl mcversion.DownloadInfo) (Status, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return Status{}, err
	}
	if strings.TrimSpace(dl.URL) == "" {
		return Status{}, ErrNoDownload
	}
	if dl.Size > s.maxBytes {
		return Status{}, ErrTooLarge
	}

	lock := s.lockFor(v + "-" + sd)
	lock.Lock()
	defer lock.Unlock()

	// 进度跟踪：整个准备过程（下载 + 建索引）内可见，供前端渲染进度条。
	key := progKey(v, sd)
	defer s.clearProgress(key)

	st := s.statusLocked(v, sd)
	// 已缓存且摘要与官方清单一致 → 无需重新下载。
	if st.Cached && (dl.SHA1 == "" || strings.EqualFold(st.SHA1, dl.SHA1)) {
		if st.Indexed {
			return st, nil
		}
	} else {
		s.setProgress(key, progress{phase: PhaseDownloading, total: dl.Size})
		if err := s.download(ctx, v, sd, dl); err != nil {
			return Status{}, err
		}
	}

	s.setProgress(key, progress{phase: PhaseIndexing, downloaded: dl.Size, total: dl.Size})
	if err := s.buildIndexLocked(ctx, v, sd); err != nil {
		return Status{}, err
	}
	// 完成即清除进度：否则响应体里会残留 preparing=true（前端据此渲染进度条）。
	s.clearProgress(key)
	return s.statusLocked(v, sd), nil
}

// download 下载并校验 jar，成功后原子替换缓存文件。
func (s *Store) download(ctx context.Context, version, side string, dl mcversion.DownloadInfo) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dl.URL, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 jar 失败：上游返回 %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(s.dir, "dl-*.part")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 失败路径清理临时文件；成功路径改名后 Remove 会因文件不存在而忽略错误。
	defer func() { _ = os.Remove(tmpName) }()

	hasher := sha1.New()
	// LimitReader 多读 1 字节：既能发现超限，也不会无限写入磁盘。
	// 第三个 writer 只累计已下载字节数，用于前端进度条。
	written, err := io.Copy(
		io.MultiWriter(tmp, hasher, progressWriter{store: s, key: progKey(version, side)}),
		io.LimitReader(resp.Body, s.maxBytes+1),
	)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if written > s.maxBytes {
		_ = tmp.Close()
		return ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	if dl.SHA1 != "" && !strings.EqualFold(sum, dl.SHA1) {
		return ErrChecksum
	}
	if dl.Size > 0 && written != dl.Size {
		return fmt.Errorf("%w: 期望 %d 字节，实际 %d", ErrChecksum, dl.Size, written)
	}

	if err := os.Rename(tmpName, s.jarPath(version, side)); err != nil {
		return err
	}

	// jar 内容变化时旧索引失效，需重建。
	meta := &entryMeta{
		Version:   version,
		Side:      side,
		SHA1:      sum,
		Size:      written,
		URL:       dl.URL,
		FetchedAt: time.Now().UTC(),
	}
	return s.writeMeta(version, side, meta)
}

// buildIndexLocked 基于已缓存 jar 构建类索引（调用方需持有该目标的锁）。
func (s *Store) buildIndexLocked(ctx context.Context, version, side string) error {
	meta, err := s.readMeta(s.metaPath(version, side))
	if err != nil {
		return err
	}
	if meta == nil {
		return ErrNotFound
	}
	if meta.Index != nil {
		// 索引已存在且 jar 摘要未变（摘要写入同一文件，天然一致）。
		return nil
	}

	idx, err := BuildIndex(ctx, s.jarPath(version, side), version, side, meta.SHA1, s.maxClasses)
	if err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.indexPath(version, side), data, 0o644); err != nil {
		return err
	}

	meta.Index = &indexMeta{
		BuiltAt:    idx.BuiltAt,
		ClassCount: len(idx.Classes),
		Truncated:  idx.Truncated,
	}
	return s.writeMeta(version, side, meta)
}

// LoadIndex 读取类索引；未建索引时返回 ErrNotIndexed。
func (s *Store) LoadIndex(version, side string) (*Index, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.indexPath(v, sd))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotIndexed
		}
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("索引文件损坏: %w", err)
	}
	return &idx, nil
}

// Search 在指定版本的索引中检索类。
func (s *Store) Search(version, side string, q SearchQuery) (SearchResult, error) {
	idx, err := s.LoadIndex(version, side)
	if err != nil {
		return SearchResult{}, err
	}
	return idx.Search(q), nil
}

// statusLocked 由元数据文件推导状态（不读取索引正文），并合并进行中的准备进度。
func (s *Store) statusLocked(version, side string) Status {
	meta, err := s.readMeta(s.metaPath(version, side))
	var st Status
	if err != nil || meta == nil {
		st = Status{Version: version, Side: side}
	} else {
		st = meta.toStatus()
	}
	if p, ok := s.progressOf(progKey(version, side)); ok {
		st.Preparing = true
		st.Phase = p.phase
		st.Downloaded = p.downloaded
		st.Total = p.total
	}
	return st
}

func (m *entryMeta) toStatus() Status {
	st := Status{
		Version: m.Version,
		Side:    m.Side,
		Cached:  m.Size > 0,
		Size:    m.Size,
		SHA1:    m.SHA1,
	}
	if !m.FetchedAt.IsZero() {
		at := m.FetchedAt
		st.FetchedAt = &at
	}
	if m.Index != nil {
		st.Indexed = true
		st.ClassCount = m.Index.ClassCount
		st.Truncated = m.Index.Truncated
		if !m.Index.BuiltAt.IsZero() {
			at := m.Index.BuiltAt
			st.IndexBuiltAt = &at
		}
	}
	return st
}

func (s *Store) readMeta(path string) (*entryMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var meta entryMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		// 元数据损坏视为未缓存，交由下一次 Prepare 重建。
		return nil, nil
	}
	return &meta, nil
}

func (s *Store) writeMeta(version, side string, meta *entryMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(version, side), data, 0o644)
}
