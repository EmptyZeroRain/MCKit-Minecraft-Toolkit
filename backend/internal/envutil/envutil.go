// Package envutil 提供 .env 文件的统一加载辅助。
// 仅在文件存在时加载，避免生产容器缺文件时报错。
package envutil

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadDotEnv 尝试从工作目录向上查找 .env 并加载。
// 找不到文件时不报错（生产环境直接注入环境变量）。
func LoadDotEnv() error {
	dir, err := os.Getwd()
	if err != nil {
		return nil
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}