package docmd

import "testing"

func TestFlatten(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "atx headings lose their markers",
			in:   "# Title\n\n## Subtitle\n\nBody text.",
			want: "Title\n\nSubtitle\n\nBody text.",
		},
		{
			name: "closed atx heading",
			in:   "## Heading ##",
			want: "Heading",
		},
		{
			name: "setext heading keeps text drops underline",
			in:   "Title\n=====\n\nBody.",
			want: "Title\n\nBody.",
		},
		{
			name: "bullet list markers stripped",
			in:   "- one\n- two\n* three\n+ four",
			want: "one\ntwo\nthree\nfour",
		},
		{
			name: "ordered list markers stripped",
			in:   "1. one\n2) two",
			want: "one\ntwo",
		},
		{
			name: "task list boxes stripped",
			in:   "- [ ] todo\n- [x] done",
			want: "todo\ndone",
		},
		{
			name: "nested list keeps indentation",
			in:   "- outer\n  - inner",
			want: "outer\n  inner",
		},
		{
			name: "block quote markers stripped",
			in:   "> quoted line\n>> nested",
			want: "quoted line\nnested",
		},
		{
			name: "emphasis unwrapped",
			in:   "**bold** and *italic* and __also bold__ and _also italic_ and ~~struck~~",
			want: "bold and italic and also bold and also italic and struck",
		},
		{
			name: "nested emphasis unwrapped",
			in:   "**bold _and_ italic**",
			want: "bold and italic",
		},
		{
			name: "links keep their label",
			in:   "See [the docs](https://example.com/a) for more.",
			want: "See the docs for more.",
		},
		{
			name: "reference links keep their label",
			in:   "See [the docs][ref] for more.",
			want: "See the docs for more.",
		},
		{
			name: "autolinks unwrapped",
			in:   "Visit <https://example.com> today.",
			want: "Visit https://example.com today.",
		},
		{
			name: "images keep alt text",
			in:   "![a diagram](https://example.com/x.png)",
			want: "a diagram",
		},
		{
			name: "code spans keep contents",
			in:   "Run `go test ./...` now.",
			want: "Run go test ./... now.",
		},
		{
			name: "fenced code kept verbatim without fences",
			in:   "```go\nfmt.Println(\"hi\")\n```",
			want: "fmt.Println(\"hi\")",
		},
		{
			name: "hash inside fenced code is not a heading",
			in:   "```\n# not a heading\n```",
			want: "# not a heading",
		},
		{
			name: "table becomes tab separated without divider",
			in:   "| Name | Role |\n|------|------|\n| Ada  | Lead |",
			want: "Name\tRole\nAda\tLead",
		},
		{
			name: "thematic breaks removed",
			in:   "Above\n\n---\n\nBelow",
			want: "Above\n\nBelow",
		},
		{
			name: "footnote references dropped",
			in:   "A claim[^1] with a note.",
			want: "A claim with a note.",
		},
		{
			name: "escaped punctuation unescaped",
			in:   `A literal \* star and \_ underscore.`,
			want: "A literal * star and _ underscore.",
		},
		{
			name: "paragraph breaks preserved",
			in:   "One.\n\nTwo.\n\n\n\nThree.",
			want: "One.\n\nTwo.\n\nThree.",
		},
		{
			name: "trailing whitespace trimmed",
			in:   "Line one   \nLine two\t",
			want: "Line one\nLine two",
		},
		// A document carrying a private-use code point must survive untouched.
		// Unparking the whole range would turn U+E000 into a NUL byte, which
		// Postgres rejects in a text column — failing the job on every retry.
		{
			name: "private-use code points from the document are left alone",
			in:   "before \uE000\uE001\uE042 after",
			want: "before \uE000\uE001\uE042 after",
		},
		{
			name: "escapes still round-trip alongside private-use code points",
			in:   "\\* and \uE000",
			want: "* and \uE000",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "whitespace only input",
			in:   "\n\n   \n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Flatten(tt.in); got != tt.want {
				t.Errorf("Flatten(%q)\n got = %q\nwant = %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFlattenIsIdempotent(t *testing.T) {
	const in = "# Title\n\n**Bold** and [a link](https://example.com).\n\n| a | b |\n|---|---|\n| 1 | 2 |"
	once := Flatten(in)
	if twice := Flatten(once); twice != once {
		t.Errorf("Flatten not idempotent:\n once = %q\ntwice = %q", once, twice)
	}
}

func TestFirstHeading(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "level one heading", in: "# The Title\n\nBody.", want: "The Title"},
		{name: "skips level two", in: "## Not This\n\n# But This", want: "But This"},
		{name: "strips inline syntax", in: "# A **bold** title", want: "A bold title"},
		{name: "ignores fenced code", in: "```\n# not a title\n```\n\n# Real Title", want: "Real Title"},
		{name: "no heading", in: "Just prose.", want: ""},
		{name: "empty heading", in: "#\n\nBody.", want: ""},
		{name: "empty input", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstHeading(tt.in); got != tt.want {
				t.Errorf("FirstHeading(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
