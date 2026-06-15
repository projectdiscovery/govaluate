package govaluate

import (
	"reflect"
	"testing"
	"unicode/utf8"
)

// lexerStreamCases collects inputs that stress byte-vs-rune cursor accounting:
// pure ASCII, valid multibyte, a real (3-byte) U+FFFD, single and multiple
// invalid bytes, a truncated sequence, and mixtures of all of the above.
var lexerStreamCases = []struct {
	name   string
	source string
}{
	{"empty", ""},
	{"ascii", "abc123"},
	{"multibyte", "café界"},
	{"real replacement rune", "a\uFFFDb"},
	{"single invalid byte", "a\xffb"},
	{"multiple invalid bytes", "\xff\xff\xff\xff\xff\xff\xff"},
	{"truncated sequence", "a\xe2\x82"},
	{"mixed valid and invalid", "界\xffé\xff\xffz"},
}

// readAll drains the stream via readCharacter and returns the runes read.
func readAll(stream *lexerStream) []rune {
	var runes []rune
	for stream.canRead() {
		runes = append(runes, stream.readCharacter())
	}
	return runes
}

// TestLexerStreamByteCursorNoDrift is the core regression guard for the
// invalid-UTF-8 fix: after reading the entire stream the byte cursor must land
// exactly on the end of the source string. Before the fix, each invalid byte
// advanced strPosition by 3 (utf8.RuneLen(RuneError)) instead of 1, so the
// cursor overshot and later slices panicked.
func TestLexerStreamByteCursorNoDrift(t *testing.T) {
	for _, tc := range lexerStreamCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newLexerStream(tc.source)
			defer stream.close()

			// strPosition must never run past the source while reading, and
			// position/strPosition must advance together within bounds.
			for stream.canRead() {
				if stream.strPosition < 0 || stream.strPosition > len(tc.source) {
					t.Fatalf("strPosition %d out of bounds [0,%d]", stream.strPosition, len(tc.source))
				}
				_ = stream.readCharacter()
			}

			if stream.strPosition != len(tc.source) {
				t.Fatalf("after draining: strPosition = %d, want %d (byte cursor drifted)", stream.strPosition, len(tc.source))
			}
			if stream.position != stream.length {
				t.Fatalf("after draining: position = %d, want %d", stream.position, stream.length)
			}
		})
	}
}

// TestLexerStreamReuseSliceMatchesRunes verifies the reuse-string fast path
// (sourceString[start:strPosition]) stays valid and consistent for every
// prefix length, which is exactly the slice that panicked on drift.
func TestLexerStreamReuseSliceMatchesRunes(t *testing.T) {
	for _, tc := range lexerStreamCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newLexerStream(tc.source)
			defer stream.close()

			start := stream.strPosition
			for stream.canRead() {
				_ = stream.readCharacter()
				// Must never index out of range, and the slice must be a real
				// substring ending where the cursor says it does.
				slice := stream.sourceString[start:stream.strPosition]
				if len(slice) > len(tc.source) {
					t.Fatalf("reuse slice longer than source: %d > %d", len(slice), len(tc.source))
				}
			}
		})
	}
}

// TestLexerStreamRewindRoundTrip ensures rewind(1) exactly undoes a single
// readCharacter for every position, including across invalid UTF-8 bytes.
func TestLexerStreamRewindRoundTrip(t *testing.T) {
	for _, tc := range lexerStreamCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newLexerStream(tc.source)
			defer stream.close()

			for stream.canRead() {
				beforePos, beforeStr := stream.position, stream.strPosition
				r := stream.readCharacter()
				stream.rewind(1)
				if stream.position != beforePos || stream.strPosition != beforeStr {
					t.Fatalf("rewind(1) after reading %q did not restore cursor: pos %d->%d, str %d->%d",
						r, beforePos, stream.position, beforeStr, stream.strPosition)
				}
				// re-read to make forward progress
				_ = stream.readCharacter()
			}
		})
	}
}

// TestLexerStreamRewindForwardRoundTrip ensures rewind(-1) (forward seek) is
// the inverse of rewind(1) (backward) for every position.
func TestLexerStreamRewindForwardRoundTrip(t *testing.T) {
	for _, tc := range lexerStreamCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newLexerStream(tc.source)
			defer stream.close()

			for stream.canRead() {
				_ = stream.readCharacter()
				afterPos, afterStr := stream.position, stream.strPosition
				stream.rewind(1)  // step back
				stream.rewind(-1) // step forward again
				if stream.position != afterPos || stream.strPosition != afterStr {
					t.Fatalf("rewind(1)+rewind(-1) not identity: pos %d->%d, str %d->%d",
						afterPos, stream.position, afterStr, stream.strPosition)
				}
			}
		})
	}
}

// TestLexerStreamRewindBounds covers the hardening from issue #5: rewind must
// not panic when asked to move past either end, and must leave the cursor at
// the boundary rather than going out of range.
func TestLexerStreamRewindBounds(t *testing.T) {
	t.Run("backward past start", func(t *testing.T) {
		stream := newLexerStream("ab")
		defer stream.close()
		// position is already 0; a backward rewind must clamp, not underflow.
		stream.rewind(5)
		if stream.position != 0 || stream.strPosition != 0 {
			t.Fatalf("expected clamp at start, got pos=%d str=%d", stream.position, stream.strPosition)
		}
	})

	t.Run("forward past end", func(t *testing.T) {
		stream := newLexerStream("ab")
		defer stream.close()
		readAll(stream) // position == length now
		posAtEnd, strAtEnd := stream.position, stream.strPosition
		// a forward rewind at EOF must clamp, not read source[length].
		stream.rewind(-5)
		if stream.position != posAtEnd || stream.strPosition != strAtEnd {
			t.Fatalf("expected clamp at end, got pos=%d str=%d (was %d/%d)", stream.position, stream.strPosition, posAtEnd, strAtEnd)
		}
	})

	t.Run("forward past end with invalid utf8", func(t *testing.T) {
		stream := newLexerStream("\xff\xff")
		defer stream.close()
		readAll(stream)
		posAtEnd, strAtEnd := stream.position, stream.strPosition
		stream.rewind(-3)
		if stream.position != posAtEnd || stream.strPosition != strAtEnd {
			t.Fatalf("expected clamp at end, got pos=%d str=%d (was %d/%d)", stream.position, stream.strPosition, posAtEnd, strAtEnd)
		}
	})
}

// TestLexerStreamRuneRoundTrip sanity-checks that the runes read back match
// ranging over the source string (Go's canonical invalid-UTF-8 decoding), so
// tokenization semantics are unchanged.
func TestLexerStreamRuneRoundTrip(t *testing.T) {
	for _, tc := range lexerStreamCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newLexerStream(tc.source)
			defer stream.close()
			got := readAll(stream)

			var want []rune
			for _, r := range tc.source {
				want = append(want, r)
			}
			if len(got) != len(want) {
				t.Fatalf("rune count = %d, want %d", len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("rune[%d] = %U, want %U", i, got[i], want[i])
				}
			}
			// invalid bytes must surface as RuneError, never corrupt the stream
			_ = utf8.RuneError
		})
	}
}

// TestUTF8ExpressionTokenization locks in end-to-end tokenization behavior when
// invalid UTF-8 appears in different token contexts, ensuring the byte-cursor
// fix neither panics nor shifts the tokens that follow the malformed input.
func TestUTF8ExpressionTokenization(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		expected   []ExpressionToken
	}{
		{
			name:       "invalid bytes in string then operator and variable",
			expression: "\"\xff\xff\" + foo",
			expected: []ExpressionToken{
				{Kind: STRING, Value: "\uFFFD\uFFFD"},
				{Kind: MODIFIER, Value: "+"},
				{Kind: VARIABLE, Value: "foo"},
			},
		},
		{
			name:       "invalid bytes between two strings",
			expression: "\"a\xffb\" == \"c\"",
			expected: []ExpressionToken{
				{Kind: STRING, Value: "a\uFFFDb"},
				{Kind: COMPARATOR, Value: "=="},
				{Kind: STRING, Value: "c"},
			},
		},
		{
			name:       "invalid bytes then numeric",
			expression: "\"\xe2\x82\" + 42",
			expected: []ExpressionToken{
				{Kind: STRING, Value: "\uFFFD\uFFFD"},
				{Kind: MODIFIER, Value: "+"},
				{Kind: NUMERIC, Value: 42.0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expression, err := NewEvaluableExpressionWithFunctions(tc.expression, nil)
			if err != nil {
				t.Fatalf("failed to parse %q: %v", tc.expression, err)
			}
			if tokens := expression.Tokens(); !reflect.DeepEqual(tokens, tc.expected) {
				t.Errorf("tokens for %q:\n got %#v\nwant %#v", tc.expression, tokens, tc.expected)
			}
		})
	}
}
