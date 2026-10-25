// tasksdump 一次性诊断工具：遍历 ./auths/workbuddy-*.json 全部账号，dump 每个账号的
// **全量**成长任务列表（默认口径 + 小程序口径），用于发现面板「任务中心」因
// growthPending 过滤而看不到的新任务/锁定任务。
//
// 与面板任务中心的区别：面板只列「未完成且 autoActions 有注册」的待办；本工具列全部，
// 含 locked / 已领取 / 无自动动作的任务，code 一览无余，便于给新任务补自动完成。
//
// 用法:
//
//	go run ./cmd/tasksdump [auths_dir]
//	# 或编译后 ./tasksdump ./auths
//
// 只读，不改账号状态（除过期 token 的 refresh 写回，与 signin 同策略）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// autoCodes 面板 autoActions 已注册的任务码（与 internal/panel/autotask.go 保持同步的
// 镜像，仅用于本诊断的高亮标注；不是功能依赖，不同步也不影响 dump 本身）。
var autoCodes = map[string]bool{
	"chat_5": true, "first_buddy": true, "Model_chat_GLM5.2": true, "RichMeow_Chat": true,
	"Buddy_App": true, "Buddy_App_QQ": true, "automation_1": true, "Library_read": true,
	"template_5": true, "playbook_prompt": true, "create_canvas": true, "expert_5": true,
	"Expert_team_use_3": true, "Hp_Appearance": true, "skill_1": true, "Expert_lighthouse": true,
	"black_cat": true, "school_season": true,
	"expert_use_maidehao": true, "expert_use_douyinyin": true, "expert_use_shangjia_xuanchuan": true,
	"expert_use_bowangwang": true, "WorkBuddy_Valorant": true,
	"Sequential_Tasks_1": true, "Sequential_Tasks_2": true, "Sequential_Tasks_3": true,
	"Sequential_Tasks_4": true, "Sequential_Tasks_5": true, "Sequential_Tasks_6": true,
	"Sequential_Tasks_7": true,
}

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

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("\n## %s — 读取失败: %v\n", filepath.Base(f), err)
			continue
		}
		a, err := auth.Parse(raw)
		if err != nil {
			fmt.Printf("\n## %s — 解析失败: %v\n", filepath.Base(f), err)
			continue
		}
		a.FilePath = f
		head := fmt.Sprintf("%s (uid=%s realm=%s)", filepath.Base(f), a.UID, realmOf(a))
		fmt.Printf("\n==================== %s ====================\n", head)

		if a.NeedsRefresh(2 * 3600) {
			if err := up.RefreshToken(a); err != nil {
				fmt.Printf("  [refresh 失败，跳过该账号] %v\n", err)
				continue
			}
			if err := a.SaveAtomic(); err != nil {
				fmt.Printf("  [refresh 后写回失败] %v\n", err)
			}
		}

		dumpList(up, a, "默认口径", up.ListTasks)
		dumpList(up, a, "小程序口径(miniprogram)", up.ListTasksMP)
	}
}

// dumpList 拉一类口径并逐条打印。
func dumpList(up *upstream.Client, a *auth.Auth, label string, fn func(*auth.Auth) ([]upstream.Task, error)) {
	tasks, err := fn(a)
	if err != nil {
		fmt.Printf("  [%s] 拉取失败: %v\n", label, err)
		return
	}
	fmt.Printf("  ---- %s：%d 项 ----\n", label, len(tasks))
	for _, t := range tasks {
		auto := "❌无自动动作"
		if autoCodes[t.TaskCode] {
			auto = "✅已注册"
		}
		flags := []string{}
		if t.Locked {
			flags = append(flags, "locked")
		}
		if t.Claimed {
			flags = append(flags, "claimed")
		} else if t.Claimable {
			flags = append(flags, "claimable")
		}
		fmt.Printf("  %-22s %-8s prog=%d/%d accept=%-11s status=%-9s reward=%dc+%de %s\n",
			t.TaskCode, auto, t.Current, t.Target, dash(t.AcceptStatus), dash(t.Status),
			t.Credit, t.Energy, strings.Join(flags, ","))
		if t.Title != "" || t.Description != "" || t.TaskDesc != "" {
			fmt.Printf("      title=%q desc=%q task_desc=%q\n", t.Title, t.Description, t.TaskDesc)
		}
		if t.JumpURL != "" {
			fmt.Printf("      jump=%s\n", t.JumpURL)
		}
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func realmOf(a *auth.Auth) string {
	if a.IsGlobal() {
		return "global"
	}
	return "cn"
}
