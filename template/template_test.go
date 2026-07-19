package template

import (
	"testing"
	"time"
)

func TestRenderStringVars(t *testing.T) {
	data := map[string]any{
		"name":  "逆变器-01",
		"value": 3.14,
		"count": 5,
		"ok":    true,
		"ts":    time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC),
	}
	got := RenderString("设备${name} 值=${value} 次数=${count} 状态=${ok} 时间=${ts}", data)
	want := "设备逆变器-01 值=3.14 次数=5 状态=true 时间=2026-07-19 12:00:00"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderStringJSExpr(t *testing.T) {
	data := map[string]any{"value": 3.14159}
	got := RenderString("四舍五入：${= value.toFixed(2) }", data)
	if got != "四舍五入：3.14" {
		t.Errorf("got %q", got)
	}
}

func TestRenderStringJSExprError(t *testing.T) {
	// 表达式失败时保留原文
	got := RenderString("bad: ${= not_defined.x }", map[string]any{})
	if got != "bad: ${= not_defined.x }" {
		t.Errorf("expected original text preserved, got %q", got)
	}
}

func TestRenderStringUnknownVar(t *testing.T) {
	// 未定义变量保留原文
	got := RenderString("hello ${missing}", map[string]any{"name": "x"})
	if got != "hello ${missing}" {
		t.Errorf("got %q", got)
	}
}

func TestRenderConfig(t *testing.T) {
	config := map[string]any{
		"subject": "告警：${ruleName}",
		"content": "设备 ${deviceName} 触发 ${= level === 'critical' ? '严重' : '一般' }告警",
	}
	data := map[string]any{
		"ruleName":   "功率过低",
		"deviceName": "PV-001",
		"level":      "critical",
	}
	got, err := Render(config, data)
	if err != nil {
		t.Fatal(err)
	}
	if got["subject"] != "告警：功率过低" {
		t.Errorf("subject = %q", got["subject"])
	}
	if got["content"] != "设备 PV-001 触发 严重告警" {
		t.Errorf("content = %q", got["content"])
	}
}

func TestRenderComplexValueSkipped(t *testing.T) {
	// map/slice 类型变量不参与替换
	got := RenderString("x=${obj}", map[string]any{"obj": map[string]any{"a": 1}})
	if got != "x=${obj}" {
		t.Errorf("got %q", got)
	}
}
