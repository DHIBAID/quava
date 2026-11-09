package main

import "testing"

func TestProfilingOptIn(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"true", false},
		{"1", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("QUAVA_ENABLE_PPROF", tc.value)
			if got := profilingEnabled(); got != tc.want {
				t.Errorf("profilingEnabled() = %t, want %t", got, tc.want)
			}
		})
	}
}
