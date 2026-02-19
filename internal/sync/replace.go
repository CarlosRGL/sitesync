package sync

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ReplaceOptions controls the behaviour of the replace engine.
type ReplaceOptions struct {
	// Regex treats Search as a Go regular expression.
	Regex bool
	// OnlyIntoSerialized skips replacement in plain (non-serialized) text.
	OnlyIntoSerialized bool
}

// phpSerialPattern matches PHP serialized strings: s:N:"content";
// Capture group 1 = byte-length integer N
// Capture group 2 = string content (no unescaped double-quotes inside)
var phpSerialPattern = regexp.MustCompile(`s:(\d+):"((?:[^"\\]|\\.)*?)";`)

// phpSerialPatternEsc matches the escaped-quote variant: s:N:\"content\";
// (This form appears in mysqldump output that double-escapes quotes.)
var phpSerialPatternEsc = regexp.MustCompile(`s:(\d+):\\"((?:[^"\\]|\\.)*?)\\";`)

// ResilientReplaceLine applies a single search/replace pair to one line,
// correctly adjusting the byte-count in any PHP serialized string values.
//
// This is a port of bin/resilient_replace with a bug fix: the PHP original
// only counts the first occurrence of the search string when computing the
// new byte length. This implementation counts all occurrences.
func ResilientReplaceLine(search, replace, line string, opts ReplaceOptions) string {
	if opts.Regex {
		return resilientReplaceRegex(search, replace, line, opts)
	}
	return resilientReplaceLiteral(search, replace, line, opts)
}

func resilientReplaceLiteral(search, replace, line string, opts ReplaceOptions) string {
	searchLen := len([]byte(search))
	replaceLen := len([]byte(replace))
	delta := replaceLen - searchLen

	// First pass: fix serialized strings, adjusting s:N: byte counts.
	fixSerial := func(re *regexp.Regexp, wrap func(n int, s string) string) func(string) string {
		return func(match string) string {
			sub := re.FindStringSubmatch(match)
			if sub == nil {
				return match
			}
			origN, _ := strconv.Atoi(sub[1])
			inner := sub[2]
			count := strings.Count(inner, search)
			if count == 0 {
				return match
			}
			newInner := strings.ReplaceAll(inner, search, replace)
			newN := origN + count*delta
			return wrap(newN, newInner)
		}
	}

	result := phpSerialPattern.ReplaceAllStringFunc(line,
		fixSerial(phpSerialPattern, func(n int, s string) string {
			return fmt.Sprintf(`s:%d:"%s";`, n, s)
		}),
	)
	result = phpSerialPatternEsc.ReplaceAllStringFunc(result,
		fixSerial(phpSerialPatternEsc, func(n int, s string) string {
			return fmt.Sprintf(`s:%d:\"%s\";`, n, s)
		}),
	)

	// Second pass: replace in plain text (unless suppressed).
	if !opts.OnlyIntoSerialized {
		result = strings.ReplaceAll(result, search, replace)
	}
	return result
}

func resilientReplaceRegex(search, replace, line string, opts ReplaceOptions) string {
	re := regexp.MustCompile(search)

	fixSerial := func(pattern *regexp.Regexp, wrap func(n int, s string) string) func(string) string {
		return func(match string) string {
			sub := pattern.FindStringSubmatch(match)
			if sub == nil {
				return match
			}
			origN, _ := strconv.Atoi(sub[1])
			inner := sub[2]
			newInner := re.ReplaceAllString(inner, replace)
			if newInner == inner {
				return match
			}
			newN := origN + len([]byte(newInner)) - len([]byte(inner))
			return wrap(newN, newInner)
		}
	}

	result := phpSerialPattern.ReplaceAllStringFunc(line,
		fixSerial(phpSerialPattern, func(n int, s string) string {
			return fmt.Sprintf(`s:%d:"%s";`, n, s)
		}),
	)
	result = phpSerialPatternEsc.ReplaceAllStringFunc(result,
		fixSerial(phpSerialPatternEsc, func(n int, s string) string {
			return fmt.Sprintf(`s:%d:\"%s\";`, n, s)
		}),
	)

	if !opts.OnlyIntoSerialized {
		result = re.ReplaceAllString(result, replace)
	}
	return result
}

// ResilientReplaceStream applies search/replace to every line read from r,
// writing results to w. This is used for streaming (e.g. SQL dump pipeline).
func ResilientReplaceStream(search, replace string, r io.Reader, w io.Writer, opts ReplaceOptions) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	bw := bufio.NewWriter(w)
	for sc.Scan() {
		line := ResilientReplaceLine(search, replace, sc.Text(), opts)
		if _, err := fmt.Fprintln(bw, line); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return bw.Flush()
}

// ResilientReplaceFile applies search/replace to a file in-place.
// It writes to a temp file and renames atomically.
func ResilientReplaceFile(search, replace, filePath string, opts ReplaceOptions) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".sitesync-replace-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if err := ResilientReplaceStream(search, replace, f, tmp, opts); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("replace in %s: %w", filePath, err)
	}
	tmp.Close()

	// Preserve permissions of original file.
	if fi, err := os.Stat(filePath); err == nil {
		_ = os.Chmod(tmpPath, fi.Mode())
	}

	return os.Rename(tmpPath, filePath)
}

// ApplyAllReplacements applies every search/replace pair in order to a file.
func ApplyAllReplacements(pairs [][2]string, filePath string) error {
	for _, p := range pairs {
		if err := ResilientReplaceFile(p[0], p[1], filePath, ReplaceOptions{}); err != nil {
			return err
		}
	}
	return nil
}
