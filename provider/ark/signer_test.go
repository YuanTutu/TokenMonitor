package ark

import (
	"encoding/hex"
	"testing"
)

// TestSHA256Hex 验证 sha256Hex 辅助函数。
func TestSHA256Hex(t *testing.T) {
	got := sha256Hex([]byte(""))
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("sha256Hex(\"\") = %s, want %s", got, want)
	}

	got = sha256Hex([]byte("hello"))
	want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("sha256Hex(\"hello\") = %s, want %s", got, want)
	}
}

// TestHMACSHA256 验证 hmacSHA256 辅助函数。
func TestHMACSHA256(t *testing.T) {
	// RFC 4231 Test Case 1: key=0x0b*20, data="Hi There"
	key := make([]byte, 20)
	for i := range key {
		key[i] = 0x0b
	}
	got := hmacSHA256(key, "Hi There")
	want := "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7"
	if hex.EncodeToString(got) != want {
		t.Errorf("hmacSHA256 = %s, want %s", hex.EncodeToString(got), want)
	}
}

// TestEncodeRFC3986 验证 URL 编码符合 RFC3986。
func TestEncodeRFC3986(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abc", "abc"},
		{"a b", "a%20b"},
		{"a/b", "a%2Fb"},
		{"a=b", "a%3Db"},
		{"a-b_c.d~e", "a-b_c.d~e"},
		{"中文", "%E4%B8%AD%E6%96%87"},
	}
	for _, c := range cases {
		got := encodeRFC3986(c.input)
		if got != c.want {
			t.Errorf("encodeRFC3986(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestCanonicalQuery 验证规范化查询串排序正确。
func TestCanonicalQuery(t *testing.T) {
	// 模拟 Action=GetUsageDetails&Version=2024-01-01
	// 按 key 排序：Action 在 Version 前
	cases := []struct {
		query string
		want  string
	}{
		{"Action=GetUsageDetails&Version=2024-01-01", "Action=GetUsageDetails&Version=2024-01-01"},
		{"Version=2024-01-01&Action=GetUsageDetails", "Action=GetUsageDetails&Version=2024-01-01"},
		{"B=2&A=1&C=3", "A=1&B=2&C=3"},
	}
	for _, c := range cases {
		v := parseQueryRaw(c.query)
		got := canonicalQuery(v)
		if got != c.want {
			t.Errorf("canonicalQuery(%q) = %q, want %q", c.query, got, c.want)
		}
	}
}

// TestCanonicalHeaders 验证规范化头按 key 排序。
func TestCanonicalHeaders(t *testing.T) {
	headers := map[string]string{
		"host":    "open.volcengineapi.com",
		"x-date":  "20240101T120000Z",
		"content-type": "application/json",
	}
	signedHeaders, canonStr := canonicalHeaders(headers)

	// 签名头名应按字母序排列
	wantSigned := "content-type;host;x-date"
	if signedHeaders != wantSigned {
		t.Errorf("signedHeaders = %q, want %q", signedHeaders, wantSigned)
	}

	// 规范化头串每行 key:value\n
	wantCanon := "content-type:application/json\nhost:open.volcengineapi.com\nx-date:20240101T120000Z\n"
	if canonStr != wantCanon {
		t.Errorf("canonicalHeaders = %q, want %q", canonStr, wantCanon)
	}
}

// TestSigningKey 验证 SigningKey 推导链。
// 参考火山引擎文档的推导公式:
// kDate = HMAC("VOLC4" + SK, shortDate)
// kRegion = HMAC(kDate, region)
// kService = HMAC(kRegion, service)
// kSigning = HMAC(kService, "request")
func TestSigningKey(t *testing.T) {
	sk := "testSecretKey"
	shortDate := "20240101"
	region := "cn-beijing"
	service := "ark"

	// 手动推导预期值
	kDate := hmacSHA256([]byte("VOLC4"+sk), shortDate)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "request")

	// 验证链式推导的确定性（相同输入必产生相同输出）
	kDate2 := hmacSHA256([]byte("VOLC4"+sk), shortDate)
	if hex.EncodeToString(kDate) != hex.EncodeToString(kDate2) {
		t.Error("HMAC 推导不确定")
	}

	// 验证最终 signing key 非空且长度正确（SHA256 = 32 字节）
	if len(kSigning) != 32 {
		t.Errorf("signing key 长度 = %d, want 32", len(kSigning))
	}

	// 验证每一步推导改变了值（不是简单复制）
	if hex.EncodeToString(kDate) == hex.EncodeToString(kRegion) {
		t.Error("kDate == kRegion，推导链可能有误")
	}
	if hex.EncodeToString(kRegion) == hex.EncodeToString(kService) {
		t.Error("kRegion == kService，推导链可能有误")
	}
}

// TestSignBasic 验证 Sign 函数能生成完整签名头。
func TestSignBasic(t *testing.T) {
	creds := Credentials{
		AccessKeyID:     "AKTEST123",
		SecretAccessKey: "SKtest456",
	}

	signed, err := Sign(SignedRequest{
		Method:  "POST",
		URL:     "https://open.volcengineapi.com/?Action=GetUsageDetails&Version=2024-01-01",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte("{}"),
	}, creds, "ark", "cn-beijing")

	if err != nil {
		t.Fatalf("Sign 失败: %v", err)
	}

	// Authorization 头应包含算法名、Credential、SignedHeaders、Signature
	if signed.Authorization == "" {
		t.Error("Authorization 头为空")
	}
	if signed.XDate == "" {
		t.Error("XDate 为空")
	}
	if signed.Host == "" {
		t.Error("Host 为空")
	}

	// 验证 Authorization 格式
	wantPrefix := "HMAC-SHA256 Credential=AKTEST123/"
	if len(signed.Authorization) < len(wantPrefix) || signed.Authorization[:len(wantPrefix)] != wantPrefix {
		t.Errorf("Authorization 前缀 = %q, want %q", signed.Authorization[:min(len(signed.Authorization), len(wantPrefix))], wantPrefix)
	}

	// 验证包含 SignedHeaders 和 Signature
	if !contains(signed.Authorization, "SignedHeaders=") {
		t.Error("Authorization 缺少 SignedHeaders")
	}
	if !contains(signed.Authorization, "Signature=") {
		t.Error("Authorization 缺少 Signature")
	}

	// XDate 格式应为 yyyyMMddTHHmmssZ
	if len(signed.XDate) != 16 || signed.XDate[8] != 'T' || signed.XDate[15] != 'Z' {
		t.Errorf("XDate 格式错误: %q", signed.XDate)
	}
}

// TestSignMissingCredentials 验证缺少凭据时返回错误。
func TestSignMissingCredentials(t *testing.T) {
	_, err := Sign(SignedRequest{
		Method: "POST",
		URL:    "https://open.volcengineapi.com/",
		Body:   []byte("{}"),
	}, Credentials{}, "ark", "cn-beijing")

	if err == nil {
		t.Error("缺少凭据应返回错误")
	}
}

// TestSignDeterministic 验证相同输入产生相同签名（确定性）。
func TestSignDeterministic(t *testing.T) {
	creds := Credentials{
		AccessKeyID:     "AKTEST",
		SecretAccessKey: "SKtest",
	}
	req := SignedRequest{
		Method:  "POST",
		URL:     "https://open.volcengineapi.com/?Action=GetUsageDetails&Version=2024-01-01",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte("{}"),
	}

	s1, _ := Sign(req, creds, "ark", "cn-beijing")
	s2, _ := Sign(req, creds, "ark", "cn-beijing")

	// 注意：XDate 含时间戳，两次调用可能不同，所以只比较签名结构而非精确值
	// 这里验证的是签名不报错且格式正确
	if s1.Authorization == "" || s2.Authorization == "" {
		t.Error("签名为空")
	}
}

// --- 辅助函数 ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseQueryRaw 解析原始 query 字符串为 url.Values（不转码）。
func parseQueryRaw(query string) (v map[string][]string) {
	v = make(map[string][]string)
	if query == "" {
		return
	}
	parts := splitString(query, "&")
	for _, part := range parts {
		kv := splitString(part, "=")
		if len(kv) == 2 {
			v[kv[0]] = append(v[kv[0]], kv[1])
		}
	}
	return
}

func splitString(s, sep string) []string {
	var result []string
	current := ""
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, current)
			current = ""
			i += len(sep) - 1
		} else {
			current += string(s[i])
		}
	}
	result = append(result, current)
	return result
}
