package redact

import (
	"strings"
)

func Sanitize(s, token string) string {
	if token != "" {
		s = strings.ReplaceAll(s, token, "[已隐藏令牌]")
	}
	for _, word := range []string{"sauth_json", "access_token", "login_token", "cookie", "chainInfo"} {
		if strings.Contains(strings.ToLower(s), strings.ToLower(word)) {
			return "验证失败，服务端错误含敏感字段；请在 FGAuth 后台查看"
		}
	}
	if len([]rune(s)) > 300 {
		s = string([]rune(s)[:300]) + "…"
	}
	return s
}
