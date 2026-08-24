package dsp

// benchSize is the element count the fixed-size benchmarks here use: large
// enough to be bandwidth-bound, which is why the functions whose loop overhead
// matters use the simdBenchSizes sweep instead.
const benchSize = 1 << 14

// simdBenchSizes is the size sweep for functions whose loop overhead is only
// visible while the working set fits in L1 or L2; a single large buffer is
// already bandwidth-bound and cannot see it. 1K-1 is deliberately not a block
// multiple, so it exercises the vector tail.
var simdBenchSizes = []struct {
	name string
	n    int
}{
	{"64", 64}, {"256", 256}, {"1K", 1024}, {"1K-1", 1023},
	{"4K", 4096}, {"16K", 16384}, {"64K", 65536},
}
