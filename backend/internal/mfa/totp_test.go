package mfa

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// RFC 4226 附录 D 的测试种子（ASCII "12345678901234567890"）与其 base32 形式。
const (
	rfcSecretASCII  = "12345678901234567890"
	rfcSecretBase32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
)

// RFC 4226 附录 D 的 6 位 HOTP 测试向量。
var hotpVectors = []string{
	"755224", "287082", "359152", "969429", "338314",
	"254676", "287922", "162583", "399871", "520489",
}

func TestCodeAtMatchesRFC4226Vectors(t *testing.T) {
	for counter, want := range hotpVectors {
		at := time.Unix(int64(counter)*Period, 0)
		got, err := CodeAt(rfcSecretBase32, at)
		if err != nil {
			t.Fatalf("counter=%d: 计算失败: %v", counter, err)
		}
		if got != want {
			t.Fatalf("counter=%d: got %q, want %q", counter, got, want)
		}
	}
}

func TestValidateAcceptsSkewWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	code, err := CodeAt(rfcSecretBase32, now)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		at    time.Time
		code  string
		valid bool
	}{
		{"当前时间步", now, code, true},
		{"前一步（时钟偏慢）", now.Add(-Period * time.Second), code, true},
		{"后一步（时钟偏快）", now.Add(Period * time.Second), code, true},
		{"超出窗口", now.Add(2 * Period * time.Second), code, false},
		{"错误验证码", now, "000000", false},
		{"位数不足", now, "12345", false},
		{"非数字", now, "abcdef", false},
		{"空值", now, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := Validate(rfcSecretBase32, tc.code, tc.at)
			if ok != tc.valid {
				t.Fatalf("Validate(%q) = %v, want %v", tc.code, ok, tc.valid)
			}
		})
	}
}

func TestValidateReturnsMatchedStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	code, err := CodeAt(rfcSecretBase32, now)
	if err != nil {
		t.Fatal(err)
	}
	step, ok := Validate(rfcSecretBase32, code, now)
	if !ok {
		t.Fatal("期望校验通过")
	}
	if want := uint64(now.Unix() / Period); step != want {
		t.Fatalf("step = %d, want %d", step, want)
	}
}

func TestValidateRejectsMalformedSecret(t *testing.T) {
	if _, ok := Validate("not-base32-!!!", "123456", time.Now()); ok {
		t.Fatal("非法 secret 不应通过校验")
	}
}

func TestParseKeyAcceptsBase64AndHex(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}

	inputs := map[string]string{
		"base64 标准":  base64.StdEncoding.EncodeToString(raw),
		"base64 无填充": base64.RawStdEncoding.EncodeToString(raw),
		"base64 URL": base64.RawURLEncoding.EncodeToString(raw),
		"hex":        strings.ToLower(hexEncode(raw)),
	}

	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			key, err := ParseKey(input)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if len(key) != 32 {
				t.Fatalf("密钥长度 = %d, want 32", len(key))
			}
		})
	}
}

func TestParseKeyRejectsWrongLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := ParseKey(short); err == nil {
		t.Fatal("长度不足的密钥应被拒绝")
	}
	if _, err := ParseKey(""); err == nil {
		t.Fatal("空密钥应被拒绝")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}

	enc, err := Encrypt(key, secret)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if strings.Contains(enc, secret) {
		t.Fatal("密文中不应出现明文 secret")
	}

	dec, err := Decrypt(key, enc)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if dec != secret {
		t.Fatalf("解密结果 = %q, want %q", dec, secret)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	a, err := Encrypt(key, "same-plaintext")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(key, "same-plaintext")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("相同明文两次加密结果不应相同（nonce 必须随机）")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	enc, err := Encrypt([]byte("0123456789abcdef0123456789abcdef"), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt([]byte("ffffffffffffffffffffffffffffffff"), enc); err == nil {
		t.Fatal("密钥不匹配时应解密失败")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	if _, err := Decrypt(key, "not-base64!!!"); err == nil {
		t.Fatal("非法 base64 应失败")
	}
	if _, err := Decrypt(key, base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("长度不足的密文应失败")
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(RecoveryCodeCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("恢复码数量 = %d, want %d", len(codes), RecoveryCodeCount)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("恢复码重复: %q", c)
		}
		seen[c] = true
		normalized := NormalizeRecoveryCode(c)
		if len(normalized) != RecoveryCodeLength {
			t.Fatalf("归一化后长度 = %d, want %d (%q)", len(normalized), RecoveryCodeLength, c)
		}
		for _, ch := range normalized {
			if !strings.ContainsRune(recoveryAlphabet, ch) {
				t.Fatalf("恢复码含字符集外字符 %q: %q", ch, c)
			}
		}
	}
}

func TestNormalizeAndHashRecoveryCode(t *testing.T) {
	cases := map[string]string{
		"abcde-fghjk":   "ABCDEFGHJK",
		"ABCDE FGHJK":   "ABCDEFGHJK",
		"  abcdefghjk ": "ABCDEFGHJK",
	}
	for input, want := range cases {
		if got := NormalizeRecoveryCode(input); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}

	// 归一化后哈希一致：用户带不带连字符都能校验通过。
	a := HashRecoveryCode("abcde-fghjk")
	b := HashRecoveryCode("ABCDEFGHJK")
	if a != b {
		t.Fatal("归一化后哈希应相同")
	}
	if len(a) != 64 {
		t.Fatalf("SHA-256 hex 长度 = %d, want 64", len(a))
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("MC 工具站", "admin@example.com", rfcSecretBase32)
	for _, want := range []string{
		"otpauth://totp/",
		"secret=" + rfcSecretBase32,
		"issuer=",
		"digits=6",
		"period=30",
		"algorithm=SHA1",
	} {
		if !strings.Contains(uri, want) {
			t.Fatalf("URI 缺少 %q: %s", want, uri)
		}
	}
}

func hexEncode(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}
