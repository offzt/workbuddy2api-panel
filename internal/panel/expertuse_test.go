package panel

import "testing"

// TestParseExpertIDFromJump 钉住 expert_use_* 族任务从 jump_url 的 deeplink 抽 expertId。
// 覆盖真实观测到的三种形态：百分号编码的 deeplink、已解码的 workbuddy://、无 id 的跳转。
func TestParseExpertIDFromJump(t *testing.T) {
	cases := []struct {
		name string
		jump string
		want string
	}{
		{
			name: "encoded deeplink (maidehao)",
			jump: "https://www.workbuddy.cn/work/launch/?deeplink=workbuddy%3A%2F%2Fexperts%3FexpertId%3Dex_GDl31WahyIH0&buddy_type=workbuddy",
			want: "ex_GDl31WahyIH0",
		},
		{
			name: "encoded deeplink (douyinyin)",
			jump: "https://www.workbuddy.cn/work/launch/?deeplink=workbuddy%3A%2F%2Fexperts%3FexpertId%3Dex_FbY5nlWTqDod&buddy_type=workbuddy",
			want: "ex_FbY5nlWTqDod",
		},
		{
			name: "already-decoded scheme form",
			jump: "workbuddy://experts?expertId=ex_WZY136wV5aMt",
			want: "ex_WZY136wV5aMt",
		},
		{
			name: "snake_case expert_id variant",
			jump: "workbuddy://experts?expert_id=ex_ABC123",
			want: "ex_ABC123",
		},
		{
			name: "no id (Valorant jumps to expert page)",
			jump: "workbuddy://expert",
			want: "",
		},
		{
			name: "empty",
			jump: "",
			want: "",
		},
	}
	for _, c := range cases {
		if got := parseExpertIDFromJump(c.jump); got != c.want {
			t.Errorf("%s: parseExpertIDFromJump(%q) = %q, want %q", c.name, c.jump, got, c.want)
		}
	}
}

// TestExpertUseTasksRegistered expert_use_* / WorkBuddy_Valorant 必须注册进 autoActions，
// 否则 growthPending/队列/一键完成都会因 autoActionFor==nil 而跳过它们。
func TestExpertUseTasksRegistered(t *testing.T) {
	for _, code := range []string{
		"expert_use_maidehao", "expert_use_douyinyin",
		"expert_use_shangjia_xuanchuan", "expert_use_bowangwang", "WorkBuddy_Valorant",
	} {
		if autoActionFor(code) == nil {
			t.Errorf("任务 %q 未注册 autoAction（将无法自动完成）", code)
		}
	}
}
