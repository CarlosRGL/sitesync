package sync

import (
	"strings"
	"testing"
)

func TestResilientReplaceLine_BasicLiteral(t *testing.T) {
	// Simple value where replace is same length
	in := `s:5:"hello";`
	out := ResilientReplaceLine("hello", "world", in, ReplaceOptions{})
	if out != `s:5:"world";` {
		t.Errorf("got %q", out)
	}
}

func TestResilientReplaceLine_LengthIncrease(t *testing.T) {
	// "hello world" (11 bytes) → "hello Go" (8 bytes)
	in := `s:11:"hello world";`
	out := ResilientReplaceLine("world", "Go", in, ReplaceOptions{})
	want := `s:8:"hello Go";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_LengthDecrease(t *testing.T) {
	// "foobarfoo" (9) → "xbarx" (5)
	in := `s:9:"foobarfoo";`
	out := ResilientReplaceLine("foo", "x", in, ReplaceOptions{})
	want := `s:5:"xbarx";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_MultipleOccurrences(t *testing.T) {
	// "aa baa" (6) has 2 occurrences of "aa" → "z bz" (4)
	// PHP bug would give wrong N here; Go must be correct.
	in := `s:6:"aa baa";`
	out := ResilientReplaceLine("aa", "z", in, ReplaceOptions{})
	want := `s:4:"z bz";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_NoMatch(t *testing.T) {
	in := `s:5:"hello";`
	out := ResilientReplaceLine("xyz", "abc", in, ReplaceOptions{})
	if out != in {
		t.Errorf("should be unchanged, got %q", out)
	}
}

func TestResilientReplaceLine_RawReplacement(t *testing.T) {
	// Non-serialized line: plain URL replacement
	in := `INSERT INTO wp_options VALUES (1,'siteurl','http://example.com','yes');`
	out := ResilientReplaceLine("http://example.com", "http://local.test", in, ReplaceOptions{})
	want := `INSERT INTO wp_options VALUES (1,'siteurl','http://local.test','yes');`
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestResilientReplaceLine_OnlyIntoSerialized(t *testing.T) {
	// With OnlyIntoSerialized, raw occurrences must NOT be replaced.
	in := `s:18:"http://example.com"; AND other text http://example.com`
	opts := ReplaceOptions{OnlyIntoSerialized: true}
	out := ResilientReplaceLine("http://example.com", "http://local.test", in, opts)

	// Serialized part must be updated
	if !strings.Contains(out, `s:17:"http://local.test";`) {
		t.Errorf("serialized part not updated in %q", out)
	}
	// Raw occurrence must remain
	if !strings.Contains(out, "AND other text http://example.com") {
		t.Errorf("raw part was erroneously replaced in %q", out)
	}
}

func TestResilientReplaceLine_UTF8ByteLength(t *testing.T) {
	// é = 2 bytes, à = 2 bytes, ü = 2 bytes → total 6 bytes
	// Replacing with "abc" (3 bytes).
	in := "s:6:\"éàü\";"
	out := ResilientReplaceLine("éàü", "abc", in, ReplaceOptions{})
	want := `s:3:"abc";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_EscapedQuoteVariant(t *testing.T) {
	// The mysqldump escaped-quote variant: s:N:\"content\";
	in := `s:18:\"http://example.com\";`
	out := ResilientReplaceLine("http://example.com", "http://local.test", in, ReplaceOptions{})
	want := `s:17:\"http://local.test\";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_RegexMode(t *testing.T) {
	// Replace https?:// with http:// inside a serialized string
	in := `s:20:"https://example.com/";`
	opts := ReplaceOptions{Regex: true}
	out := ResilientReplaceLine(`https?://`, `http://`, in, opts)
	want := `s:19:"http://example.com/";`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestResilientReplaceLine_MixedLine(t *testing.T) {
	// A realistic WordPress wp_options row with a serialized value mid-line
	in := `(1,42,'widget_text','a:1:{i:1;a:2:{s:4:"text";s:22:"http://example.com/hi!";}}','yes');`
	out := ResilientReplaceLine("http://example.com", "http://local.test", in, ReplaceOptions{})
	// The serialized count: "http://local.test/hi!" = 21 bytes
	want := `(1,42,'widget_text','a:1:{i:1;a:2:{s:4:"text";s:21:"http://local.test/hi!";}}','yes');`
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

func TestResilientReplaceStream(t *testing.T) {
	input := "s:18:\"http://example.com\";\nplain http://example.com text\n"
	r := strings.NewReader(input)
	var sb strings.Builder
	if err := ResilientReplaceStream("http://example.com", "http://local.test", r, &sb, ReplaceOptions{}); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, `s:17:"http://local.test";`) {
		t.Errorf("serialized not replaced: %q", out)
	}
	if !strings.Contains(out, "plain http://local.test text") {
		t.Errorf("raw not replaced: %q", out)
	}
}
