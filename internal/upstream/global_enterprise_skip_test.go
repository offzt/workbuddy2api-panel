package upstream

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// TestProbeGlobalModels_SkipsEnterpriseOnGlobal 回归：global 账号的企业端点家族
// （/v2、/console）实测恒 404/500，探测必须跳过它们、只用 v3/config 三路结果。
// 若未来有人移除跳过逻辑，本测试会用"企业端点仍被请求"失败。
func TestProbeGlobalModels_SkipsEnterpriseOnGlobal(t *testing.T) {
	var entHits, v3Hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v3/config"):
			atomic.AddInt64(&v3Hits, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"data":{"models":[{"id":"global-model-a","contextWindow":128000}]}}`)
		case strings.Contains(r.URL.Path, "/enterprises/personal/models"):
			atomic.AddInt64(&entHits, 1)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `<html>500</html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New()
	c.ChatBaseGlobal = srv.URL
	c.ChatBaseCN = srv.URL
	global := &auth.Auth{AccessToken: "at", UID: "u-global", Domain: "www.workbuddy.ai"}
	global.BackfillRealm()
	if global.Realm() != "global" {
		t.Fatalf("test setup: expected global realm, got %s", global.Realm())
	}

	names, infos, _, _, err := c.probeGlobalModels(global)
	if err != nil {
		t.Fatalf("probeGlobalModels(global): unexpected err: %v", err)
	}
	if v3Hits == 0 {
		t.Fatal("expected v3/config probes, got 0")
	}
	if got := atomic.LoadInt64(&entHits); got != 0 {
		t.Fatalf("global enterprise endpoints must be skipped (0 hits), got %d", got)
	}
	if len(names) == 0 || len(infos) == 0 {
		t.Fatalf("expected v3 models to be returned despite skipped enterprise route; names=%v infos=%d", names, len(infos))
	}
}

// TestProbeGlobalModels_CNStillProbesEnterprise 对照：CN 账号的企业端点可用，必须照常探测
// （不被 global 跳过逻辑误伤）。
func TestProbeGlobalModels_CNStillProbesEnterprise(t *testing.T) {
	var entHits, v3Hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v3/config"):
			atomic.AddInt64(&v3Hits, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"data":{"models":[{"id":"cn-model-a"}]}}`)
		case strings.Contains(r.URL.Path, "/enterprises/personal/models"):
			atomic.AddInt64(&entHits, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"data":{"models":[{"id":"cn-ent-only"}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New()
	c.ChatBaseCN = srv.URL
	c.ChatBaseGlobal = srv.URL
	cn := &auth.Auth{AccessToken: "at", UID: "u-cn", Domain: "copilot.tencent.com"}
	cn.BackfillRealm()

	_, _, _, _, err := c.probeGlobalModels(cn)
	if err != nil {
		t.Fatalf("probeGlobalModels(cn): unexpected err: %v", err)
	}
	if got := atomic.LoadInt64(&entHits); got == 0 {
		t.Fatal("CN enterprise endpoint must still be probed, got 0 hits")
	}
	if v3Hits == 0 {
		t.Fatal("expected v3/config probes for CN too")
	}
}
