package region

import (
	"encoding/binary"
	"fmt"
	"sort"
	"time"
)

// 诊断级别。刻意区分「确定错误」与「警告」：
//   - error：结构已损坏，区块数据无法可靠读取（偏移/长度越界、扇区不足、未知压缩等）；
//   - warning：可疑但不必然导致读取失败（时间戳异常、扇区重叠、末尾残余等）。
//
// 仅做只读检查，绝不修改文件，也不提供自动修复。
const (
	LevelError   = "error"
	LevelWarning = "warning"
)

// 诊断结论代码，便于前端过滤与测试断言。
const (
	IssueFileTooSmall         = "file_too_small"
	IssueOffsetInHeader       = "offset_in_header"
	IssueOffsetBeyondFile     = "offset_beyond_file"
	IssueChunkHeaderTruncated = "chunk_header_truncated"
	IssueLengthInvalid        = "length_invalid"
	IssueLengthBeyondFile     = "length_beyond_file"
	IssueSectorsInsufficient  = "sectors_insufficient"
	IssueSectorsBeyondFile    = "sectors_beyond_file"
	IssueUnknownCompression   = "unknown_compression"
	IssueEntryInconsistent    = "entry_inconsistent"
	IssueFutureTimestamp      = "future_timestamp"
	IssueZeroTimestamp        = "zero_timestamp"
	IssueSectorOverlap        = "sector_overlap"
	IssueTrailingBytes        = "trailing_bytes"
	IssueSectorBudgetExceeded = "sector_budget_exceeded"
)

// Issue 单条诊断结论。
// LocalX/LocalZ 以指针表示，区分「文件级问题（无坐标）」与「区块级问题（坐标 0,0）」。
type Issue struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	LocalX  *int   `json:"local_x,omitempty"`
	LocalZ  *int   `json:"local_z,omitempty"`
}

// Stats 区域文件基础统计。
type Stats struct {
	FileSize        int            `json:"file_size"`
	TotalChunks     int            `json:"total_chunks"`     // 非空位置项数量
	EmptySlots      int            `json:"empty_slots"`      // 空位置项数量（应为 1024-TotalChunks）
	SectorsDeclared int            `json:"sectors_declared"` // 位置表声明的扇区总数
	SectorCapacity  int            `json:"sector_capacity"`  // 文件可容纳的扇区数（向下取整）
	TrailingBytes   int            `json:"trailing_bytes"`   // 末尾不足一个扇区的残余字节
	Compression     map[string]int `json:"compression"`      // 压缩类型名称 -> 区块数量
	OldestTimestamp int64          `json:"oldest_timestamp"` // 有效（>0）时间戳的最小值，unix 秒
	NewestTimestamp int64          `json:"newest_timestamp"` // 有效时间戳的最大值
}

// Diagnosis 只读诊断报告。
type Diagnosis struct {
	RegionX  int     `json:"region_x"`
	RegionZ  int     `json:"region_z"`
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
	Stats    Stats   `json:"stats"`
}

// maxFutureSkew 时间戳允许的最大超前量（时钟漂移/时区误设容差）。
const maxFutureSkew = 24 * time.Hour

// Diagnose 对区域文件做只读结构检查，返回诊断报告。
// now 由调用方注入以便测试；生产路径使用 time.Now()。
func Diagnose(data []byte, regionX, regionZ int) *Diagnosis {
	return DiagnoseAt(data, regionX, regionZ, time.Now())
}

// DiagnoseAt 与 Diagnose 相同，但允许指定「当前时间」，便于测试未来时间戳判定。
func DiagnoseAt(data []byte, regionX, regionZ int, now time.Time) *Diagnosis {
	d := &Diagnosis{
		RegionX:  regionX,
		RegionZ:  regionZ,
		Errors:   []Issue{},
		Warnings: []Issue{},
		Stats: Stats{
			FileSize:    len(data),
			Compression: map[string]int{},
		},
	}
	if len(data) < headerSize {
		d.Errors = append(d.Errors, Issue{
			Level:   LevelError,
			Code:    IssueFileTooSmall,
			Message: fmt.Sprintf("文件仅 %d 字节，不足区域头部（需 ≥ %d 字节），无法解析", len(data), headerSize),
		})
		return d
	}

	d.Stats.SectorCapacity = len(data) / sectorSize
	d.Stats.TrailingBytes = len(data) % sectorSize
	if d.Stats.TrailingBytes != 0 {
		d.Warnings = append(d.Warnings, Issue{
			Level:   LevelWarning,
			Code:    IssueTrailingBytes,
			Message: fmt.Sprintf("文件长度不是 4096 的整数倍，末尾有 %d 字节残余", d.Stats.TrailingBytes),
		})
	}

	futureLimit := now.Add(maxFutureSkew).Unix()
	occupied := make(map[int]int) // 扇区号 -> 占用该扇区的区块项索引（用于重叠检测）

	for i := 0; i < chunksPerAxis*chunksPerAxis; i++ {
		base := i * 4
		offset := int(data[base])<<16 | int(data[base+1])<<8 | int(data[base+2])
		sectors := int(data[base+3])

		if offset == 0 && sectors == 0 {
			d.Stats.EmptySlots++
			continue
		}

		localX := i % chunksPerAxis
		localZ := i / chunksPerAxis
		pos := func() (*int, *int) { x, z := localX, localZ; return &x, &z }

		d.Stats.TotalChunks++

		// 位置项不自洽：offset 与 sectors 只有一个为 0。
		if (offset == 0) != (sectors == 0) {
			x, z := pos()
			d.Warnings = append(d.Warnings, Issue{
				Level:   LevelWarning,
				Code:    IssueEntryInconsistent,
				Message: fmt.Sprintf("位置表项不自洽：offset=%d、sectors=%d（仅一方为 0）", offset, sectors),
				LocalX:  x, LocalZ: z,
			})
		}

		// offset < 2 会落在 2 个扇区的头部之内。
		if offset != 0 && offset < 2 {
			x, z := pos()
			d.Errors = append(d.Errors, Issue{
				Level:   LevelError,
				Code:    IssueOffsetInHeader,
				Message: fmt.Sprintf("区块数据偏移 %d 落在区域头部（前 2 个扇区），数据被头部覆盖", offset),
				LocalX:  x, LocalZ: z,
			})
			continue
		}

		// 时间戳检查（独立于数据是否可读）。
		tsBase := 4096 + i*4
		timestamp := int64(binary.BigEndian.Uint32(data[tsBase : tsBase+4]))
		if timestamp > 0 {
			if d.Stats.OldestTimestamp == 0 || timestamp < d.Stats.OldestTimestamp {
				d.Stats.OldestTimestamp = timestamp
			}
			if timestamp > d.Stats.NewestTimestamp {
				d.Stats.NewestTimestamp = timestamp
			}
			if timestamp > futureLimit {
				x, z := pos()
				d.Warnings = append(d.Warnings, Issue{
					Level:   LevelWarning,
					Code:    IssueFutureTimestamp,
					Message: fmt.Sprintf("时间戳位于未来（%s）", time.Unix(timestamp, 0).UTC().Format(time.RFC3339)),
					LocalX:  x, LocalZ: z,
				})
			}
		} else {
			x, z := pos()
			d.Warnings = append(d.Warnings, Issue{
				Level:   LevelWarning,
				Code:    IssueZeroTimestamp,
				Message: "位置项有数据但时间戳为 0",
				LocalX:  x, LocalZ: z,
			})
		}

		// offset=0 表示无数据（sectors>0 属位置项不自洽，上面已告警），跳过数据检查。
		if offset == 0 {
			continue
		}

		// 扇区预算：文件容量是否装得下声明的范围。
		dataStart := offset * sectorSize
		declaredEnd := dataStart + sectors*sectorSize
		if sectors > 0 {
			d.Stats.SectorsDeclared += sectors
		}
		if sectors > 0 && declaredEnd > len(data) {
			x, z := pos()
			d.Errors = append(d.Errors, Issue{
				Level:   LevelError,
				Code:    IssueSectorsBeyondFile,
				Message: fmt.Sprintf("声明的扇区范围 [%d,%d) 超出文件末尾（%d 字节）", dataStart, declaredEnd, len(data)),
				LocalX:  x, LocalZ: z,
			})
		}

		// 扇区重叠：同一扇区被多个区块项占用，后写入者读取会得到错误数据。
		if sectors > 0 {
			for s := offset; s < offset+sectors && s < d.Stats.SectorCapacity; s++ {
				if owner, ok := occupied[s]; ok {
					x, z := pos()
					d.Warnings = append(d.Warnings, Issue{
						Level:   LevelWarning,
						Code:    IssueSectorOverlap,
						Message: fmt.Sprintf("扇区 %d 同时被位置项 #%d 与本项占用", s, owner),
						LocalX:  x, LocalZ: z,
					})
				} else {
					occupied[s] = i
				}
			}
		}

		// 区块数据头部（长度 + 压缩类型）可读性检查。
		if dataStart+5 > len(data) {
			if dataStart < len(data) {
				x, z := pos()
				d.Errors = append(d.Errors, Issue{
					Level:   LevelError,
					Code:    IssueChunkHeaderTruncated,
					Message: "区块数据头部（长度 + 压缩类型，共 5 字节）被截断",
					LocalX:  x, LocalZ: z,
				})
			} else if offset >= 2 {
				x, z := pos()
				d.Errors = append(d.Errors, Issue{
					Level:   LevelError,
					Code:    IssueOffsetBeyondFile,
					Message: fmt.Sprintf("区块数据偏移 %d（字节 %d）超出文件末尾（%d 字节）", offset, dataStart, len(data)),
					LocalX:  x, LocalZ: z,
				})
			}
			continue
		}

		length := int(uint32(data[dataStart])<<24 | uint32(data[dataStart+1])<<16 | uint32(data[dataStart+2])<<8 | uint32(data[dataStart+3]))
		compression := int(data[dataStart+4])

		if length < 1 {
			x, z := pos()
			d.Errors = append(d.Errors, Issue{
				Level:   LevelError,
				Code:    IssueLengthInvalid,
				Message: fmt.Sprintf("区块数据长度非法：%d（应 ≥ 1，需包含压缩类型字节）", length),
				LocalX:  x, LocalZ: z,
			})
		}

		// 数据实际占用（4 字节长度 + length 字节负载）所需的扇区数。
		if length >= 1 {
			needed := (4 + length + sectorSize - 1) / sectorSize
			if sectors < needed {
				x, z := pos()
				d.Errors = append(d.Errors, Issue{
					Level:   LevelError,
					Code:    IssueSectorsInsufficient,
					Message: fmt.Sprintf("声明的扇区数不足以容纳数据：需要 %d，声明 %d", needed, sectors),
					LocalX:  x, LocalZ: z,
				})
			}
			if dataStart+4+length > len(data) {
				x, z := pos()
				d.Errors = append(d.Errors, Issue{
					Level:   LevelError,
					Code:    IssueLengthBeyondFile,
					Message: fmt.Sprintf("区块数据声明长度 %d 超出文件末尾（数据从字节 %d 开始）", length, dataStart+5),
					LocalX:  x, LocalZ: z,
				})
			}
		}

		if compression != CompressionGzip && compression != CompressionZlib && compression != CompressionNone {
			x, z := pos()
			d.Errors = append(d.Errors, Issue{
				Level:   LevelError,
				Code:    IssueUnknownCompression,
				Message: fmt.Sprintf("不支持的压缩类型：%d（仅支持 1=gzip、2=zlib、3=无压缩）", compression),
				LocalX:  x, LocalZ: z,
			})
		} else {
			d.Stats.Compression[compressionName(compression)]++
		}
	}

	if d.Stats.SectorsDeclared > d.Stats.SectorCapacity {
		d.Errors = append(d.Errors, Issue{
			Level: LevelError,
			Code:  IssueSectorBudgetExceeded,
			Message: fmt.Sprintf("位置表声明的扇区总数 %d 超过文件容量 %d，文件被截断或头部损坏",
				d.Stats.SectorsDeclared, d.Stats.SectorCapacity),
		})
	}

	// 输出稳定：按 (localZ, localX, code) 排序，避免 map 迭代带来的抖动。
	sortIssues(d.Errors)
	sortIssues(d.Warnings)
	return d
}

// compressionName 压缩类型的人类可读名称（用于统计键）。
func compressionName(c int) string {
	switch c {
	case CompressionGzip:
		return "gzip"
	case CompressionZlib:
		return "zlib"
	case CompressionNone:
		return "none"
	default:
		return "unknown"
	}
}

// sortIssues 按坐标与代码排序；文件级问题（无坐标）排在最前。
func sortIssues(list []Issue) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch {
		case a.LocalZ == nil && b.LocalZ != nil:
			return true
		case a.LocalZ != nil && b.LocalZ == nil:
			return false
		case a.LocalZ != nil && b.LocalZ != nil:
			if *a.LocalZ != *b.LocalZ {
				return *a.LocalZ < *b.LocalZ
			}
			if *a.LocalX != *b.LocalX {
				return *a.LocalX < *b.LocalX
			}
		}
		return a.Code < b.Code
	})
}
