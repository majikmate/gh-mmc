package mmc

import (
	"github.com/cli/go-gh/v2/pkg/term"
)

// colorEnabled is set if the output is a terminal that supports colors, respecting NO_COLOR and CLICOLOR_FORCE
var colorEnabled = term.FromEnv().IsColorEnabled()

// Cyan returns the text colored cyan if the output supports colors
func Cyan(text string) string {
	if !colorEnabled {
		return text
	}
	return "\033[36m" + text + "\033[0m"
}
