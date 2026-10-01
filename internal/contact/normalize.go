package contact

import "strings"

// NormalizeName 去除首尾空白、合并连续空白并忽略大小写。
func NormalizeName(s string) string {
	return strings.ToLower(collapseSpaces(s))
}

// NormalizeCompany 与姓名相同的规范化方式。
func NormalizeCompany(s string) string {
	return strings.ToLower(collapseSpaces(s))
}

// NormalizeEmail 去除首尾空白并忽略大小写，不处理内部空白。
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizePhone 只移除空白、连字符和括号，不猜测国家区号。
func NormalizePhone(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f', '-', '(', ')':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func collapseSpaces(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' {
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
