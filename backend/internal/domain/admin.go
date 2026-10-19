// Package domain 定义数据库实体模型（GORM）。
package domain

// 内置角色常量（角色定义见 roles 表，迁移 0021；统一账号见 account.go）。
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
)
