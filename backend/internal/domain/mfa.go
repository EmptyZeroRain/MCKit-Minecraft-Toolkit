package domain

import (
	"time"

	"github.com/google/uuid"
)

// AccountMFA 账号的 TOTP 双因子绑定（迁移 0032）。
//
// SecretEnc 存 AES-256-GCM 密文，密钥来自部署侧 MFA_ENCRYPTION_KEY；
// LastUsedStep 用于防重放：同一时间步的验证码只接受一次，
// 避免攻击者截获验证码后在有效窗口内重复使用。
type AccountMFA struct {
	AccountID uuid.UUID `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	SecretEnc string    `gorm:"column:secret_enc;not null" json:"-"`
	// Enabled 为 false 表示已生成 secret 但尚未完成首次验证，登录不要求第二因子。
	Enabled      bool       `gorm:"not null;default:false" json:"enabled"`
	EnabledAt    *time.Time `gorm:"column:enabled_at" json:"enabled_at"`
	LastUsedAt   *time.Time
	LastUsedStep int64     `gorm:"column:last_used_step;not null;default:0" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName 指定表名。
func (AccountMFA) TableName() string { return "account_mfa" }

// MFARecoveryCode 一次性恢复码（只存 SHA-256 哈希）。
type MFARecoveryCode struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID uuid.UUID  `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	CodeHash  string     `gorm:"column:code_hash;size:64;not null" json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName 指定表名。
func (MFARecoveryCode) TableName() string { return "account_mfa_recovery_codes" }

// MFALoginTicket 「密码已通过、等待第二因子」的短期票据（只存哈希）。
type MFALoginTicket struct {
	TokenHash string    `gorm:"column:token_hash;size:64;primaryKey" json:"-"`
	AccountID uuid.UUID `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null" json:"expires_at"`
	// Attempts 已消耗的错误验证码次数（达到上限后票据作废）。
	Attempts  int `gorm:"not null;default:0" json:"-"`
	UsedAt    *time.Time
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (MFALoginTicket) TableName() string { return "mfa_login_tickets" }
