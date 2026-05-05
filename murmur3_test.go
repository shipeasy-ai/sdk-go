package shipeasy

import "testing"

func TestMurmur3Vectors(t *testing.T) {
	// Values verified against the Ruby SDK reference impl in
	// packages/server-sdks/sdk-ruby — cross-language consistency is what
	// matters. Note: experiment-platform/04-evaluation.md lists divergent
	// values; that table predates the Ruby reference and is unverified.
	cases := []struct {
		in       string
		expected uint32
	}{
		{"", 0x00000000},
		{"a", 0x3c2569b2},
		{"ab", 0x9bbfd75f},
		{"abc", 0xb3dd93fa},
		{"aaaa", 0x7eeed987},
		{"aaaaa", 0xe9ca302b},
		{"Hello, 世界", 0xe2a131eb},
		{"The quick brown fox jumps over the lazy dog", 0x2e4ff723},
	}
	for _, tc := range cases {
		got := Murmur3(tc.in)
		if got != tc.expected {
			t.Errorf("Murmur3(%q) = %#x, want %#x", tc.in, got, tc.expected)
		}
	}
}
