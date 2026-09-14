package handler

import "testing"

func TestParseThumbFile(t *testing.T) {
	cases := []struct {
		name string
		want int64
		ok   bool
	}{
		{"5239.webp", 5239, true},
		{"1.webp", 1, true},
		{"68635.webp", 68635, true},

		// Anything that is not a plain positive id followed by .webp must be
		// refused, so this route cannot be walked into another path.
		{"", 0, false},
		{".webp", 0, false},
		{"0.webp", 0, false},
		{"-1.webp", 0, false},
		{"5239", 0, false},
		{"5239.png", 0, false},
		{"5239.webp.webp", 0, false},
		{"../../etc/passwd", 0, false},
		{"..%2f..%2fetc%2fpasswd", 0, false},
		{"5239/../../etc/passwd", 0, false},
		{"12a9.webp", 0, false},
		{" 5239.webp", 0, false},
		{"5239.webp ", 0, false},
		{"99999999999999999999.webp", 0, false},
	}

	for _, tc := range cases {
		got, ok := parseThumbFile(tc.name)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseThumbFile(%q) = (%d, %v), want (%d, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
