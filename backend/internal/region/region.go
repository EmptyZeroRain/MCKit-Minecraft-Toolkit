// Package region 解析 Minecraft Java 版 Anvil 区域文件（.mca）。
// 格式参考：Minecraft Wiki "Region file format"。
//
// 结构：
//   - 头部 8192 字节：4096 字节位置表 + 4096 字节时间戳表
//   - 位置项：3 字节大端 offset（单位 4KiB 扇区）+ 1 字节扇区数
//   - 区块数据：4 字节长度 + 1 字节压缩类型 + 压缩后的 NBT
package region

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

// 压缩类型。
const (
	CompressionGzip    = 1
	CompressionZlib    = 2
	CompressionNone    = 3
	CompressionUnknown = 0
)

// ChunkEntry 单个区块条目（不含 NBT 解析）。
type ChunkEntry struct {
	LocalX       int   `json:"local_x"` // 0-31
	LocalZ       int   `json:"local_z"` // 0-31
	GlobalChunkX int   `json:"global_chunk_x"`
	GlobalChunkZ int   `json:"global_chunk_z"`
	Timestamp    int64 `json:"timestamp"` // unix 秒
	Compression  int   `json:"compression"`
	DataLength   int   `json:"data_length"`
}

// RegionFile 解析结果。
type RegionFile struct {
	RegionX int          `json:"region_x"`
	RegionZ int          `json:"region_z"`
	Chunks  []ChunkEntry `json:"chunks"`
}

const (
	headerSize     = 8192
	sectorSize     = 4096
	chunksPerAxis  = 32
	maxSectorsByte = 255
)

// Parse 解析 region 文件字节流。
// regionX/regionZ 为区域坐标（文件名 r.<x>.<z>.mca 中的值），
// 用于计算区块的全局坐标。
func Parse(data []byte, regionX, regionZ int) (*RegionFile, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("文件过小，不足 region 头部（需 ≥ %d 字节）", headerSize)
	}

	rf := &RegionFile{RegionX: regionX, RegionZ: regionZ}

	for i := 0; i < chunksPerAxis*chunksPerAxis; i++ {
		base := i * 4
		offset := int(data[base])<<16 | int(data[base+1])<<8 | int(data[base+2])
		sectors := int(data[base+3])

		// 空区块（offset=0 且 sectors=0）
		if offset == 0 && sectors == 0 {
			continue
		}

		// 时间戳表紧随位置表
		tsBase := 4096 + i*4
		timestamp := int64(binary.BigEndian.Uint32(data[tsBase : tsBase+4]))

		localX := i % chunksPerAxis
		localZ := i / chunksPerAxis

		entry := ChunkEntry{
			LocalX:       localX,
			LocalZ:       localZ,
			GlobalChunkX: regionX*chunksPerAxis + localX,
			GlobalChunkZ: regionZ*chunksPerAxis + localZ,
			Timestamp:    timestamp,
			Compression:  CompressionUnknown,
		}

		// 读取区块数据（若文件被截断则跳过该区块）
		dataStart := offset * sectorSize
		if dataStart+5 <= len(data) {
			length := int(binary.BigEndian.Uint32(data[dataStart : dataStart+4]))
			if length > 0 && dataStart+5 <= len(data) {
				entry.Compression = int(data[dataStart+4])
				entry.DataLength = length
			}
		}

		rf.Chunks = append(rf.Chunks, entry)
	}

	// 按全局坐标排序，输出稳定。
	sort.Slice(rf.Chunks, func(i, j int) bool {
		if rf.Chunks[i].GlobalChunkZ != rf.Chunks[j].GlobalChunkZ {
			return rf.Chunks[i].GlobalChunkZ < rf.Chunks[j].GlobalChunkZ
		}
		return rf.Chunks[i].GlobalChunkX < rf.Chunks[j].GlobalChunkX
	})

	return rf, nil
}

// ExtractChunkNBT 解压并返回指定本地区块的原始 NBT 字节。
// 未做完整 NBT 结构解析，仅解压，供上层按需处理。
func ExtractChunkNBT(data []byte, localX, localZ int) ([]byte, int, error) {
	if localX < 0 || localX >= chunksPerAxis || localZ < 0 || localZ >= chunksPerAxis {
		return nil, 0, fmt.Errorf("本地区块坐标越界: (%d,%d)", localX, localZ)
	}
	i := localX + localZ*chunksPerAxis
	base := i * 4
	offset := int(data[base])<<16 | int(data[base+1])<<8 | int(data[base+2])
	if offset == 0 {
		return nil, 0, fmt.Errorf("该区块为空")
	}
	dataStart := offset * sectorSize
	if dataStart+5 > len(data) {
		return nil, 0, fmt.Errorf("区块数据超出文件范围")
	}
	length := int(binary.BigEndian.Uint32(data[dataStart : dataStart+4]))
	compression := int(data[dataStart+4])
	if length < 1 || dataStart+4+length > len(data) {
		return nil, 0, fmt.Errorf("区块数据长度非法")
	}
	compressed := data[dataStart+5 : dataStart+4+length]

	nbt, err := decompress(compressed, compression)
	if err != nil {
		return nil, compression, err
	}
	return nbt, compression, nil
}

// decompress 按压缩类型解压。
func decompress(data []byte, compression int) ([]byte, error) {
	switch compression {
	case CompressionGzip:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, 64<<20)) // 限制 64MiB，防内存放大
	case CompressionZlib:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, 64<<20))
	case CompressionNone:
		return data, nil
	default:
		return nil, fmt.Errorf("不支持的压缩类型: %d", compression)
	}
}

// ParseRegionName 从文件名解析 region 坐标（r.X.Z.mca）。
func ParseRegionName(name string) (int, int, error) {
	var x, z int
	if _, err := fmt.Sscanf(name, "r.%d.%d.mca", &x, &z); err != nil {
		return 0, 0, fmt.Errorf("region 文件名格式非法（应为 r.X.Z.mca）: %s", name)
	}
	return x, z, nil
}
