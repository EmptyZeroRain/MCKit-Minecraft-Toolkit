package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---- NBT 构造工具（测试专用，命名加 lv 前缀避免与其他测试文件冲突）----

func lvBe16(n uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, n)
	return b
}

func lvBe32(n int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

func lvBe64(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func lvStr(s string) []byte {
	return append(lvBe16(uint16(len(s))), []byte(s)...)
}

func lvTag(tagType byte, name string, payload []byte) []byte {
	out := []byte{tagType}
	out = append(out, lvStr(name)...)
	return append(out, payload...)
}

// lvLevelDat 构造一个 gzip 压缩的 level.dat 字节流。
func lvLevelDat(t *testing.T) []byte {
	t.Helper()

	var data bytes.Buffer
	data.Write(lvTag(3, "DataVersion", lvBe32(3465)))
	data.Write(lvTag(8, "LevelName", lvStr("测试世界")))
	data.Write(lvTag(4, "LastPlayed", lvBe64(1700000000000)))
	data.Write(lvTag(4, "RandomSeed", lvBe64(12345)))
	data.Write(lvTag(3, "SpawnX", lvBe32(10)))
	data.Write(lvTag(3, "SpawnY", lvBe32(64)))
	data.Write(lvTag(3, "SpawnZ", lvBe32(-3)))
	data.Write(lvTag(4, "Time", lvBe64(1000)))
	data.Write(lvTag(4, "DayTime", lvBe64(500)))
	data.Write(lvTag(3, "GameType", lvBe32(1)))
	data.Write(lvTag(1, "Difficulty", []byte{2}))
	data.Write(lvTag(1, "hardcore", []byte{0}))
	data.Write(lvTag(1, "allowCommands", []byte{1}))
	data.Write(lvTag(8, "generatorName", lvStr("default")))

	// Version 复合标签
	var version bytes.Buffer
	version.Write(lvTag(3, "Id", lvBe32(3465)))
	version.Write(lvTag(8, "Name", lvStr("1.20.1")))
	version.Write(lvTag(1, "Snapshot", []byte{0}))
	version.WriteByte(0) // End
	data.Write(lvTag(10, "Version", version.Bytes()))

	// GameRules 复合标签
	var rules bytes.Buffer
	rules.Write(lvTag(8, "doDaylightCycle", lvStr("true")))
	rules.WriteByte(0)
	data.Write(lvTag(10, "GameRules", rules.Bytes()))

	data.WriteByte(0) // Data End

	var root bytes.Buffer
	root.WriteByte(10)
	root.Write(lvStr(""))
	root.Write(lvTag(10, "Data", data.Bytes()))
	root.WriteByte(0)

	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write(root.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return gz.Bytes()
}

// levelParseData 构造一次 /mc/level/parse 请求并返回响应 data 对象。
func levelParseData(t *testing.T, fileName string, content []byte) map[string]any {
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mc/level/parse", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	NewLevelHandler().Parse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return resp.Data
}

func TestLevelParseWorldInfo(t *testing.T) {
	data := levelParseData(t, "level.dat", lvLevelDat(t))

	world, ok := data["world"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 world 对象: %#v", data)
	}
	if world["level_name"] != "测试世界" {
		t.Errorf("LevelName 不符: %#v", world["level_name"])
	}
	if world["data_version"] != float64(3465) {
		t.Errorf("DataVersion 不符: %#v", world["data_version"])
	}
	if world["random_seed"] != "12345" {
		t.Errorf("RandomSeed 不符: %#v", world["random_seed"])
	}
	if world["spawn_z"] != float64(-3) {
		t.Errorf("SpawnZ 应为 -3: %#v", world["spawn_z"])
	}
	if world["allow_commands"] != true {
		t.Errorf("allow_commands 应为 true: %#v", world["allow_commands"])
	}
	ver, ok := world["version"].(map[string]any)
	if !ok || ver["name"] != "1.20.1" || ver["id"] != float64(3465) {
		t.Errorf("Version 不符: %#v", world["version"])
	}
	rules, ok := world["game_rules"].(map[string]any)
	if !ok || rules["doDaylightCycle"] != "true" {
		t.Errorf("GameRules 不符: %#v", world["game_rules"])
	}
	// 字段齐全时不应产生这些告警。
	warnings, _ := data["warnings"].([]any)
	for _, w := range warnings {
		wm := w.(map[string]any)
		switch wm["code"] {
		case "missing_data_version", "missing_seed", "missing_version":
			t.Errorf("字段齐全不应出现告警 %v", wm["code"])
		}
	}
}

func TestLevelParseRejectsNonLevel(t *testing.T) {
	// 顶层缺少 Data 复合标签 → 422。
	var root bytes.Buffer
	root.WriteByte(10)
	root.Write(lvStr(""))
	root.WriteByte(0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mc/level/parse", bytes.NewReader(root.Bytes()))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	NewLevelHandler().Parse(rec, req)
	// 并非 multipart，解析表单会失败 → 422（不 panic 即可）。
	if rec.Code == http.StatusOK {
		t.Fatalf("非 level.dat 请求不应返回 200：%s", rec.Body.String())
	}
}

func TestLevelParseInvalidNBT(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "level.dat")
	_, _ = fw.Write([]byte{0xFF, 0x00, 0x01})
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mc/level/parse", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	NewLevelHandler().Parse(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("非法 NBT 不应返回 200：%s", rec.Body.String())
	}
}

// TestBuildWorldInfoWarnings 直接校验诊断告警：缺字段 / 出生点越界。
func TestBuildWorldInfoWarnings(t *testing.T) {
	badSpawn := int32(500)
	wi, warnings := buildWorldInfo(map[string]any{
		"DataVersion": int32(0),
		"SpawnY":      badSpawn,
	})
	if wi.SpawnY == nil || *wi.SpawnY != 500 {
		t.Fatalf("SpawnY 未正确读取: %#v", wi.SpawnY)
	}
	codes := map[string]bool{}
	for _, w := range warnings {
		codes[w.Code] = true
	}
	for _, want := range []string{"missing_data_version", "missing_seed", "missing_version", "missing_last_played", "spawn_y_out_of_range"} {
		if !codes[want] {
			t.Errorf("应包含告警 %s，实际 %+v", want, warnings)
		}
	}
}
