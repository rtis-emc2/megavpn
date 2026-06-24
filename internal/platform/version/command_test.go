package version

import "testing"

func TestCommandRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "long flag", args: []string{"--version"}, want: true},
		{name: "command", args: []string{"version"}, want: true},
		{name: "legacy dash", args: []string{"-version"}, want: true},
		{name: "case and whitespace", args: []string{"  VERSION "}, want: true},
		{name: "empty", args: nil, want: false},
		{name: "unknown", args: []string{"serve"}, want: false},
		{name: "extra args", args: []string{"--version", "--json"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CommandRequested(tt.args); got != tt.want {
				t.Fatalf("CommandRequested(%q) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
}
