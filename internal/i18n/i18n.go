// Package i18n 提供机器人多语言文案支持（中文 / 英文）。
package i18n

import "strings"

// Lang 是受支持的语言代码。
type Lang string

const (
	// ZH 简体中文（默认语言）。
	ZH Lang = "zh"
	// EN 英语。
	EN Lang = "en"
)

// Default 为缺省语言。
const Default = ZH

// Resolve 将任意 language code（如 Telegram 上报的 "en-US"、用户选择的 "zh"）
// 归一化为受支持的 Lang；无法识别时回退到 Default。
func Resolve(code string) Lang {
	code = strings.ToLower(strings.TrimSpace(code))
	switch {
	case code == "":
		return Default
	case strings.HasPrefix(code, "en"):
		return EN
	case strings.HasPrefix(code, "zh"):
		return ZH
	default:
		return Default
	}
}

// T 返回 key 对应语言的文案。缺失时回退：目标语言 → 默认语言 → key 本身。
func T(lang Lang, key string) string {
	if m, ok := catalog[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if s, ok := catalog[Default][key]; ok {
		return s
	}
	return key
}
