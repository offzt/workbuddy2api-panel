// probeendpoints 一次性诊断：遍历 ./auths/workbuddy-*.json 全部账号，实测每个账号
// 所有模型目录端点（企业家族 /v2、/console + v3/config 三路 UA）的可用性，
// 输出 HTTP 状态 / 模型条数 / 耗时 / 错误摘要，用于摸清"哪些端点可用"。
//
// 只读探测：不改账号状态（过期 token 会 refresh 写回，与 signin 同策略）。
// 警告：会向各端点发真实请求（每账号 5 次），有上游配额/风控敏感时酌情使用。
//
// 用法: probeendpoints [auths_dir]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

func main() {
	dir := "auths"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	files, err := filepath.Glob(filepath.Join(dir, "workbuddy-*.json"))
	if err != nil || len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no auth files in %s\n", dir)
		os.Exit(1)
	}
	sort.Strings(files)
	up := upstream.New()
	asJSON := len(os.Args) > 2 && os.Args[2] == "-json"

	type acctResult struct {
		UID    string                   `json:"uid"`
		Realm  string                   `json:"realm"`
		File   string                   `json:"file"`
		Probes []upstream.EndpointProbe `json:"probes"`
	}
	var results []acctResult

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", f, err)
			continue
		}
		a, err := auth.Parse(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", f, err)
			continue
		}
		a.FilePath = f
		realm := "cn"
		if a.IsGlobal() {
			realm = "global"
		}
		// 过期 token 先 refresh（与 signin 同策略），否则探测必然 401 误判为端点不可用。
		if a.NeedsRefresh(2 * 3600) {
			if err := up.RefreshToken(a); err != nil {
				fmt.Fprintf(os.Stderr, "refresh %s failed: %v\n", a.UID, err)
				continue
			}
			_ = a.SaveAtomic()
		}
		res := acctResult{UID: a.UID, Realm: realm, File: filepath.Base(f)}
		res.Probes = up.ProbeModelEndpoints(a)
		results = append(results, res)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}

	// 人类可读：每账号一段，逐端点一行。
	for _, r := range results {
		fmt.Printf("\n==================== %s (%s) ====================\n", r.File, r.Realm)
		fmt.Printf("%-40s %-52s %-6s %-7s %-8s %s\n", "ENDPOINT", "UA", "HTTP", "MODELS", "MS", "NOTE")
		for _, p := range r.Probes {
			ok := "OK"
			if !p.OK {
				ok = "--"
			}
			note := ""
			if p.Err != "" {
				note = p.Err
			}
			fmt.Printf("%-40s %-52s %-6d %-7d %-8d [%s] %s\n",
				p.Endpoint, trunc(p.UA, 52), p.Status, p.Models, p.ElapsedMS, ok, note)
		}
	}
	fmt.Println()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
