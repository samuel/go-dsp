package dspviz

// A Sink consumes a real signal one block at a time.
//
// Every analysis of a recording in this package is a single-pass accumulator
// behind one of these, which is what lets a file larger than memory be looked at
// as thoroughly as one that fits: a reader hands each block to every sink in
// turn and nothing holds more than its own state. SpectrogramBuilder, the level,
// waveform and histogram builders and the statistics all satisfy it.
type Sink interface {
	Write(x []float64) error
}

// A ComplexSink consumes an I/Q signal one block at a time.
type ComplexSink interface {
	WriteComplex(x []complex128) error
}

// fold reduces a stream of rows to at most cap of them, merging adjacent pairs
// and doubling how many source rows each covers whenever the buffer fills.
//
// It is the columns reducer generalized to a row type and a merge rule, for the
// time-domain overviews, whose rows are two or three numbers rather than a whole
// spectrum. The final count lands between cap/2 and cap.
//
// The merge takes the number of source rows behind each of its arguments,
// because not every rule is indifferent to them. A maximum is, being idempotent;
// a mean is not, and merging at even weight would slide every average towards
// whatever arrived last.
type fold[T any] struct {
	cap   int
	per   int64 // source rows per column
	rows  []T
	times []float64
	n     int64 // source rows seen
	merge func(a T, an int64, b T, bn int64) T
}

func newFold[T any](capacity int, merge func(a T, an int64, b T, bn int64) T) fold[T] {
	if capacity > 0 {
		// Forced even so a merge always pairs cleanly and no column is left over.
		capacity += capacity % 2
		if capacity < 2 {
			capacity = 2
		}
	}
	return fold[T]{cap: capacity, per: 1, merge: merge}
}

func (f *fold[T]) add(row T, t float64) {
	if f.cap <= 0 {
		f.rows = append(f.rows, row)
		f.times = append(f.times, t)
		f.n++
		return
	}
	idx := int(f.n / f.per)
	if idx == len(f.rows) {
		if len(f.rows) == f.cap {
			f.mergePairs()
		}
		f.rows = append(f.rows, row)
		f.times = append(f.times, t)
	} else {
		// The column holds every row since it started, and this is one more.
		f.rows[idx] = f.merge(f.rows[idx], f.n-int64(idx)*f.per, row, 1)
	}
	f.n++
}

// mergePairs halves the number of columns. It runs only when every one of them
// is exactly full -- the trigger is the row whose index is one past the last
// column, which cannot arrive before then -- so both sides of every pair carry
// the same per source rows.
func (f *fold[T]) mergePairs() {
	half := len(f.rows) / 2
	for i := range half {
		f.rows[i] = f.merge(f.rows[2*i], f.per, f.rows[2*i+1], f.per)
		f.times[i] = f.times[2*i]
	}
	f.rows = f.rows[:half]
	f.times = f.times[:half]
	f.per *= 2
}
