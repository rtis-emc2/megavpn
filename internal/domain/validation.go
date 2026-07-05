package domain

import (
	"fmt"
	"strings"
	"unicode"
)

func ValidateRequiredSingleLine(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return ValidateSingleLine(field, value)
}

func ValidateSingleLine(field, value string) error {
	for _, r := range value {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
	}
	return nil
}
