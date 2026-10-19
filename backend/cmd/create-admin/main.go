// main 程序：初始化管理员账号。
// 用法：
//   ADMIN_USERNAME=admin ADMIN_PASSWORD=<强密码> go run ./cmd/create-admin
//
// 不从命令行参数读取密码（避免出现在 shell 历史），只从环境变量读取；
// 生产环境务必使用强随机密码，绝不提供固定默认密码（需求 §6.1）。
package main

import (
	"fmt"
	"os"
	"strings"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	db "mckit/backend/internal/repository"

	"gorm.io/gorm"
)

func main() {
	_ = envutil.LoadDotEnv()

	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := os.Getenv("ADMIN_PASSWORD")
	role := strings.TrimSpace(os.Getenv("ADMIN_ROLE"))
	if role == "" {
		role = domain.RoleAdmin
	}
	if role != domain.RoleAdmin && role != domain.RoleEditor {
		fmt.Fprintln(os.Stderr, "ADMIN_ROLE 只能是 admin 或 editor")
		os.Exit(1)
	}
	if username == "" {
		fmt.Fprintln(os.Stderr, "缺少 ADMIN_USERNAME")
		os.Exit(1)
	}
	if len(password) < 12 {
		fmt.Fprintln(os.Stderr, "ADMIN_PASSWORD 长度至少 12 位；禁止使用弱默认密码")
		os.Exit(1)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "缺少 DATABASE_URL")
		os.Exit(1)
	}

	gormDB, err := db.Open(databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库失败: %v\n", err)
		os.Exit(1)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "密码哈希失败: %v\n", err)
		os.Exit(1)
	}

	admin := &domain.Admin{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		IsActive:     true,
	}

	var existing domain.Admin
	err = gormDB.Where("username = ?", username).First(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		fmt.Fprintf(os.Stderr, "查询管理员失败: %v\n", err)
		os.Exit(1)
	}

	if err == nil {
		// 已存在则更新密码和角色（幂等重跑）。
		if err := gormDB.Model(&existing).Updates(map[string]any{
			"password_hash": hash,
			"role":          role,
			"is_active":     true,
		}).Error; err != nil {
			fmt.Fprintf(os.Stderr, "更新管理员失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已更新管理员 %s（role=%s）\n", username, role)
		return
	}

	if err := gormDB.Create(admin).Error; err != nil {
		fmt.Fprintf(os.Stderr, "创建管理员失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已创建管理员 %s（role=%s），ID=%s\n", username, role, admin.ID)
}