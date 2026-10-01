package contacts

import "strings"

// collapseSpaces trims leading/trailing whitespace and collapses runs of
// whitespace into a single space.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalizeName canonicalizes a name: trim, collapse whitespace, lowercase.
func normalizeName(s string) string {
	return strings.ToLower(collapseSpaces(s))
}

// normalizeCompany canonicalizes a company name like a personal name.
func normalizeCompany(s string) string {
	return strings.ToLower(collapseSpaces(s))
}

// normalizeEmail trims whitespace and lowercases.
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizePhone removes spaces, hyphens and parentheses only. No country
// code is inferred.
func normalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '-', '(', ')':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalize(r Record) normalized {
	return normalized{
		name:    normalizeName(r.Name),
		company: normalizeCompany(r.Company),
		email:   normalizeEmail(r.Email),
		phone:   normalizePhone(r.Phone),
	}
}
