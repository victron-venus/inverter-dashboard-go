package version

import "testing"

func TestBuildVersion(t *testing.T) {
	original := version
	defer func() { version = original }()
	for _, tc := range []struct{ input, want string }{
		{"v1.9.4", "1.9.4"},
		{"nightly-abcdef0", "nightly-abcdef0"},
		{"dev", "1.9.3"},
		{"", "1.9.3"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			version = "1.9.3"
			SetBuildVersion(tc.input)
			if got := GetCurrent(); got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}
