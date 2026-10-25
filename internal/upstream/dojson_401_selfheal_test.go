package upstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// TestDoJSON401SelfHeal 验证 doJSON 的 401 自愈：首次 401（token 过期）时自动刷新
// 账号 token 并重放请求体重试一次，刷新成功则返回 200 业务数据，且账号 token 已更新。
//
// 同时验证：刷新端点自身的 doJSON 调用不带账号上下文，不会递归触发自愈（仅调用 1 次）。
func TestDoJSON401SelfHeal(t *testing.T) {
	var mainHits, refreshHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/token/refresh":
			atomic.AddInt32(&refreshHits, 1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"code":0,"data":{"accessToken":"NEW_AT","refreshToken":"NEW_RT","expiresIn":5184000}}`)
		case "/v1/data":
			n := atomic.AddInt32(&mainHits, 1)
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `<html><head><title>401 Authorization Required</title></head><body>openresty</body></html>`)
				return
			}
			// 重试（自愈第二次）必须携带刷新后的新 token；若 doJSON 漏了重签鉴权头，
			// 这里仍按旧 token 拒绝 401，从而暴露 bug。
			if r.Header.Get("Authorization") != "Bearer NEW_AT" {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `<html><head><title>401 Authorization Required</title></head><body>openresty</body></html>`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"code":0,"data":{"ok":true}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := &auth.Auth{
		AccessToken:  "OLD_AT",
		RefreshToken: "OLD_RT",
		UID:          "u1",
	} // FilePath 故意留空：SaveAtomic 会报错但不应阻断自愈

	c := &Client{
		HTTP:          srv.Client(),
		ChatBaseCN:    srv.URL,
		GlobalEnabled: false,
	}
	c.SanitizeFingerprints.Store(true)

	body, _ := json.Marshal(map[string]any{"x": 1})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/data", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	data, err := c.doJSON(c.withAccount(req, a))
	if err != nil {
		t.Fatalf("doJSON 应在 401 自愈后成功，但返回 err=%v", err)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("解析 data 失败: %v (data=%s)", err, string(data))
	}
	if !out.OK {
		t.Fatalf("data.ok 期望 true，实际 %v", out.OK)
	}
	if got := atomic.LoadInt32(&mainHits); got != 2 {
		t.Fatalf("主端点应恰好被调用 2 次（首 401 + 重试），实际 %d", got)
	}
	if got := atomic.LoadInt32(&refreshHits); got != 1 {
		t.Fatalf("刷新端点应恰好被调用 1 次，实际 %d", got)
	}
	if a.AccessToken != "NEW_AT" {
		t.Fatalf("自愈后 a.AccessToken 应更新为 NEW_AT，实际 %q", a.AccessToken)
	}
}

// TestDoJSON401RefreshFailsKeepsError 验证：刷新失败（无 refreshToken）时保持原 401
// 错误，回落既有轮转语义，不掩盖、不 panic。
func TestDoJSON401RefreshFailsKeepsError(t *testing.T) {
	var mainHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/token/refresh":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"code":0,"data":{"accessToken":"","refreshToken":""}}`) // 缺 accessToken → 刷新失败
		case "/v1/data":
			atomic.AddInt32(&mainHits, 1)
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `<html><head><title>401 Authorization Required</title></head><body>openresty</body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := &auth.Auth{AccessToken: "OLD_AT", UID: "u1"} // 无 RefreshToken → 刷新必失败

	c := &Client{HTTP: srv.Client(), ChatBaseCN: srv.URL, GlobalEnabled: false}
	c.SanitizeFingerprints.Store(true)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/data", bytes.NewReader([]byte(`{}`)))
	_, err := c.doJSON(c.withAccount(req, a))
	if err == nil {
		t.Fatal("刷新失败时 doJSON 应返回错误，但得到 nil")
	}
	var ue *Error
	if !errors.As(err, &ue) {
		t.Fatalf("错误应为 *Error，实际 %T: %v", err, err)
	}
	if ue.Status != http.StatusUnauthorized {
		t.Fatalf("错误 Status 应为 401，实际 %d", ue.Status)
	}
	if got := atomic.LoadInt32(&mainHits); got != 1 {
		t.Fatalf("主端点应只被调用 1 次（重试应被跳过），实际 %d", got)
	}
}
