// main 程序：执行数据库迁移。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"mckit/backend/internal/envutil"
	db "mckit/backend/internal/repository"
)

func main() {
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

	migrationsDir := "migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	executed, err := db.Migrate(ctx, gormDB, migrationsDir)
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