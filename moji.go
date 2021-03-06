package readline

import (
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

var isVsCodeTerminal = os.Getenv("VSCODE_PID") != ""
var isWindowsTerminal = os.Getenv("WT_SESSION") != "" && os.Getenv("WT_PROFILE_ID") != "" && !isVsCodeTerminal

var (
	// SurrogatePairOk is true when the surrogated pair unicode is supported
	// If it is false, <NNNN> is displayed instead.
	SurrogatePairOk = isWindowsTerminal

	// ZeroWidthJoinSequenceOk is true when ZWJ(U+200D) is supported.
	// If it is false, <NNNN> is displayed instead.
	ZeroWidthJoinSequenceOk = isWindowsTerminal

	// VariationSequenceOk is true when Variation Sequences are supported.
	// If it is false, <NNNN> is displayed instead.
	VariationSequenceOk = isWindowsTerminal

	// ModifierSequenceOk is false, SkinTone sequence are treated as two
	// character
	ModifierSequenceOk = isWindowsTerminal
)

var wtRuneWidth *runewidth.Condition

func init() {
	wtRuneWidth = runewidth.NewCondition()
	if isWindowsTerminal {
		wtRuneWidth.EastAsianWidth = false
	}
}

// Moji is the interface for minimum unit to edit in readline
//
// When we make a new implement type of Moji,
// we have to append the code in the function:
// string2moji() and KeyFuncInsertSelf().
type Moji interface {
	Width() WidthT
	WriteTo(io.Writer) (int64, error)
	PrintTo(io.Writer)
}

type _ZeroWidthJoinSequence [2]Moji

func (s _ZeroWidthJoinSequence) Width() WidthT {
	// runewidth.StringWidth should not be used because the width that it gives
	// has no compatible with WindowsTerminal's.
	return s[0].Width() + 1 + s[1].Width()
}

func (s _ZeroWidthJoinSequence) WriteTo(w io.Writer) (int64, error) {
	n1, err := s[0].WriteTo(w)
	if err != nil {
		return n1, err
	}
	n2, err := writeRune(w, zeroWidthJoinRune)
	if err != nil {
		return n1 + n2, err
	}
	n3, err := s[1].WriteTo(w)
	return n1 + n2 + n3, err
}

func (s _ZeroWidthJoinSequence) PrintTo(w io.Writer) {
	switch s0 := s[0].(type) {
	case _WavingWhiteFlagCodePoint:
		saveCursorAfterN(w, s.Width())
		s0.WriteTo(w)
		writeRune(w, zeroWidthJoinRune)
		s[1].WriteTo(w)
		restoreCursor(w)
	default:
		s0.PrintTo(w)
		writeRune(w, zeroWidthJoinRune)
		s[1].PrintTo(w)
	}
}

type _ModifierSequence [2]Moji

func isEmojiModifier(ch rune) bool {
	if !ModifierSequenceOk {
		return false
	}
	return '\U0001F3FB' <= ch && ch <= '\U0001F3FF'
}

func areEmojiModifier(s string) bool {
	if !ModifierSequenceOk {
		return false
	}
	u, _ := utf8.DecodeRuneInString(s)
	return isEmojiModifier(u)
}

func (s _ModifierSequence) Width() WidthT {
	return s[0].Width() + s[1].Width()
}

func (s _ModifierSequence) WriteTo(w io.Writer) (int64, error) {
	n1, err := s[0].WriteTo(w)
	if err != nil {
		return n1, err
	}
	n2, err := s[1].WriteTo(w)
	return n1 + n2, err
}

func (s _ModifierSequence) PrintTo(w io.Writer) {
	s.WriteTo(w)
}

type _VariationSequence [2]Moji

func (s _VariationSequence) Width() WidthT {
	switch s0 := s[0].(type) {
	case _WavingWhiteFlagCodePoint:
		return s0.Width()
	default:
		return s0.Width() + 1
	}
}

func (s _VariationSequence) WriteTo(w io.Writer) (int64, error) {
	n1, err := s[0].WriteTo(w)
	if err != nil {
		return n1, err
	}
	n2, err := s[1].WriteTo(w)
	return n1 + n2, err
}

func saveCursorAfterN(w io.Writer, n WidthT) {
	for i := WidthT(0); i < n; i++ {
		w.Write([]byte{' '})
	}
	w.Write([]byte{'\x1B', '7'})
	for i := WidthT(0); i < n; i++ {
		w.Write([]byte{'\b'})
	}
}

func restoreCursor(w io.Writer) {
	w.Write([]byte{'\x1B', '8'})
}

func (s _VariationSequence) PrintTo(w io.Writer) {
	saveCursorAfterN(w, s.Width())
	// The sequence 'ESC 7' can not remember the cursor position more than one.
	// When _VariationSequence contains another _VariationSequence
	// s[0].PrintTo(w) does not work as we expect.
	s[0].WriteTo(w)
	s[1].WriteTo(w)
	restoreCursor(w)
}

const zeroWidthJoinRune = '\u200D'

func isZeroWidthJoin(r rune) bool {
	return ZeroWidthJoinSequenceOk && unicode.Is(unicode.Join_Control, r)
}

func areZeroWidthJoin(s string) bool {
	if !ZeroWidthJoinSequenceOk {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return isZeroWidthJoin(r)
}

func string2moji(s string) []Moji {
	runes := []rune(s)
	mojis := make([]Moji, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if isZeroWidthJoin(runes[i]) && i > 0 && i+1 < len(runes) {
			mojis[len(mojis)-1] =
				_ZeroWidthJoinSequence(
					[...]Moji{mojis[len(mojis)-1], _RawCodePoint(runes[i+1])})
			i++
		} else if isVariationSelectorLike(runes[i]) && i > 0 {
			mojis[len(mojis)-1] =
				_VariationSequence(
					[...]Moji{mojis[len(mojis)-1], _RawCodePoint(runes[i])})
		} else if isEmojiModifier(runes[i]) && i > 0 {
			mojis[len(mojis)-1] =
				_ModifierSequence(
					[...]Moji{mojis[len(mojis)-1], _RawCodePoint(runes[i])})
		} else {
			mojis = append(mojis, rune2moji(runes[i]))
		}
	}
	return mojis
}

func moji2string(m []Moji) string {
	var buffer strings.Builder
	for _, m1 := range m {
		m1.WriteTo(&buffer)
	}
	return buffer.String()
}
