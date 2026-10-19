package handler

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// lvRegionFile 构造一个仅含单个区块（本地 0,0，无压缩）的最小 .mca 字节流。
func lvRegionFile() []byte {
	const sector = 4096
	out := make([]byte, 3*sector)

	// 位置项 0：offset=2、sectors=1。
	out[0] = 0
	out[1] = 0
	out[2] = 2
	out[3] = 1
	binary.BigEndian.PutUint32(out[4096:], uint32(time.Now().Unix()))

	// 数据扇区：长度 + 压缩类型(3=无压缩) + 负载。
	payload := []byte{0x0A, 0x00, 0x00} // 非完整 NBT：TAG_Compound + 空名 + TAG_End
	length := uint32(1 + len(payload))
	start := 2 * sector
	binary.BigEndian.PutUint32(out[start:], length)
	out[start+4] = 3
	copy(out[start+5:], payload)
	return out
}

// lvRegionRequest 组装一次 /mc/region/inspect 请求。
func lvRegionRequest(t *testing.T, fileName string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mc/region/inspect", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	NewRegionHandler(0, 0).Inspect(rec, req)
	return rec
}

func TestRegionInspectBadFilename(t *testing.T) {
	rec := lvRegionRequest(t, "not-a-region.mca", lvRegionFile())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法文件名应返回 422，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

func TestRegionInspectIncludesDiagnosis(t *testing.T) {
	rec := lvRegionRequest(t, "r.0.0.mca", lvRegionFile())
	if rec.Code != http.StatusOK {
		t.Fatalf("有效文件应返回 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ChunkCount int `json:"chunk_count"`
			Stats      struct {
				TotalChunks int `json:"total_chunks"`
			} `json:"stats"`
			Diagnosis struct {
				Errors   []map[string]any `json:"errors"`
				Warnings []map[string]any `json:"warnings"`
			} `json:"diagnosis"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.ChunkCount != 1 || resp.Data.Stats.TotalChunks != 1 {
		t.Fatalf("区块统计不符: %#v", resp.Data)
	}
	if len(resp.Data.Diagnosis.Errors) != 0 {
		t.Fatalf("有效文件不应有诊断错误: %#v", resp.Data.Diagnosis.Errors)
	}
}
