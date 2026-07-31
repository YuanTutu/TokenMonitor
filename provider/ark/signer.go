// Package ark 实现火山引擎方舟(Ark)订阅套餐用量查询。
//
// GetUsageDetails 接口通过火山引擎 OpenAPI 网关调用，需用 Volcengine
// Signature V4（类 AWS SigV4）对请求签名。本文件实现签名算法。
package ark

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 签名算法常量。火山引擎 V4 签名基于 AWS SigV4，但算法名是 "HMAC-SHA256"
// （不带 VOLC 前缀），且 SigningKey 推导不加 "VOLC4" 前缀。
const (
	signingAlgorithm = "HMAC-SHA256"
	credentialScope  = "request" // 签名末端固定为 "request"
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// Credentials 是火山引擎 IAM 凭据。
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

// SignRequest 对请求进行 V4 签名，返回需要设置的请求头。
// service: 服务名（ark 用 "ark"）；region: 区域（如 "cn-beijing"）。
type SignedRequest struct {
	Method  string
	URL     string              // 完整 URL（含 query）
	Headers map[string]string  // 原始头（不含签名头）
	Body    []byte              // 请求体
}

// SignedHeaders 是签名后需要附加到 HTTP 请求的头。
type SignedHeaders struct {
	Authorization  string
	XDate          string // x-date 头值
	Host           string // Host 头值
	ContentType    string
	XContentSha256 string // x-content-sha256 头值（参与签名，请求必须带上）
}

// Sign 计算 V4 签名，返回需要附加的请求头。
func Sign(req SignedRequest, creds Credentials, service, region string) (SignedHeaders, error) {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return SignedHeaders{}, fmt.Errorf("缺少 AK/SK")
	}

	// 解析 URL，分离 path / query / host
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return SignedHeaders{}, fmt.Errorf("解析 URL 失败: %w", err)
	}
	host := parsed.Host
	path := parsed.Path
	if path == "" {
		path = "/"
	}

	// 1. 时间戳（UTC）
	now := time.Now().UTC()
	xDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")

	// 准备待签名头。火山引擎要求 x-date、host 必签。
	headers := map[string]string{
		"x-date":    xDate,
		"host":      host,
	}
	if req.Headers != nil {
		for k, v := range req.Headers {
			headers[strings.ToLower(k)] = v
		}
	}
	contentType := headers["content-type"]

	// 2. 计算请求体哈希。火山引擎要求 X-Content-Sha256 头参与签名。
	payloadHash := emptyPayloadHash
	if len(req.Body) > 0 {
		sum := sha256.Sum256(req.Body)
		payloadHash = hex.EncodeToString(sum[:])
	}
	headers["x-content-sha256"] = payloadHash

	// 3. 规范化查询串（标准库排序+编码，+ 替换为 %20）
	canonQuery := canonicalQuery(parsed.Query())

	// 4. 规范化头
	signedHeaderNames, canonicalHeadersStr := canonicalHeaders(headers)

	// 5. 构造 CanonicalRequest
	canonReq := strings.Join([]string{
		strings.ToUpper(req.Method),
		canonPath(path),
		canonQuery,
		canonicalHeadersStr,
		signedHeaderNames,
		payloadHash,
	}, "\n")

	// 6. 构造 StringToSign
	credScope := fmt.Sprintf("%s/%s/%s/%s", shortDate, region, service, credentialScope)
	canonReqHash := sha256Hex([]byte(canonReq))
	stringToSign := strings.Join([]string{
		signingAlgorithm,
		xDate,
		credScope,
		canonReqHash,
	}, "\n")

	// 7. 推导 SigningKey: HMAC 链。火山引擎不加前缀（不同于 AWS 的 "AWS4" 前缀）。
	kDate := hmacSHA256([]byte(creds.SecretAccessKey), shortDate)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, credentialScope)

	// 8. 计算签名
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	// 9. 组装 Authorization 头
	auth := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		signingAlgorithm, creds.AccessKeyID, credScope, signedHeaderNames, signature)

	return SignedHeaders{
		Authorization:  auth,
		XDate:          xDate,
		Host:           host,
		ContentType:    contentType,
		XContentSha256: payloadHash,
	}, nil
}

// canonicalQuery 对 query 参数排序并 RFC3986 编码。
// canonicalQuery 用 url.Values.Encode() 规范化查询串（标准库自动按 key 排序
// 并 URL 编码），再把 + 替换为 %20。这与火山引擎官方 SDK 的 normquery 一致。
func canonicalQuery(values url.Values) string {
	encoded := values.Encode()
	return strings.ReplaceAll(encoded, "+", "%20")
}

// canonicalHeaders 构造规范化头串和已签名头名列表。
// 返回 (signedHeaders, canonicalHeadersString)。
// canonicalHeadersString 格式: "key:value\nkey:value\n"（每行末尾有 \n）。
func canonicalHeaders(headers map[string]string) (string, string) {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(strings.TrimSpace(headers[k]))
		b.WriteString("\n")
	}
	return strings.Join(keys, ";"), b.String()
}

// canonPath 对 path 做 RFC3986 编码（保留 /）。
func canonPath(path string) string {
	// 火山引擎 SDK 对 path 每段分别编码，这里用 QueryEscape 后把 + 还原、/ 还原。
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = encodeRFC3986(s)
	}
	return strings.Join(segments, "/")
}

// encodeRFC3986 按 RFC3986 编码（比 QueryEscape 多编 -_.~ 之外的所有字符，且空格用 %20）。
func encodeRFC3986(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '~' {
			b.WriteRune(r)
		} else {
			// 多字节字符按 UTF-8 字节编码
			bs := []byte(string(r))
			for _, bt := range bs {
				fmt.Fprintf(&b, "%%%02X", bt)
			}
		}
	}
	return b.String()
}

// sha256Hex 返回 data 的 SHA256 十六进制摘要。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hmacSHA256 返回 HMAC-SHA256(key, data)。
func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
