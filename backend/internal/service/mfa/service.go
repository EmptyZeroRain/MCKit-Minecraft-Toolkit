// Package mfa 提供后台账号 TOTP 双因子认证的持久化业务逻辑（gj.md §10.1 / §17 阶段一）。
//
// 生命周期：
//  1. Setup 生成 secret 并以「待启用」状态落库，返回 otpauth URI 供前端出示二维码；
//  2. Enable 用用户输入的验证码确认绑定，落库 enabled=true 并一次性返回恢复码明文；
//  3. 之后登录：密码校验通过后若已启用 MFA，则签发短期票据，要求第二因子；
//  4. Verify 校验 TOTP 或恢复码，成功后签发会话；
//  5. Disable / Reset 解绑（管理员重置用于账号恢复）。
//
// 安全取舍：
//   - secret 落库前 AES-256-GCM 加密，密钥只在部署侧（MFA_ENCRYPTION_KEY）；
//   - 恢复码只存 SHA-256 哈希，一次性使用；
//   - 票据只存 SHA-256 哈希、单次有效、5 分钟过期，避免被截获后重放；
//   - TOTP 同一时间步只接受一次（last_used_step），防止窗口内重放。
package mfa

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	mfacrypto "mckit/backend/internal/mfa"
)

// 业务错误。
var (
	// ErrNotConfigured 服务未配置加密密钥（部署未开启 MFA）。
	ErrNotConfigured = errors.New("MFA 未配置：请设置 MFA_ENCRYPTION_KEY")
	// ErrNotEnabled 账号未启用 MFA。
	ErrNotEnabled = errors.New("该账号未启用 MFA")
	// ErrAlreadyEnabled 账号已启用 MFA，需先解绑再重新绑定。
	ErrAlreadyEnabled = errors.New("该账号已启用 MFA")
	// ErrPending 已存在待启用的绑定，需先完成或放弃。
	ErrPending = errors.New("已存在待启用的 MFA 绑定")
	// ErrInvalidCode 验证码错误（TOTP 或恢复码均不匹配）。
	ErrInvalidCode = errors.New("验证码错误或已失效")
	// ErrTicketInvalid 登录票据无效、已使用或已过期。
	ErrTicketInvalid = errors.New("登录票据无效或已过期，请重新登录")
)

// TicketTTL 第二因子登录票据有效期。
const TicketTTL = 5 * time.Minute

// MaxTicketAttempts 单张票据允许的错误验证码次数。
//
// 一次密码认证最多换来 5 次 6 位验证码猜测，配合登录限流（5 次 / 15 分钟），
// 暴力枚举 TOTP 在实践中不可行；同时允许用户手滑重试几次。
const MaxTicketAttempts = 5

// Service MFA 业务服务。
type Service struct {
	db     *gorm.DB
	key    []byte
	issuer string
	// now 便于测试注入固定时间；默认 time.Now。
	now func() time.Time
}

// NewService 创建服务。key 为空表示 MFA 不可用（Setup/Enable 返回 ErrNotConfigured），
// 但已有绑定的账号仍可正常校验（key 为空时无法解密，会返回 ErrNotConfigured）。
func NewService(db *gorm.DB, key []byte, issuer string) *Service {
	if strings.TrimSpace(issuer) == "" {
		issuer = "MC 工具站"
	}
	return &Service{db: db, key: key, issuer: issuer, now: time.Now}
}

// Configured 是否已配置加密密钥。
func (s *Service) Configured() bool { return len(s.key) > 0 }

// SetupResult 绑定初始化结果。
type SetupResult struct {
	Secret string `json:"secret"`
	// URI otpauth:// 地址，前端据此渲染二维码（也可让用户手动输入 secret）。
	URI string `json:"uri"`
}

// Status 账号 MFA 状态（不含任何密钥材料）。
type Status struct {
	Enabled bool `json:"enabled"`
	// Pending 已生成 secret 但尚未完成首次验证。
	Pending bool `json:"pending"`
	// RecoveryCodesLeft 剩余可用恢复码数量。
	RecoveryCodesLeft int `json:"recovery_codes_left"`
	// LastUsedAt 最近一次通过 MFA 校验的时间。
	LastUsedAt *time.Time `json:"last_used_at"`
	// Configured 服务端是否已配置加密密钥（未配置时前端应禁用绑定入口）。
	Configured bool `json:"configured"`
}

// Status 查询绑定状态。
func (s *Service) Status(ctx context.Context, accountID uuid.UUID) (Status, error) {
	out := Status{Configured: s.Configured()}
	var row domain.AccountMFA
	err := s.db.WithContext(ctx).Where("account_id = ?", accountID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Enabled = row.Enabled
	out.Pending = !row.Enabled
	out.LastUsedAt = row.LastUsedAt

	var left int64
	if err := s.db.WithContext(ctx).Model(&domain.MFARecoveryCode{}).
		Where("account_id = ? AND used_at IS NULL", accountID).
		Count(&left).Error; err != nil {
		return out, err
	}
	out.RecoveryCodesLeft = int(left)
	return out, nil
}

// IsEnabled 账号是否已启用 MFA（登录流程使用）。
func (s *Service) IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var enabled bool
	err := s.db.WithContext(ctx).Model(&domain.AccountMFA{}).
		Select("enabled").
		Where("account_id = ?", accountID).
		Scan(&enabled).Error
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// Setup 生成新的 secret 并以「待启用」状态保存，返回 otpauth URI。
//
// 重复调用会覆盖尚未启用的旧 secret（用户放弃后重新扫码的场景）；
// 已启用时返回 ErrAlreadyEnabled，必须显式解绑。
func (s *Service) Setup(ctx context.Context, accountID uuid.UUID, username string) (*SetupResult, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	var existing domain.AccountMFA
	err := s.db.WithContext(ctx).Where("account_id = ?", accountID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && existing.Enabled {
		return nil, ErrAlreadyEnabled
	}

	secret, err := mfacrypto.GenerateSecret()
	if err != nil {
		return nil, err
	}
	enc, err := mfacrypto.Encrypt(s.key, secret)
	if err != nil {
		return nil, err
	}

	row := domain.AccountMFA{
		AccountID: accountID,
		SecretEnc: enc,
		Enabled:   false,
		CreatedAt: s.now(),
	}
	// upsert：覆盖待启用记录，重置防重放计数。
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}

	return &SetupResult{
		Secret: secret,
		URI:    mfacrypto.ProvisioningURI(s.issuer, username, secret),
	}, nil
}

// Enable 校验首次验证码并激活绑定，返回一次性恢复码明文（仅此一次）。
func (s *Service) Enable(ctx context.Context, accountID uuid.UUID, code string) ([]string, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	var row domain.AccountMFA
	if err := s.db.WithContext(ctx).Where("account_id = ?", accountID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotEnabled
		}
		return nil, err
	}
	if row.Enabled {
		return nil, ErrAlreadyEnabled
	}

	secret, err := mfacrypto.Decrypt(s.key, row.SecretEnc)
	if err != nil {
		return nil, err
	}
	step, ok := mfacrypto.Validate(secret, code, s.now())
	if !ok {
		return nil, ErrInvalidCode
	}

	codes, err := mfacrypto.GenerateRecoveryCodes(mfacrypto.RecoveryCodeCount)
	if err != nil {
		return nil, err
	}

	now := s.now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"enabled":        true,
			"enabled_at":     now,
			"last_used_at":   now,
			"last_used_step": int64(step),
		}
		if err := tx.Model(&domain.AccountMFA{}).
			Where("account_id = ?", accountID).
			Updates(updates).Error; err != nil {
			return err
		}
		// 重新绑定：清空旧恢复码，避免历史码继续可用。
		if err := tx.Where("account_id = ?", accountID).
			Delete(&domain.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		rows := make([]domain.MFARecoveryCode, 0, len(codes))
		for _, c := range codes {
			rows = append(rows, domain.MFARecoveryCode{
				AccountID: accountID,
				CodeHash:  mfacrypto.HashRecoveryCode(c),
				CreatedAt: now,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable 解绑 MFA（同时清除恢复码）。用户自助解绑须先通过密码 + 当前验证码校验
// （由 handler 完成），此处只做数据清理。
func (s *Service) Disable(ctx context.Context, accountID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", accountID).
			Delete(&domain.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		return tx.Where("account_id = ?", accountID).
			Delete(&domain.AccountMFA{}).Error
	})
}

// Verify 校验 TOTP 或一次性恢复码。
//
// 校验成功会更新 last_used_at；TOTP 额外校验时间步单调递增（防重放），
// 恢复码则标记 used_at 使其立即失效。
func (s *Service) Verify(ctx context.Context, accountID uuid.UUID, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrInvalidCode
	}

	var row domain.AccountMFA
	if err := s.db.WithContext(ctx).Where("account_id = ?", accountID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotEnabled
		}
		return err
	}
	if !row.Enabled {
		return ErrNotEnabled
	}
	if !s.Configured() {
		return ErrNotConfigured
	}

	secret, err := mfacrypto.Decrypt(s.key, row.SecretEnc)
	if err != nil {
		return err
	}

	// 6 位数字按 TOTP 校验；其余长度按恢复码处理。
	if len(code) == mfacrypto.Digits && isDigits(code) {
		step, ok := mfacrypto.Validate(secret, code, s.now())
		if !ok {
			return ErrInvalidCode
		}
		if int64(step) <= row.LastUsedStep {
			// 同一（或更早）时间步的验证码已被使用过。
			return ErrInvalidCode
		}
		now := s.now()
		res := s.db.WithContext(ctx).Model(&domain.AccountMFA{}).
			Where("account_id = ? AND last_used_step < ?", accountID, int64(step)).
			Updates(map[string]any{
				"last_used_step": int64(step),
				"last_used_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 并发请求已消费该时间步。
			return ErrInvalidCode
		}
		return nil
	}

	// 恢复码：归一化后哈希匹配未使用的记录，并原子标记已用。
	hash := mfacrypto.HashRecoveryCode(code)
	now := s.now()
	res := s.db.WithContext(ctx).Model(&domain.MFARecoveryCode{}).
		Where("account_id = ? AND code_hash = ? AND used_at IS NULL", accountID, hash).
		Update("used_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInvalidCode
	}
	_ = s.db.WithContext(ctx).Model(&domain.AccountMFA{}).
		Where("account_id = ?", accountID).
		Update("last_used_at", now).Error
	return nil
}

// CreateTicket 为已通过密码校验的账号签发第二因子票据，返回票据明文。
func (s *Service) CreateTicket(ctx context.Context, accountID uuid.UUID) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	row := domain.MFALoginTicket{
		TokenHash: hashToken(token),
		AccountID: accountID,
		ExpiresAt: now.Add(TicketTTL),
		CreatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	// 顺手清理过期票据：量极小且无需独立后台任务。
	_ = s.db.WithContext(ctx).
		Where("expires_at < ?", now.Add(-time.Hour)).
		Delete(&domain.MFALoginTicket{}).Error
	return token, nil
}

// RedeemTicket 校验第二因子并消费票据，返回账号 ID。
//
// 验证码错误时票据保留并累计 attempts，达到 MaxTicketAttempts 后作废；
// 成功时原子标记 used_at，保证同一票据只能兑换一次会话。
func (s *Service) RedeemTicket(ctx context.Context, token, code string) (uuid.UUID, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return uuid.Nil, ErrTicketInvalid
	}
	hash := hashToken(token)
	now := s.now()

	var row domain.MFALoginTicket
	err := s.db.WithContext(ctx).
		Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", hash, now).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, ErrTicketInvalid
		}
		return uuid.Nil, err
	}
	if row.Attempts >= MaxTicketAttempts {
		return uuid.Nil, ErrTicketInvalid
	}

	if err := s.Verify(ctx, row.AccountID, code); err != nil {
		if errors.Is(err, ErrInvalidCode) {
			// 累计失败次数；达到上限直接作废，避免无限重试。
			next := row.Attempts + 1
			updates := map[string]any{"attempts": next}
			if next >= MaxTicketAttempts {
				updates["used_at"] = now
			}
			_ = s.db.WithContext(ctx).Model(&domain.MFALoginTicket{}).
				Where("token_hash = ?", hash).
				Updates(updates).Error
		}
		return uuid.Nil, err
	}

	res := s.db.WithContext(ctx).Model(&domain.MFALoginTicket{}).
		Where("token_hash = ? AND used_at IS NULL", hash).
		Update("used_at", now)
	if res.Error != nil {
		return uuid.Nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 并发请求已兑换同一票据。
		return uuid.Nil, ErrTicketInvalid
	}
	return row.AccountID, nil
}

// PurgeExpiredTickets 清理过期票据（供运维 / 测试调用）。
func (s *Service) PurgeExpiredTickets(ctx context.Context) (int64, error) {
	res := s.db.WithContext(ctx).
		Where("expires_at < ?", s.now()).
		Delete(&domain.MFALoginTicket{})
	return res.RowsAffected, res.Error
}

// Issuer 供文档 / 调试使用。
func (s *Service) Issuer() string { return s.issuer }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// randomToken 生成 32 字节随机票据（base64url 无填充）。
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
