package buffer

// piece is a single piece of text in the piece table.
// We use doubly linked list to represent a piece table here.
type piece struct {
	next *piece
	prev *piece

	// offset is the rune offset in the buffer.
	offset int
	// length is the rune length of text of the piece covers.
	length int
	// byte offset in the buffer.
	byteOff int
	// byte length of the text.
	byteLength int
	// source specifies which buffer this piece point to.
	source bufSrc
}

type pieceCache struct {
	// last piece used.
	lastPiece *piece
	// rune offset of cached piece in the document.
	startRunes int
	// bytes offset of cached piece in the document.
	startBytes int
	rev        int
}

// Use sentinel nodes to be used as head and tail, as pointed out in https://www.catch22.net/tuts/neatpad/piece-chains/.
type pieceList struct {
	head, tail *piece
	// piece cache for rapid offset query.
	cache pieceCache
	// rev tracks the revision of the overall piece list. Everytime some pieces in the list
	// are mutated, splitted, removed, added, rev should be increased.
	rev int
}

// CursorPos keep track of the previous cursor position of undo/redo.
type CursorPos struct {
	// start rune offset
	Start int
	// end rune offset
	End int
}

// A piece-range effectively represents the range of pieces affected by an operation on the sequence.
// Two kinds range exist here:
//  1. Normal range of pieces with the first and last all effective pieces.
//  2. Boundary range that has no piece in the range. The first and last pointer points to the encompassed pieces in the sequence.
type pieceRange struct {
	first    *piece
	last     *piece
	boundary bool

	// The cursor position in runes before insert or delete.
	cursor CursorPos
	// batchId is the id of a group of modifications cased by a atomic operation.
	// undo/redo should check continuous same batchId to find all batched modifications.
	batchId *int
}

func newPieceList() *pieceList {
	p := &pieceList{
		head: &piece{},
		tail: &piece{},
	}
	p.head.next = p.tail
	p.tail.prev = p.head

	return p
}

func (pl *pieceList) Head() *piece {
	return pl.head.next
}

func (pl *pieceList) Tail() *piece {
	return pl.tail.prev
}

func (pl *pieceList) InsertBefore(existing *piece, newPiece *piece) {
	newPiece.next = existing
	newPiece.prev = existing.prev
	existing.prev.next = newPiece
	existing.prev = newPiece
}

func (pl *pieceList) InsertAfter(existing *piece, newPiece *piece) {
	newPiece.prev = existing
	newPiece.next = existing.next
	existing.next.prev = newPiece
	existing.next = newPiece
}

func (pl *pieceList) Append(newPiece *piece) {
	pl.InsertBefore(pl.tail, newPiece)
}

// FindPiece finds a piece by a runeIndex in the sequence/document, returning
// the found piece, it rune offset in the found piece, and the bytes offset
// of the piece in the text sequence. If the runeIndex reaches the end of the
// piece chain, the sentinal tail piece is returned.
func (pl *pieceList) FindPiece(runeIndex int) (p *piece, offset int, bytesOff int) {
	if runeIndex <= 0 {
		return pl.head.next, 0, 0
	}

	// Try the piece cache first
	if pl.cache.lastPiece != nil && pl.rev == pl.cache.rev {
		if runeIndex >= pl.cache.startRunes && runeIndex < pl.cache.startRunes+pl.cache.lastPiece.length {
			return pl.cache.lastPiece, runeIndex - pl.cache.startRunes, pl.cache.startBytes
		}
	}

	// Fallback to scan if the cache is invalid or missed.
	pieceOff := 0
	for n := pl.head.next; n != nil; n = n.next {
		nextPos := pieceOff + n.length
		if runeIndex < nextPos {
			// update cache
			pl.cache.lastPiece = n
			pl.cache.startRunes = pieceOff
			pl.cache.startBytes = bytesOff
			pl.cache.rev = pl.rev

			return n, runeIndex - pieceOff, bytesOff
		}

		pieceOff = nextPos
		bytesOff += n.byteLength
	}

	// reached tail sentinel
	return pl.tail, 0, bytesOff
}

// FindPieceByBytes finds a piece by a byteIndex in the sequence/document, returning
// the found piece, it byte offset in the found piece. If the runeIndex reaches the
// end of the piece chain, the sentinal tail piece is returned.
func (pl *pieceList) FindPieceByBytes(byteIndex int) (*piece, int) {
	if byteIndex <= 0 {
		return pl.head.next, 0
	}

	if pl.cache.lastPiece != nil && pl.rev == pl.cache.rev {
		if byteIndex >= pl.cache.startBytes && byteIndex < pl.cache.startBytes+pl.cache.lastPiece.byteLength {
			return pl.cache.lastPiece, byteIndex - pl.cache.startBytes
		}
	}

	// Fallback  to scan if the cache is invalid or missed.
	bytesOff := 0
	runesOff := 0
	for n := pl.head.next; n != nil; n = n.next {
		nextPos := bytesOff + n.byteLength
		if byteIndex < nextPos {
			// update cache
			pl.cache.lastPiece = n
			pl.cache.startRunes = runesOff
			pl.cache.startBytes = bytesOff
			pl.cache.rev = pl.rev

			return n, byteIndex - bytesOff
		}

		bytesOff = nextPos
		runesOff += n.length
	}

	return pl.tail, 0
}

func (pl *pieceList) invalidateCache() {
	pl.rev++
}

// Remove a piece from the chain.
func (pl *pieceList) Remove(piece *piece) {
	if piece == nil || piece == pl.head || piece == pl.tail {
		return
	}

	piece.prev.next = piece.next
	piece.next.prev = piece.prev
}

// Length returns total pieces of the chain
func (pl *pieceList) Length() int {
	t := 0
	for n := pl.head.next; n != pl.tail; n = n.next {
		t++
	}

	return t
}

// AsBoundary turns the pieceRange to a boundary range by linking its first to the prev node of target,
// and the last ndoe as target.
func (p *pieceRange) AsBoundary(target *piece) {
	p.first = target.prev
	p.last = target
	p.boundary = true
}

func (p *pieceRange) Append(piece *piece) {
	if piece == nil {
		return
	}

	if p.first == nil {
		// first time insert of a piece
		p.first = piece
	} else {
		p.last.next = piece
		piece.prev = p.last
	}

	p.last = piece
	p.boundary = false
}

// Swap replaces the pieces of p that it contains with the ones from the dest pieceRange.
// If p is in the main list, this method links the pieces list contained in dest into the main list.
// After swap p's linkage still points to the previous places so that it can be used for undo by pushing
// it into the undo stack.
//
// The opposite operation is Restore.
func (p *pieceRange) Swap(dest *pieceRange) {
	if p.boundary {
		if !dest.boundary {
			p.first.next = dest.first
			p.last.prev = dest.last
			dest.first.prev = p.first
			dest.last.next = p.last
		}
	} else {
		if dest.boundary {
			p.first.prev.next = p.last.next
			p.last.next.prev = p.first.prev
		} else {
			p.first.prev.next = dest.first
			p.last.next.prev = dest.last
			dest.first.prev = p.first.prev
			dest.last.next = p.last.next
		}
	}
}

// Restore the saved pieces in undo/redo stack to the main list.
func (p *pieceRange) Restore() {
	if p.boundary {
		first := p.first.next
		last := p.last.prev

		// Unlink the pieces from the list
		p.first.next = p.last
		p.last.prev = p.first

		// Store the removed range
		p.first = first
		p.last = last
		p.boundary = false
	} else {
		first := p.first.prev
		last := p.last.next

		// The dest range is empty, thus a boundary range.
		if first.next == last {
			// move the old range back to the empty region.
			first.next = p.first
			last.prev = p.last
			// store the removed range
			p.first = first
			p.last = last
			p.boundary = true
		} else {
			// Replacing a range of pieces in the list.
			// Find the range that is currently in the list
			first := first.next
			last := last.prev

			// unlink
			first.prev.next = p.first
			last.next.prev = p.last

			// store
			p.first = first
			p.last = last
			p.boundary = false

		}
	}
}

// Size returns the runes and bytes contained in the pieces of the range.
func (p *pieceRange) Size() (runes, bytes int) {
	if p.first == nil || p.boundary {
		return 0, 0
	}

	for n := p.first; n != p.last.next; n = n.next {
		runes += n.length
		bytes += n.byteLength
	}

	return
}

// Length returns the total pieces in the range.
func (p *pieceRange) Length() int {
	if p.first == nil || p.boundary {
		return 0
	}

	t := 0
	if p.first == p.last {
		return 1
	}

	for n := p.first; n != p.last.next; n = n.next {
		t++
	}

	return t
}

// Pieces retruns all the pieces of pieceRange as a slice.
func (p *pieceRange) Pieces() []*piece {
	pieces := make([]*piece, 0)

	if p.first == nil || p.boundary {
		return pieces
	}

	if p.first == p.last {
		pieces = append(pieces, p.first)
		return pieces
	}

	for n := p.first; n != p.last.next; n = n.next {
		pieces = append(pieces, n)
	}

	return pieces
}
