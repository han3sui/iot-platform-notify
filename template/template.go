// Package template 提供通知模板渲染：${var} 变量替换与 ${= expr} JS 表达式求值。
//
// 从 iot-platform 基座 alertNotifyService.processTemplateConfig 泛化抽取，
// 不依赖任何业务模型，data 由调用方自行构造。
package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/dop251/goja"
)

var (
	// jsExprRe 匹配 ${= expr} JS 表达式
	jsExprRe = regexp.MustCompile(`\$\{=\s*(.+?)\s*\}`)
)

// Render 渲染模板配置：先执行 ${= js} 表达式，再替换 ${var} 变量。
// config 为模板配置（任意 JSON 结构），data 为变量集。
// 表达式执行失败时保留原文（与基座行为一致），返回渲染后的新 map。
func Render(config map[string]any, data map[string]any) (map[string]any, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("template: marshal config failed: %w", err)
	}

	configStr := RenderString(string(configJSON), data)

	var processed map[string]any
	if err := json.Unmarshal([]byte(configStr), &processed); err != nil {
		return nil, fmt.Errorf("template: unmarshal rendered config failed: %w", err)
	}
	return processed, nil
}

// RenderString 渲染单个模板字符串：先 ${= js} 表达式，再 ${var} 变量替换
func RenderString(tpl string, data map[string]any) string {
	s := evaluateJSExpressions(tpl, data)

	for key, value := range data {
		strValue, ok := stringify(value)
		if !ok {
			continue
		}
		pattern := fmt.Sprintf(`\$\{%s\}`, regexp.QuoteMeta(key))
		re := regexp.MustCompile(pattern)
		s = re.ReplaceAllString(s, strValue)
	}
	return s
}

// stringify 将变量值转为字符串，复杂类型（map/slice/struct）不参与替换
func stringify(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case int, int64, uint, uint64, float32, float64:
		return fmt.Sprintf("%v", v), true
	case bool:
		return fmt.Sprintf("%v", v), true
	case time.Time:
		return v.Format("2006-01-02 15:04:05"), true
	default:
		return "", false
	}
}

// evaluateJSExpressions 执行 ${= expr} JS 表达式，失败时保留原文
func evaluateJSExpressions(tpl string, data map[string]any) string {
	if !jsExprRe.MatchString(tpl) {
		return tpl
	}

	vm := goja.New()
	for key, value := range data {
		switch v := value.(type) {
		case string, int, int64, uint, uint64, float32, float64, bool:
			_ = vm.Set(key, v)
		case time.Time:
			_ = vm.Set(key, v.Format("2006-01-02 15:04:05"))
		}
	}

	return jsExprRe.ReplaceAllStringFunc(tpl, func(match string) string {
		submatches := jsExprRe.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		val, err := vm.RunString(submatches[1])
		if err != nil {
			// 表达式失败保留原文，与基座行为一致
			return match
		}
		return val.String()
	})
}
