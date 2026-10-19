package region

import (
	"encoding/binary"
	"testing"
	"time"
)

// padToSector 将数据补零到 4096 的整数倍（真实 region 文件按扇区对齐）。
func padToSector(data []byte) []byte {
	if rem := len(data) % sectorSize; rem != 0 {
		data = append(data, make([]byte, sectorSize-rem)...)
	}
	return data
}

// setPosEntry 写入第 i 个位置项的 offset/sectors。
func setPosEntry(data []byte, i, offset, sectors int) {
	base := i * 4
	data[base] = byte(offset >> 16)
	data[base+1] = byte(offset >> 8)
	data[base+2] = byte(offset)
	data[base+3] = byte(sectors)
}

// setTimestamp 写入第 i 个时间戳项。
func setTimestamp(data []byte, i int, ts uint32) {
	binary.BigEndian.PutUint32(data[4096+i*4:], ts)
}

// hasCode 判断问题列表中是否包含指定代码。
func hasCode(list []Issue, code string) bool {
	for _, it := range list {
		if it.Code == code {
			return true
		}
	}
	return false
}

func TestDiagnoseValid(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	d := Diagnose(data, 0, 0)
	if len(d.Errors) != 0 {
		t.Fatalf("有效文件不应有错误: %+v", d.Errors)
	}
	if len(d.Warnings) != 0 {
		t.Fatalf("对齐后的有效文件不应有警告: %+v", d.Warnings)
	}
	if d.Stats.TotalChunks != 1 || d.Stats.EmptySlots != 1023 {
		t.Fatalf("区块统计不符: %+v", d.Stats)
	}
	if d.Stats.Compression["zlib"] != 1 {
		t.Fatalf("压缩统计不符: %+v", d.Stats.Compression)
	}
	if d.Stats.TrailingBytes != 0 {
		t.Fatalf("对齐后残余字节应为 0: %d", d.Stats.TrailingBytes)
	}
	if d.Stats.OldestTimestamp != 1700000000 || d.Stats.NewestTimestamp != 1700000000 {
		t.Fatalf("时间戳统计不符: %+v", d.Stats)
	}
}

func TestDiagnoseFileTooSmall(t *testing.T) {
	d := Diagnose(make([]byte, 100), 0, 0)
	if !hasCode(d.Errors, IssueFileTooSmall) {
		t.Fatalf("过小文件应报 file_too_small: %+v", d.Errors)
	}
	if d.Stats.TotalChunks != 0 {
		t.Fatalf("过小文件不应统计区块: %+v", d.Stats)
	}
}

func TestDiagnoseOffsetBeyondFile(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	setPosEntry(data, 0, 9999, 1)
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Errors, IssueOffsetBeyondFile) {
		t.Fatalf("偏移越界应报 offset_beyond_file: %+v", d.Errors)
	}
	if !hasCode(d.Errors, IssueSectorsBeyondFile) {
		t.Fatalf("扇区范围越界应报 sectors_beyond_file: %+v", d.Errors)
	}
}

func TestDiagnoseOffsetInHeader(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	setPosEntry(data, 0, 1, 1)
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Errors, IssueOffsetInHeader) {
		t.Fatalf("偏移落在头部应报 offset_in_header: %+v", d.Errors)
	}
}

func TestDiagnoseUnknownCompression(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	data[2*sectorSize+4] = 42 // 压缩类型字节
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Errors, IssueUnknownCompression) {
		t.Fatalf("未知压缩类型应报错: %+v", d.Errors)
	}
}

func TestDiagnoseLengthBeyondFile(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	binary.BigEndian.PutUint32(data[2*sectorSize:], uint32(len(data)+1000))
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Errors, IssueLengthBeyondFile) {
		t.Fatalf("长度越界应报 length_beyond_file: %+v", d.Errors)
	}
	if !hasCode(d.Errors, IssueSectorsInsufficient) {
		t.Fatalf("扇区不足应报 sectors_insufficient: %+v", d.Errors)
	}
}

func TestDiagnoseFutureTimestamp(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	now := time.Unix(1700000000, 0)
	setTimestamp(data, 0, uint32(now.Add(48*time.Hour).Unix()))
	d := DiagnoseAt(data, 0, 0, now)
	if !hasCode(d.Warnings, IssueFutureTimestamp) {
		t.Fatalf("未来时间戳应告警: %+v", d.Warnings)
	}
}

func TestDiagnoseZeroTimestamp(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	setTimestamp(data, 0, 0)
	d := DiagnoseAt(data, 0, 0, time.Unix(1700000000, 0))
	if !hasCode(d.Warnings, IssueZeroTimestamp) {
		t.Fatalf("有数据但时间戳为 0 应告警: %+v", d.Warnings)
	}
}

func TestDiagnoseSectorOverlap(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	// 第二个位置项指向与第一项相同的扇区。
	setPosEntry(data, 1, 2, 1)
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Warnings, IssueSectorOverlap) {
		t.Fatalf("扇区重叠应告警: %+v", d.Warnings)
	}
}

func TestDiagnoseSectorBudgetExceeded(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	setPosEntry(data, 0, 1000, 255)
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Errors, IssueSectorBudgetExceeded) {
		t.Fatalf("扇区总数超容量应报错: %+v", d.Errors)
	}
}

func TestDiagnoseTrailingBytes(t *testing.T) {
	data := buildRegion(t, CompressionZlib) // 未补齐到扇区边界
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Warnings, IssueTrailingBytes) {
		t.Fatalf("未对齐文件应报 trailing_bytes: %+v", d.Warnings)
	}
}

func TestDiagnoseInconsistentEntry(t *testing.T) {
	data := padToSector(buildRegion(t, CompressionZlib))
	setPosEntry(data, 0, 0, 1) // offset=0 但 sectors=1
	d := Diagnose(data, 0, 0)
	if !hasCode(d.Warnings, IssueEntryInconsistent) {
		t.Fatalf("位置项不自洽应告警: %+v", d.Warnings)
	}
}
