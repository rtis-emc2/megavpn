package version

import "strings"

// CommandRequested reports whether argv asks the binary to print its build
// version and exit before loading runtime configuration.
func CommandRequested(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "version", "--version", "-version":
		return true
	default:
		return false
	}
}
