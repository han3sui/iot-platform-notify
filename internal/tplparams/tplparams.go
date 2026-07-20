// Package tplparams 解析 SendRequest.Extra["templateParams"] 中的结构化模板参数。
//
// 约定：templateParams 为 JSON 字符串——
//   - 命名参数（阿里云短信/语音）：JSON 对象，如 {"device":"A","temp":"80"}
//   - 位置参数（华为/腾讯短信语音）：JSON 数组，如 ["A","80"]
package tplparams

import (
	"encoding/json"
	"fmt"
)

// ParseNamed 解析命名模板参数（JSON 对象）；空串返回 nil, nil
func ParseNamed(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("templateParams 需为 JSON 对象（命名参数）: %v", err)
	}
	return obj, nil
}

// ParsePositional 解析位置模板参数（JSON 数组），元素统一转为字符串；空串返回 nil, nil
func ParsePositional(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, fmt.Errorf("templateParams 需为 JSON 数组（位置参数）: %v", err)
	}
	out := make([]string, len(arr))
	for i, v := range arr {
		switch t := v.(type) {
		case string:
			out[i] = t
		case float64:
			// JSON 数字统一为 float64，整数去掉小数部分
			if t == float64(int64(t)) {
				out[i] = fmt.Sprintf("%d", int64(t))
			} else {
				out[i] = fmt.Sprintf("%v", t)
			}
		default:
			out[i] = fmt.Sprintf("%v", v)
		}
	}
	return out, nil
}
