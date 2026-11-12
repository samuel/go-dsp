// Package riff reads and writes RIFF containers, the chunked format under WAV,
// AVI and WebP.
//
// A container is a "RIFF" header naming a form type, such as "WAVE", followed
// by chunks: a four-character ID, a little-endian uint32 size, the data, and a
// pad byte when the size is odd.
//
// [New] returns a [Reader] whose [Reader.Next] yields each [Chunk] in turn. A
// Chunk is an io.Reader bounded to its data and is valid only until the next
// call to Next; after that it returns [ErrStaleChunk] rather than reading
// bytes that belong to a later chunk. Sizes are checked against the container,
// and a stream ending inside a chunk is io.ErrUnexpectedEOF. [Chunk.List]
// descends into LIST chunks, and [Chunk.Bytes] reads a whole chunk under an
// explicit limit, since the size comes from the file. An RF64 container is
// read as an unsized one, with [Reader.RF64] set; interpreting its ds64 chunk
// is left to the caller.
//
// [NewWriter] returns a [Writer]. [Writer.WriteChunk] writes a chunk whose data
// is in hand, and [Writer.Chunk] opens a [ChunkWriter] for one written as a
// stream. When the underlying writer is an io.WriteSeeker, [Writer.Close]
// patches the sizes; otherwise they stay 0xFFFFFFFF, the form streaming
// encoders emit, which [Reader.AllowUnsizedChunk] reads back. Such a chunk runs
// to the end of the stream, so it must be the last.
package riff
