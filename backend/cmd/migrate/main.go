// main 程序：执行数据库迁移（支持前进与回退）。
//
// 用法：
//
//	go run ./cmd/migrate                 # 应用全部未执行的正向迁移
//	go run ./cmd/migrate --to 20         # 迁移到指定版本（可前进或回退）
//	go run ./cmd/migrate --down 1        # 回退最近 1 个已应用迁移
//	go run ./cmd/migrate --dir migrations
//	go run ./cmd/migrate migrations      # 兼容旧的「位置参数指定目录」写法
//
// 回退依赖与正向文件同名的 <序号>_<名称>.down.sql；
// 缺失反向文件的版本不可自动回退（会明确报错并停止）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"mckit/backend/internal/envutil"
	db "mckit/backend/internal/repository"
)

func main() {
	dir := flag.String("dir", "migrations", "迁移目录")
	to := flag.Int64("to", -1, "迁移到指定版本（可前进或回退）")
	down := flag.Int("down", 0, "回退最近 N 个已应用迁移")
	flag.Parse()

	// 兼容旧写法：位置参数指定迁移目录。
	migrationsDir := *dir
	if flag.NArg() > 0 && flag.Arg(0) != "" {
		migrationsDir = flag.Arg(0)
	}
	if *to >= 0 && *down > 0 {
		log.Fatal("--to 与 --down 不能同时使用")
	}

	ctx := context.Background()
	_ = envutil.LoadDotEnv()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("缺少必需配置 DATABASE_URL，请在环境或 .env 中设置")
	}

	gormDB, err := db.Open(databaseURL)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("获取底层连接失败: %v", err)
	}
	defer sqlDB.Close()

	var executed []string
	switch {
	case *down > 0:
		executed, err = db.Rollback(ctx, gormDB, migrationsDir, *down)
		if err != nil {
			log.Fatalf("回退失败: %v", err)
		}
		if len(executed) == 0 {
			fmt.Println("没有需要回退的迁移")
			return
		}
		for _, name := range executed {
			fmt.Printf("已回退迁移: %s\n", name)
		}
		fmt.Printf("回退完成，共 %d 个文件\n", len(executed))
	case *to >= 0:
		executed, err = db.MigrateTo(ctx, gormDB, migrationsDir, *to)
		if err != nil {
			log.Fatalf("迁移失败: %v", err)
		}
		if len(executed) == 0 {
			fmt.Printf("数据库已处于版本 %d，无需变更\n", *to)
			return
		}
		for _, name := range executed {
			fmt.Printf("已执行迁移: %s\n", name)
		}
		fmt.Printf("迁移完成，共 %d 个文件，当前目标版本 %d\n", len(executed), *to)
	default:
		executed, err = db.Migrate(ctx, gormDB, migrationsDir)
		if err != nil {
			log.Fatalf("迁移失败: %v", err)
		}
		if len(executed) == 0 {
			fmt.Println("无需迁移，数据库已是最新状态")
			return
		}
		for _, name := range executed {
			fmt.Printf("已应用迁移: %s\n", name)
		}
		fmt.Printf("迁移完成，共 %d 个文件\n", len(executed))
	}
}
