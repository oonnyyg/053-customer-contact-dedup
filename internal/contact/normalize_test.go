package contact

import "testing"

func TestNormalizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Alice   Wang  ", "alice wang"},
		{"Alice\tWang\n", "alice wang"},
		{"", ""},
		{"   ", ""},
		{"BOB", "bob"},
	}
	for _, c := range cases {
		if got := NormalizeName(c.in); got != c.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if NormalizeCompany("  Acme   Corp ") != "acme corp" {
		t.Fatal("company normalization failed")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Foo@Bar.COM "); got != "foo@bar.com" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeEmail("   "); got != "" {
		t.Fatalf("blank email = %q", got)
	}
	// 内部空白不处理
	if got := NormalizeEmail("a b@x.com"); got != "a b@x.com" {
		t.Fatalf("inner space altered: %q", got)
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"+86 138-0000-1234", "+8613800001234"},
		{"(021) 8888 9999", "02188889999"},
		{"138 0000 1234", "13800001234"},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
