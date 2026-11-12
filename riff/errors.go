package riff

import "errors"

var (
	// ErrNotRIFF is returned when the stream does not start with a RIFF or RF64 header.
	ErrNotRIFF = errors.New("riff: not a RIFF file")

	// ErrStaleChunk is returned when a chunk is read after the Reader has
	// advanced past it.
	ErrStaleChunk = errors.New("riff: chunk is no longer valid")

	// ErrBadChunkSize is returned when a declared size is inconsistent with the
	// container it appears in.
	ErrBadChunkSize = errors.New("riff: invalid chunk size")

	// ErrChunkTooLarge is returned when a chunk exceeds a caller supplied limit
	// on read, or the largest size a header field can express on write.
	ErrChunkTooLarge = errors.New("riff: chunk too large")

	// ErrClosed is returned when a Writer is used after Close.
	ErrClosed = errors.New("riff: writer is closed")

	// ErrUnsizedNotLast is returned when a chunk is written after one whose
	// size could not be patched. That chunk reads to the end of the stream, so
	// nothing after it is reachable.
	ErrUnsizedNotLast = errors.New("riff: an unsized chunk must be the last in the container")
)
