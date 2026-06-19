package main

import "testing"

func TestPortFromLocalAddress(t *testing.T) {
	t.Parallel()

	cases := map[string]int{
		"0.0.0.0:443":     443,
		"127.0.0.1:51820": 51820,
		"[::]:8443":       8443,
		"[::1]:1080":      1080,
		"*:80":            80,
		"9090":            9090,
		"":                0,
	}
	for input, want := range cases {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := portFromLocalAddress(input); got != want {
				t.Fatalf("portFromLocalAddress(%q) = %d, want %d", input, got, want)
			}
		})
	}
}
