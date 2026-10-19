package mcversion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestServer 启动一个模拟 Mojang API 的 httptest 服务，
// 并返回指向其缓存的 Client。
func newTestServer(t *testing.T) (*Client, *httptest.Server, string) {
	t.Helper()

	manifest := `{
		"latest": {"release": "1.21.8", "snapshot": "25w07a"},
		"versions": [
			{"id": "25w07a", "type": "snapshot", "url": "%s/25w07a.json", "releaseTime": "2026-02-12T13:24:08+00:00"},
			{"id": "1.21.8", "type": "release", "url": "%s/1.21.8.json", "releaseTime": "2026-01-15T10:00:00+00:00"},
			{"id": "b1.7.3", "type": "old_beta", "url": "%s/b1.7.3.json", "releaseTime": "2011-07-08T00:00:00+00:00"}
		]
	}`

	versionJSON := `{
		"id": "25w07a",
		"type": "snapshot",
		"mainClass": "net.minecraft.client.main.Main",
		"javaVersion": {"component": "java-runtime-delta", "majorVersion": 21},
		"downloads": {
			"client": {"sha1": "aaa", "size": 100, "url": "https://piston-data.mojang.com/client.jar"},
			"client_mappings": {"sha1": "bbb", "size": 200, "url": "https://piston-data.mojang.com/client.txt"},
			"server": {"sha1": "ccc", "size": 300, "url": "https://piston-data.mojang.com/server.jar"},
			"server_mappings": {"sha1": "ddd", "size": 400, "url": "https://piston-data.mojang.com/server.txt"}
		},
		"libraries": [
			{
				"name": "org.lwjgl:lwjgl:3.3.1",
				"downloads": {
					"artifact": {"sha1": "l1", "size": 1000, "url": "https://libraries.minecraft.net/lwjgl-3.3.1.jar"},
					"classifiers": {
						"natives-macos": {"sha1": "l1m", "size": 1100, "url": "https://libraries.minecraft.net/lwjgl-3.3.1-macos.jar"},
						"javadoc": {"sha1": "l1j", "size": 1200, "url": "https://libraries.minecraft.net/lwjgl-3.3.1-javadoc.jar"}
					}
				}
			},
			{
				"name": "ca.weblite:java-objc-bridge:1.1",
				"downloads": {"artifact": {"sha1": "l2", "size": 2000, "url": "https://libraries.minecraft.net/java-objc-bridge-1.1.jar"}},
				"rules": [{"action": "allow", "os": {"name": "osx"}}]
			}
		],
		"arguments": {
			"game": [
				"--username",
				"${auth_player_name}",
				"--version",
				"${version_name}",
				"--fullscreenWidth",
				"${resolution_width}",
				{"rules": [{"action": "allow", "os": {"name": "osx"}}], "value": "-XstartOnFirstThread"},
				{"rules": [{"action": "allow", "features": {"is_demo_user": true}}], "value": "--demo"}
			],
			"jvm": [
				"-Djava.library.path=${natives_directory}",
				"-cp",
				"${classpath}",
				{"rules": [{"action": "allow", "os": {"name": "windows"}}], "value": "-XX:HeapDumpPath=unused"}
			],
			"default-user-jvm": [
				{"value": ["-Xms2G", "-Xmx4G", "-XX:+UseZGC"]},
				{"rules": [{"action": "allow", "os": {"name": "osx"}}], "value": ["-XstartOnFirstThread"]}
			]
		}
	}`

	// 旧版本：无 mappings、无 server 下载，且无 javaVersion 字段；
	// library 为旧式裸 url 基址格式，启动参数为 1.13 前的字符串形式。
	legacyJSON := `{
		"id": "b1.7.3",
		"type": "old_beta",
		"mainClass": "Minecraft",
		"minecraftArguments": "${auth_player_name} ${auth_session} --gameDir ${game_directory} --assetsDir ${assets_root}",
		"downloads": {
			"client": {"sha1": "eee", "size": 500, "url": "https://piston-data.mojang.com/legacy.jar"}
		},
		"libraries": [
			{
				"name": "net.java.jinput:jinput:2.0.5",
				"url": "https://libraries.minecraft.net/",
				"natives": {"linux": "natives-linux", "windows": "natives-windows", "osx": "natives-osx"}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mc/game/version_manifest.json":
			w.Write([]byte(strings.ReplaceAll(manifest, "%s", "http://"+r.Host+"/version")))
		case "/version/25w07a.json":
			w.Write([]byte(versionJSON))
		case "/version/b1.7.3.json":
			w.Write([]byte(legacyJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	cacheDir := t.TempDir()
	c := New(cacheDir, time.Hour).WithHTTP(srv.Client()).WithManifestURL(srv.URL + "/mc/game/version_manifest.json")
	return c, srv, cacheDir
}

func TestFetchManifest(t *testing.T) {
	c, _, _ := newTestServer(t)
	m, err := c.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest 失败: %v", err)
	}
	if len(m.Versions) != 3 {
		t.Fatalf("应有 3 个版本，实际 %d", len(m.Versions))
	}
	if m.Latest.Snapshot != "25w07a" {
		t.Fatalf("latest.snapshot 不符: %s", m.Latest.Snapshot)
	}
}

func TestFetchVersion(t *testing.T) {
	c, _, _ := newTestServer(t)
	ctx := context.Background()

	d, err := c.FetchVersion(ctx, "25w07a")
	if err != nil {
		t.Fatalf("FetchVersion 失败: %v", err)
	}
	if d.JavaVersion == nil || d.JavaVersion.MajorVersion != 21 {
		t.Fatalf("javaVersion.majorVersion 应为 21: %+v", d.JavaVersion)
	}
	if len(d.Downloads) != 4 {
		t.Fatalf("应有 4 个下载项，实际 %d", len(d.Downloads))
	}
	if len(d.Libraries) != 2 {
		t.Fatalf("应有 2 个库，实际 %d", len(d.Libraries))
	}
	if d.Libraries[0].Downloads == nil || d.Libraries[0].Downloads.Artifact == nil {
		t.Fatal("第一个库应有 artifact 下载信息")
	}
	if len(d.Libraries[0].Downloads.Classifiers) != 2 {
		t.Fatalf("第一个库应有 2 个 classifier，实际 %d", len(d.Libraries[0].Downloads.Classifiers))
	}

	// 旧版本缺字段时不应崩溃。
	legacy, err := c.FetchVersion(ctx, "b1.7.3")
	if err != nil {
		t.Fatalf("旧版本获取失败: %v", err)
	}
	if legacy.JavaVersion != nil {
		t.Fatal("旧版本不应有 javaVersion")
	}
	if _, ok := legacy.Downloads["server"]; ok {
		t.Fatal("旧版本不应有 server 下载")
	}
}

func TestFetchVersionNotFound(t *testing.T) {
	c, _, _ := newTestServer(t)
	if _, err := c.FetchVersion(context.Background(), "not-exist"); err == nil {
		t.Fatal("不存在的版本应报错")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	// 两次请求间关闭模拟服务器，第二次必须命中缓存。
	c, srv, cacheDir := newTestServer(t)
	ctx := context.Background()
	if _, err := c.FetchVersion(ctx, "25w07a"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if _, err := c.FetchVersion(ctx, "25w07a"); err != nil {
		t.Fatalf("缓存命中后服务关闭也应成功: %v", err)
	}

	// 磁盘上应真实存在缓存文件。
	if _, err := os.Stat(filepath.Join(cacheDir, "versions", "25w07a.json")); err != nil {
		t.Fatalf("缓存文件应落盘: %v", err)
	}
}

func TestBuildDetail(t *testing.T) {
	c, _, _ := newTestServer(t)
	d, err := c.FetchVersion(context.Background(), "25w07a")
	if err != nil {
		t.Fatal(err)
	}
	dto := BuildDetail(d)
	if dto.Java.MajorVersion != 21 {
		t.Fatalf("Java 主版本应为 21，实际 %d", dto.Java.MajorVersion)
	}
	if len(dto.Downloads) != 4 {
		t.Fatalf("应有 4 个下载项，实际 %d", len(dto.Downloads))
	}
	if dto.StartupCommand == "" {
		t.Fatal("有 server jar 时应生成服务端启动命令")
	}
	if dto.ClientLaunch == nil || dto.ClientLaunch.Command == "" {
		t.Fatal("应生成客户端启动命令")
	}
	// 带平台限制的参数（-XstartOnFirstThread）不应进入通用命令。
	if strings.Contains(dto.ClientLaunch.Command, "-XstartOnFirstThread") {
		t.Fatalf("平台限定参数不应进入通用命令: %s", dto.ClientLaunch.Command)
	}
	// default-user-jvm 的全平台参数应拼入命令。
	if !strings.Contains(dto.ClientLaunch.Command, "-XX:+UseZGC") {
		t.Fatalf("default-user-jvm 全平台参数应进入命令: %s", dto.ClientLaunch.Command)
	}

	// 旧版本：无 javaVersion 默认 Java 8，无 server 下载则无服务端启动命令。
	legacy, err := c.FetchVersion(context.Background(), "b1.7.3")
	if err != nil {
		t.Fatal(err)
	}
	ldto := BuildDetail(legacy)
	if ldto.Java.MajorVersion != 8 {
		t.Fatalf("旧版本 Java 主版本应默认 8，实际 %d", ldto.Java.MajorVersion)
	}
	if ldto.StartupCommand != "" {
		t.Fatalf("无 server jar 不应有服务端启动命令: %s", ldto.StartupCommand)
	}
	if ldto.ClientLaunch == nil {
		t.Fatal("旧版本也应生成客户端启动命令")
	}
}
