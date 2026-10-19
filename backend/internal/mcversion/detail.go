package mcversion

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DownloadDTO 对外暴露的下载信息。
type DownloadDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
	SHA1 string `json:"sha1"`
}

// JavaDTO Java 版本要求。
type JavaDTO struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

// NativeDTO 库的平台原生变体（如 natives-macos）。
type NativeDTO struct {
	Classifier string `json:"classifier"`
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA1       string `json:"sha1"`
}

// LibraryDTO 运行库信息。
type LibraryDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
	SHA1 string `json:"sha1"`
	// Platforms 适用平台说明，空字符串表示全平台。
	Platforms string      `json:"platforms,omitempty"`
	Natives   []NativeDTO `json:"natives,omitempty"`
}

// ArgDTO 单个启动参数，Platforms 非空表示仅该平台需要。
type ArgDTO struct {
	Value     string `json:"value"`
	Platforms string `json:"platforms,omitempty"`
}

// ClientLaunchDTO 客户端完整启动命令模板。
// Command 中的 ${...} 占位符（用户名、资源目录、classpath 等）
// 由启动器在实际运行时替换。
type ClientLaunchDTO struct {
	MainClass string   `json:"mainClass"`
	Command   string   `json:"command"`
	GameArgs  []ArgDTO `json:"gameArgs"`
	JVMArgs   []ArgDTO `json:"jvmArgs"`
	// DefaultUserJVM 官方启动器的默认用户 JVM 参数（仅新版快照携带），
	// 已按全平台部分拼入 Command，平台变体仅在明细中展示。
	DefaultUserJVM []ArgDTO `json:"defaultUserJVM,omitempty"`
}

// DetailDTO 版本详情响应体，聚合了需求要求的四类信息：
// 下载地址、启动命令、Java 版本要求、版本基本元数据。
type DetailDTO struct {
	ID             string        `json:"id"`
	Type           string        `json:"type"`
	ReleaseTime    string        `json:"releaseTime"`
	MainClass      string        `json:"mainClass"`
	Java           JavaDTO       `json:"java"`
	Downloads      []DownloadDTO `json:"downloads"`
	Libraries      []LibraryDTO  `json:"libraries"`
	StartupCommand string        `json:"startupCommand"`
	// StartupNotes 启动前置条件说明（如需要的 Java 主版本）。
	StartupNotes []string `json:"startupNotes"`
	// ClientLaunch 客户端完整启动命令（1.13 前后取参逻辑不同）。
	ClientLaunch *ClientLaunchDTO `json:"clientLaunch,omitempty"`
}

// BuildDetail 将 mojang 原始详情转换为对外 DTO，
// 并生成启动命令与 Java 要求说明。
func BuildDetail(d *VersionDetail) DetailDTO {
	// 下载文件按固定顺序输出，旧版本缺失的条目自动跳过。
	nameOrder := []string{"client", "client_mappings", "server", "server_mappings"}
	names := map[string]string{
		"client":          "客户端 jar",
		"client_mappings": "客户端 mappings",
		"server":          "服务端 jar",
		"server_mappings": "服务端 mappings",
	}

	dto := DetailDTO{
		ID:          d.ID,
		Type:        d.Type,
		ReleaseTime: d.ReleaseTime.Format("2006-01-02"),
		MainClass:   d.MainClass,
		Downloads:   make([]DownloadDTO, 0, len(nameOrder)),
	}

	hasServer := false
	for _, key := range nameOrder {
		info, ok := d.Downloads[key]
		if !ok {
			continue
		}
		if key == "server" {
			hasServer = true
		}
		dto.Downloads = append(dto.Downloads, DownloadDTO{
			Name: names[key],
			URL:  info.URL,
			Size: info.Size,
			SHA1: info.SHA1,
		})
	}

	// Java 版本要求：早期版本 piston-meta 没有 javaVersion 字段，默认 Java 8。
	major := 8
	if d.JavaVersion != nil {
		major = d.JavaVersion.MajorVersion
		dto.Java.Component = d.JavaVersion.Component
	}
	dto.Java.MajorVersion = major
	dto.StartupNotes = append(dto.StartupNotes, "需要 Java "+strconv.Itoa(major)+" 或更高版本")

	// 运行库：主 artifact + natives 变体，旧格式用 Maven 坐标兜底拼接。
	dto.Libraries = make([]LibraryDTO, 0, len(d.Libraries))
	for _, lib := range d.Libraries {
		item := LibraryDTO{
			Name:      lib.Name,
			Platforms: argPlatforms(lib.Rules),
		}
		if lib.Downloads != nil && lib.Downloads.Artifact != nil {
			a := lib.Downloads.Artifact
			item.URL, item.Size, item.SHA1 = a.URL, a.Size, a.SHA1
			for cls, info := range lib.Downloads.Classifiers {
				// 只输出 natives 变体，跳过其他 classifier 类型。
				if info != nil && strings.HasPrefix(cls, "natives-") {
					item.Natives = append(item.Natives, NativeDTO{
						Classifier: cls,
						URL:        info.URL,
						Size:       info.Size,
						SHA1:       info.SHA1,
					})
				}
			}
			sort.Slice(item.Natives, func(i, j int) bool {
				return item.Natives[i].Classifier < item.Natives[j].Classifier
			})
		} else {
			// 旧版清单无 downloads，按 Maven 坐标从基址拼下载路径。
			base := lib.BaseURL
			if base == "" {
				base = "https://libraries.minecraft.net/"
			}
			if p, err := mavenJarPath(lib.Name); err == nil {
				item.URL = strings.TrimSuffix(base, "/") + "/" + p
			}
		}
		dto.Libraries = append(dto.Libraries, item)
	}

	// 启动命令仅对提供 server jar 的版本有意义。
	if hasServer {
		dto.StartupCommand = "java -Xmx1024M -Xms1024M -jar server.jar nogui"
	}

	// 客户端启动命令：mainClass 存在即可生成。
	if d.MainClass != "" {
		dto.ClientLaunch = buildClientLaunch(d)
	}
	return dto
}

// buildClientLaunch 生成客户端启动命令模板。
// 参数来源分支（以 arguments.game 是否存在为准，与 1.13 版本分界一致）：
//   - 1.13+：读 arguments.game / arguments.jvm，元素含平台 rules
//   - 1.13-：读 minecraftArguments 字符串；jvm 参数 JSON 中不存在，
//     使用启动器通用模板（内存 + natives 目录 + classpath）
//
// 带平台限制的参数不进 Command（避免复制出错误平台命令），仅在明细中标注。
func buildClientLaunch(d *VersionDetail) *ClientLaunchDTO {
	dto := &ClientLaunchDTO{MainClass: d.MainClass}

	var gameParts, jvmParts []string
	if d.Arguments != nil && len(d.Arguments.Game) > 0 {
		dto.GameArgs = flattenArgs(d.Arguments.Game)
		for _, a := range dto.GameArgs {
			if a.Platforms == "" {
				gameParts = append(gameParts, a.Value)
			}
		}
	} else if d.MinecraftArguments != "" {
		for _, v := range strings.Fields(d.MinecraftArguments) {
			dto.GameArgs = append(dto.GameArgs, ArgDTO{Value: v})
		}
		dto.GameArgs = mergeFlagValue(dto.GameArgs)
		for _, a := range dto.GameArgs {
			gameParts = append(gameParts, a.Value)
		}
	}

	if d.Arguments != nil {
		// default-user-jvm 在前：官方默认调优参数先于标准 jvm 参数出现，
		// 且标准参数末尾的 -cp ${classpath} 必须紧邻 mainClass。
		if len(d.Arguments.DefaultUserJVM) > 0 {
			dto.DefaultUserJVM = flattenArgs(d.Arguments.DefaultUserJVM)
			for _, a := range dto.DefaultUserJVM {
				if a.Platforms == "" {
					jvmParts = append(jvmParts, a.Value)
				}
			}
		}
		if len(d.Arguments.JVM) > 0 {
			dto.JVMArgs = flattenArgs(d.Arguments.JVM)
			for _, a := range dto.JVMArgs {
				if a.Platforms == "" {
					jvmParts = append(jvmParts, a.Value)
				}
			}
		}
	}
	// 1.13- 无结构化 JVM 参数，采用启动器通用模板。
	if dto.JVMArgs == nil {
		dto.JVMArgs = []ArgDTO{
			{Value: "-Xmx1024M"},
			{Value: "-Xms1024M"},
			{Value: "-Djava.library.path=${natives_directory}"},
			{Value: "-cp"},
			{Value: "${classpath}"},
		}
		jvmParts = []string{"-Xmx1024M", "-Xms1024M", "-Djava.library.path=${natives_directory}", "-cp", "${classpath}"}
	}

	// 防止 gameParts 为空时命令尾部出现悬挂空格。
	cmd := "java " + strings.Join(jvmParts, " ")
	if len(gameParts) > 0 {
		cmd += " " + d.MainClass + " " + strings.Join(gameParts, " ")
	} else {
		cmd += " " + d.MainClass
	}
	dto.Command = cmd
	return dto
}

// flattenArgs 将混合参数数组转成带平台标注的明细列表，
// 并对相邻的 "flag + 值"（如 --username 与 ${auth_player_name}）做配对合并。
func flattenArgs(args []GameArgument) []ArgDTO {
	raw := make([]ArgDTO, 0, len(args))
	for _, a := range args {
		p := argPlatforms(a.Rules)
		for _, v := range a.Values {
			raw = append(raw, ArgDTO{Value: v, Platforms: p})
		}
	}
	return mergeFlagValue(raw)
}

// mergeFlagValue 将相邻的 "flag + 值" 配对为一条。
// 值判定为紧随 flag 后不以 - 开头的元素（MC 启动参数中的值均为占位符或路径）。
func mergeFlagValue(in []ArgDTO) []ArgDTO {
	out := make([]ArgDTO, 0, len(in))
	for i := 0; i < len(in); i++ {
		cur := in[i]
		if strings.HasPrefix(cur.Value, "-") && i+1 < len(in) && !strings.HasPrefix(in[i+1].Value, "-") {
			cur.Value += " " + in[i+1].Value
			i++
		}
		out = append(out, cur)
	}
	return out
}

// mavenJarPath 将 Maven 坐标 "group:artifact:version[:classifier]"
// 转为相对 jar 路径，如 "net/sf/jopt-simple/jopt-simple/4.5/jopt-simple-4.5.jar"。
func mavenJarPath(coord string) (string, error) {
	parts := strings.Split(coord, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return "", fmt.Errorf("非法 Maven 坐标: %s", coord)
	}
	group, artifact, version := parts[0], parts[1], parts[2]
	if group == "" || artifact == "" || version == "" {
		return "", fmt.Errorf("非法 Maven 坐标: %s", coord)
	}
	file := artifact + "-" + version
	if len(parts) == 4 && parts[3] != "" {
		file += "-" + parts[3]
	}
	return strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + file + ".jar", nil
}

// argPlatforms 将 rules 转为人类可读的平台说明（库与启动参数通用）。
// 无 rules 视为全平台；存在 allow+os 限制时列出对应平台。
func argPlatforms(rules []LibraryRule) string {
	if len(rules) == 0 {
		return ""
	}
	osNames := map[string]string{
		"osx":     "macOS",
		"windows": "Windows",
		"linux":   "Linux",
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if r.Action == "allow" && r.OS != nil && osNames[r.OS.Name] != "" {
			seen[r.OS.Name] = true
		}
	}
	if len(seen) == 0 {
		return ""
	}
	var names []string
	for _, os := range []string{"osx", "windows", "linux"} {
		if seen[os] {
			names = append(names, osNames[os])
		}
	}
	return "仅 " + strings.Join(names, " / ")
}
