// Package cloudsign 提供各云厂商 API 签名算法（包内共享，不对外导出）。
package cloudsign

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AliyunRPCSignature 阿里云 RPC 风格 API 签名（HMAC-SHA1，GET 请求）
// 与基座 buildAliyunSignature 逐字一致
func AliyunRPCSignature(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryStr strings.Builder
	for i, k := range keys {
		if i > 0 {
			queryStr.WriteString("&")
		}
		queryStr.WriteString(url.QueryEscape(k) + "=" + url.QueryEscape(params[k]))
	}

	stringToSign := "GET&" + url.QueryEscape("/") + "&" + url.QueryEscape(queryStr.String())

	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// HuaweiWSSEHeader 华为云 WSSE 认证头（短信用 hex digest，与基座 sms/huawei 一致）
func HuaweiWSSEHeader(appKey, appSecret string) string {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := fmt.Sprintf("%d", time.Now().UnixNano()/1000000)

	h := sha256.New()
	h.Write([]byte(nonce + now + appSecret))
	passwordDigest := fmt.Sprintf("%x", h.Sum(nil))

	return fmt.Sprintf("UsernameToken Username=\"%s\",PasswordDigest=\"%s\",Nonce=\"%s\",Created=\"%s\"",
		appKey, passwordDigest, nonce, now)
}

// HuaweiWSSEHeaderHMAC 华为云 WSSE 认证头（HMAC-SHA256 + base64，与基座 email/phone huawei 一致）
func HuaweiWSSEHeaderHMAC(appKey, appSecret string) string {
	now := time.Now().Format(time.RFC3339)
	nonce := fmt.Sprintf("%d", time.Now().UnixNano()/1000000)

	h := hmac.New(sha256.New, []byte(appSecret))
	h.Write([]byte(nonce + now))
	passwordDigest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("UsernameToken Username=\"%s\",PasswordDigest=\"%s\",Nonce=\"%s\",Created=\"%s\"",
		appKey, passwordDigest, nonce, now)
}

// TC3Result 腾讯云 TC3-HMAC-SHA256 签名结果
type TC3Result struct {
	Authorization string // Authorization 头完整值
	Timestamp     int64  // X-TC-Timestamp
}

// TC3Sign 腾讯云 TC3-HMAC-SHA256 签名（POST JSON，按官方规范：
// https://cloud.tencent.com/document/api/1073/37995）
// payload 必须与实际发送的 HTTP body 逐字节一致。
func TC3Sign(host, service, payload, secretId, secretKey string, now time.Time) TC3Result {
	timestamp := now.Unix()
	date := now.UTC().Format("2006-01-02")

	// 1. 规范请求串
	canonicalHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\n", host)
	signedHeaders := "content-type;host"
	hashedRequestPayload := sha256Hex(payload)
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s",
		canonicalHeaders, signedHeaders, hashedRequestPayload)

	// 2. 待签名字符串
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s",
		timestamp, credentialScope, sha256Hex(canonicalRequest))

	// 3. 计算签名
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretId, credentialScope, signedHeaders, signature)

	return TC3Result{Authorization: authorization, Timestamp: timestamp}
}

func sha256Hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
