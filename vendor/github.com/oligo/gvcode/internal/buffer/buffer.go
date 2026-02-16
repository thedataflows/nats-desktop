package buffer

import (
	"errors"
	"io"
	"unicode/utf8"
)

// chunk size used to grow the buffer.
const chunkSize = 4096

var (
	errReadRune = errors.New("read rune error")
)

// textBuffer is a byte buffer with a sparsing rune index to retrieve
// rune location to byte location efficiently.
type textBuffer struct {
	buf []byte
	runeOffIndex
	// Length of the  buffer in runes.
	length int
}

func newTextBuffer() *textBuffer {
	tb := &textBuffer{}
	tb.runeOffIndex = runeOffIndex{src: tb}

	return tb
}

// ReadRuneAt implements [runeReader].
func (tb *textBuffer) ReadRuneAt(byteOff int64) (rune, int, error) {
	if int(byteOff) >= len(tb.buf) {
		return 0, 0, io.EOF
	}

	c, s := utf8.DecodeRune(tb.buf[byteOff:])
	return c, s, nil
}

// set the inital buffer.
func (tb *textBuffer) set(buf []byte) int {
	tb.buf = buf
	tb.length = utf8.RuneCount(buf)
	return tb.length
}

func (tb *textBuffer) ensure(n int) {
	if cap(tb.buf)-len(tb.buf) >= n {
		return
	}

	needed := len(tb.buf) + n
	newCap := ((needed + chunkSize - 1) / chunkSize) * chunkSize
	newBuf := make([]byte, len(tb.buf), newCap)
	copy(newBuf, tb.buf)
	tb.buf = newBuf
}

// append to the buffer, returns the append rune offset, byte offset and rune length.
func (tb *textBuffer) append(buf []byte) (runeOff int, byteOff int, runeLen int) {
	// grow the buffer in chunk to avoid too many allocation.
	tb.ensure(len(buf))

	byteOff = len(tb.buf)
	runeOff = tb.length

	tb.buf = append(tb.buf, buf...)
	runeLen = utf8.RuneCount(buf)
	tb.length += runeLen
	return
}

func (tb *textBuffer) bytesForRange(runeIdx int, runeLen int) int {
	start := tb.RuneOffset(runeIdx)
	end := tb.RuneOffset(runeIdx + runeLen)

	return end - start
}

func (tb *textBuffer) getTextByRange(byteIdx int, size int) []byte {
	if byteIdx < 0 || byteIdx >= len(tb.buf) {
		return nil
	}

	return tb.buf[byteIdx : byteIdx+size]
}

// getTextByRuneRange reads runes starting at the given rune offset.
func (tb *textBuffer) getTextByRuneRange(runeIdx int, size int) []rune {
	start := tb.RuneOffset(runeIdx)
	end := tb.RuneOffset(runeIdx + size)

	textBytes := tb.buf[start:end]
	runes := make([]rune, size)

	i := 0
	for len(textBytes) > 0 {
		c, s := utf8.DecodeRune(textBytes)
		if c == utf8.RuneError && s == 1 {
			break
		}

		runes[i] = c
		i++
		textBytes = textBytes[s:]
	}

	return runes[:i]
}

// getRuneAt is used to read a single rune. It is a faster and
// zero-allocation method compared to getTextByRuneRange.
func (tb *textBuffer) getRuneAt(runeIdx int) (rune, error) {
	start := tb.RuneOffset(runeIdx)

	// Slice into the buffer directly
	b := tb.buf[start:]

	r, s := utf8.DecodeRune(b)
	if r == utf8.RuneError && s == 1 {
		return r, errReadRune
	}
	return r, nil
}
