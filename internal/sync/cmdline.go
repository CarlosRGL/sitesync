package sync

import (
	"fmt"
	"strings"
	"unicode"
)

// splitCommandLine parses a shell-like argument string with support for
// single quotes, double quotes, and backslash escaping.
func splitCommandLine(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := rune(0)
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote != '\'':
			escaped = true
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			inQuote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

func appendSplitArgs(args []string, raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return args, nil
	}
	parsed, err := splitCommandLine(raw)
	if err != nil {
		return nil, err
	}
	return append(args, parsed...), nil
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
}
