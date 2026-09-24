package i18n

import "testing"

func TestResolve(t *testing.T) {
	cases := map[string]Lang{
		"":      Default,
		"en":    EN,
		"en-US": EN,
		"EN":    EN,
		"zh":    ZH,
		"zh-CN": ZH,
		"fr":    Default,
		" en ":  EN,
	}
	for in, want := range cases {
		if got := Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTFallback(t *testing.T) {
	// 已知 key 返回对应语言文案。
	if got := T(EN, BtnShop); got != "🛒 Recharge" {
		t.Errorf("T(EN, BtnShop) = %q", got)
	}
	// 未知 key 回退为 key 本身。
	if got := T(EN, "no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key fallback = %q", got)
	}
}

// TestCatalogParity 确保每种语言覆盖了所有 key，避免漏翻导致回退。
func TestCatalogParity(t *testing.T) {
	for key := range catalog[ZH] {
		if _, ok := catalog[EN][key]; !ok {
			t.Errorf("EN catalog missing key %q", key)
		}
	}
	for key := range catalog[EN] {
		if _, ok := catalog[ZH][key]; !ok {
			t.Errorf("ZH catalog missing key %q", key)
		}
	}
}
