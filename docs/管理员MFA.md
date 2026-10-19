# 管理员双因子认证（MFA）

对应实现：`backend/internal/mfa`（纯算法）、`backend/internal/service/mfa`（持久化）、
`backend/internal/http/handler/mfa.go`（HTTP）、`backend/migrations/0032_account_mfa.sql`。

---

## 1. 为什么做

后台账号可以改内容、删账号、清理缓存，一旦密码泄露影响面远大于普通用户。
仅靠密码 + 登录限流不足以抵御：撞库、密码复用、钓鱼、终端被控。

TOTP（RFC 6238）是成本最低、兼容性最好的第二因子：不依赖短信网关，
不向第三方暴露任何信息，密钥只存在我们自己的数据库（加密后）和用户手机里。

---

## 2. 协议与参数

| 项 | 取值 | 说明 |
|---|---|---|
| 算法 | HMAC-SHA1 | 兼容性最广；安全性由 160 bit 密钥保证，与摘要算法弱相关 |
| 位数 | 6 | |
| 时间步 | 30 秒 | |
| 校验窗口 | ±1 步（±30 秒） | 容忍客户端时钟漂移 |
| 密钥长度 | 20 字节（base32 无填充） | RFC 4226 推荐 |
| 恢复码 | 10 个，每个 10 字符 | 去掉易混淆字符（无 0/O、1/I/L） |
| 票据有效期 | 5 分钟 | 密码通过后、第二因子提交前 |
| 票据重试 | 5 次 | 达到上限票据作废 |

TOTP 实现有 RFC 4226 标准测试向量覆盖：`internal/mfa/totp_test.go`。

---

## 3. 密钥与数据存储

### 3.1 加密密钥

- 环境变量 `MFA_ENCRYPTION_KEY`，16 / 24 / 32 字节的 base64 或 hex。
- 生成：`openssl rand -base64 32`。
- 格式错误会让进程**启动失败**（`config.Validate`），而不是等到用户绑定时才报错。
- 留空 = 不部署 MFA：未绑定账号不受影响；**已绑定账号会被拒绝登录**
  （`auth.ErrMFANotConfigured`），绝不静默降级为单因子。
- 更换密钥后，历史 secret 全部无法解密。更换前必须先让所有已绑定账号解绑 / 重新绑定，
  或直接清空 `account_mfa`（等价于对所有账号执行「重置 MFA」）。

### 3.2 表结构

| 表 | 用途 | 敏感字段处理 |
|---|---|---|
| `account_mfa` | 绑定关系 | `secret_enc` = base64(nonce ‖ AES-256-GCM 密文) |
| `account_mfa_recovery_codes` | 一次性恢复码 | 仅存 SHA-256 哈希；`used_at` 标记已用 |
| `mfa_login_tickets` | 登录第二因子票据 | 仅存 SHA-256 哈希；`attempts` 计数；`used_at` 单次有效 |

数据库泄露不会直接导致 MFA 被绕过：攻击者还需要 `MFA_ENCRYPTION_KEY` 才能还原 secret。
但**密钥与数据库应分开保管**（例如密钥走密钥管理 / 环境变量，数据库备份不含密钥）。

### 3.3 防重放

- TOTP：`account_mfa.last_used_step` 单调递增，同一时间步只能成功一次。
  更新语句带 `WHERE last_used_step < ?`，并发请求只有一个能成功。
- 票据：兑换成功后 `used_at` 落库，`WHERE used_at IS NULL` 保证单次有效。
- 恢复码：`used_at IS NULL` + 单行更新，天然一次性。

---

## 4. 使用流程

### 4.1 绑定（用户自助）

1. `POST /admin/auth/mfa/status` 确认 `configured=true` 且未绑定；
2. `POST /admin/auth/mfa/setup` → 返回 `secret` 与 `otpauth://` URI（状态为「待启用」）；
3. 用户在认证器 App 中**手动添加账号**并填入 `secret`（支持 URI 的 App 可直接粘 URI）；
4. 输入 App 显示的 6 位验证码 → `POST /admin/auth/mfa/enable`；
5. 服务端返回 10 个恢复码明文（**仅此一次**），前端必须提示用户立即保存。

> 说明：当前前端不生成二维码，避免引入额外依赖与把密钥交给第三方二维码服务。
> 手动输入密钥是主流认证器（Aegis / 1Password / Bitwarden / Google Authenticator）都支持的方式。

### 4.2 登录

1. `POST /admin/auth/login`：密码正确且账号已启用 MFA →
   返回 `{ "mfa_required": true, "mfa_ticket": "..." }`，**不种 Cookie、不建立会话**；
2. `POST /admin/auth/mfa/verify`：提交 `{ticket, code}`；
3. 成功后才种下会话 Cookie；失败可重试，5 次后票据作废，需重新登录。

这样即使密码泄露，攻击者仍拿不到会话。

### 4.3 解绑

`POST /admin/auth/mfa/disable`，需要**当前密码 + 当前验证码（或恢复码）**。
只凭被劫持的会话不能关闭 MFA。

### 4.4 账号恢复（管理员）

用户换手机且恢复码丢失时：
`POST /admin/admins/mfa/reset`（需 `accounts.admins` 权限），解绑目标账号的 MFA，写审计
`admin.mfa.reset`。解绑后提醒本人立即重新绑定。

---

## 5. 审计事件

| Action | 触发时机 | 结果 |
|---|---|---|
| `admin.login.mfa_challenge` | 密码通过、等待第二因子 | success |
| `admin.login.mfa` | 第二因子校验失败 | failure |
| `admin.login` | 第二因子通过、建立会话 | success |
| `admin.mfa.setup` | 生成绑定信息 | success |
| `admin.mfa.enable` | 绑定确认成功 | success |
| `admin.mfa.disable` | 解绑（含密码错误） | success / failure |
| `admin.mfa.reset` | 管理员重置他人 MFA | success |

审计只记录动作与对象，**不记录 secret、验证码、恢复码**。

---

## 6. 上线建议

1. 先给 `admin` 角色账号绑定，再要求 `editor` 等后台角色绑定；
2. 部署 `MFA_ENCRYPTION_KEY` 并纳入密钥管理，不与数据库备份放在一起；
3. 至少保留两条恢复路径：恢复码 + 管理员重置；
4. 定期检查 `account_mfa_recovery_codes` 的剩余数量，前端已提示「恢复码已用完，建议重新绑定」；
5. 若短时间无法全面启用，应在发布记录中写明风险接受与完成期限（gj.md 允许二选一）。

---

## 7. 已知限制

- 不防「实时钓鱼代理」：攻击者诱导用户在当前会话中输入验证码。TOTP 的通用弱点，
  如需更强保障应升级到 WebAuthn（未在本次范围）。
- 无「记住此设备」：每次登录都要验证码。
- 无二维码渲染：见 4.1 说明。
