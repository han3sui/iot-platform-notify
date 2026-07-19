// Package channeltest 提供渠道单测公共工具：将 notify.HTTPClient 重定向到 httptest 服务。
package channeltest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	notify "github.com/han3sui/iot-platform-notify"
)

// rewriteTransport 将所有出站请求重写到测试服务器（保留原始 Host 供签名校验）
type rewriteTransport struct {
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 保留原始 Host 到 Header（部分渠道签名依赖 host）
	req.Header.Set("X-Original-Host", req.URL.Host)
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// Intercept 启动 httptest 服务并把 notify.HTTPClient 指向它，测试结束自动恢复
func Intercept(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	target, _ := url.Parse(srv.URL)

	old := notify.HTTPClient
	notify.HTTPClient = &http.Client{Transport: &rewriteTransport{target: target}}
	t.Cleanup(func() {
		notify.HTTPClient = old
		srv.Close()
	})
	return srv
}
