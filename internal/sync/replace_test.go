package sync

import (
	"strings"
	"testing"
)

func TestResilientReplaceLine(t *testing.T) {
	tests := []struct {
		name    string
		search  string
		replace string
		in      string
		want    string
		opts    ReplaceOptions
	}{
		{
			name:    "basic literal same length",
			search:  "hello",
			replace: "world",
			in:      `s:5:"hello";`,
			want:    `s:5:"world";`,
		},
		{
			name:    "length decrease",
			search:  "world",
			replace: "Go",
			in:      `s:11:"hello world";`,
			want:    `s:8:"hello Go";`,
		},
		{
			name:    "length increase multiple occurrences",
			search:  "foo",
			replace: "x",
			in:      `s:9:"foobarfoo";`,
			want:    `s:5:"xbarx";`,
		},
		{
			name:    "multiple occurrences PHP-bug validation",
			search:  "aa",
			replace: "z",
			in:      `s:6:"aa baa";`,
			want:    `s:4:"z bz";`,
		},
		{
			name:    "no match returns unchanged",
			search:  "xyz",
			replace: "abc",
			in:      `s:5:"hello";`,
			want:    `s:5:"hello";`,
		},
		{
			name:    "raw (non-serialized) replacement",
			search:  "http://example.com",
			replace: "http://local.test",
			in:      `INSERT INTO wp_options VALUES (1,'siteurl','http://example.com','yes');`,
			want:    `INSERT INTO wp_options VALUES (1,'siteurl','http://local.test','yes');`,
		},
		{
			name:    "OnlyIntoSerialized leaves raw text alone",
			search:  "http://example.com",
			replace: "http://local.test",
			in:      `s:18:"http://example.com"; AND other text http://example.com`,
			opts:    ReplaceOptions{OnlyIntoSerialized: true},
			// The serialized part must be updated; the raw tail must not change.
			want: `s:17:"http://local.test"; AND other text http://example.com`,
		},
		{
			name:    "UTF-8 byte length",
			search:  "éàü",
			replace: "abc",
			in:      "s:6:\"éàü\";",
			want:    `s:3:"abc";`,
		},
		{
			name:    "escaped-quote variant",
			search:  "http://example.com",
			replace: "http://local.test",
			in:      `s:18:\"http://example.com\";`,
			want:    `s:17:\"http://local.test\";`,
		},
		{
			name:    "regex mode",
			search:  `https?://`,
			replace: "http://",
			in:      `s:20:"https://example.com/";`,
			opts:    ReplaceOptions{Regex: true},
			want:    `s:19:"http://example.com/";`,
		},
		{
			name:    "mixed line with serialized value mid-row",
			search:  "http://example.com",
			replace: "http://local.test",
			in:      `(1,42,'widget_text','a:1:{i:1;a:2:{s:4:"text";s:22:"http://example.com/hi!";}}','yes');`,
			want:    `(1,42,'widget_text','a:1:{i:1;a:2:{s:4:"text";s:21:"http://local.test/hi!";}}','yes');`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResilientReplaceLine(tc.search, tc.replace, tc.in, tc.opts)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
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

func TestResilientReplaceStream_InvalidRegex(t *testing.T) {
	r := strings.NewReader("some line\n")
	var sb strings.Builder
	err := ResilientReplaceStream(`[invalid`, "x", r, &sb, ReplaceOptions{Regex: true})
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

// BenchmarkResilientReplaceFile measures the cost of running ResilientReplaceLine
// across many lines — representative of processing a real SQL dump.
func BenchmarkResilientReplaceLine(b *testing.B) {
	line := `(1,42,'widget_text','a:1:{i:1;a:2:{s:4:"text";s:22:"http://example.com/hi!";}}','yes');`
	opts := ReplaceOptions{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ResilientReplaceLine("http://example.com", "http://local.test", line, opts)
	}
}

// BenchmarkResilientReplaceLineRegex measures the regex path with a pre-compiled regex
// (as used by ResilientReplaceStream) versus cold compilation.
func BenchmarkResilientReplaceLineRegex_Stream(b *testing.B) {
	line := `s:20:"https://example.com/";`
	opts := ReplaceOptions{Regex: true}
	// Simulate what ResilientReplaceStream does: compile once, reuse.
	var sb strings.Builder
	input := strings.Repeat(line+"\n", 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		sb.Reset()
		_ = ResilientReplaceStream(`https?://`, "http://", strings.NewReader(input), &sb, opts)
	}
}

// FuzzResilientReplaceLine ensures the function never panics on arbitrary input.
func FuzzResilientReplaceLine(f *testing.F) {
	// Seed corpus from known-good cases.
	f.Add("hello", "world", `s:5:"hello";`)
	f.Add("http://example.com", "http://local.test", `s:18:"http://example.com";`)
	f.Add("foo", "bar", `INSERT INTO t VALUES ('foo');`)
	f.Add("a", "", `s:3:"aaa";`)

	f.Fuzz(func(t *testing.T, search, replace, line string) {
		// Must not panic regardless of input.
		_ = ResilientReplaceLine(search, replace, line, ReplaceOptions{})
	})
}
