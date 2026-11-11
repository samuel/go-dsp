package dsp

// Processor is the contract every stateful block processor in the package
// follows: filters, rate changers, the FM demodulator and the quarter-rate
// rotator.
//
// Process consumes all of src and returns how many samples it wrote to dst,
// which is OutputLen(len(src)). It panics if dst is shorter than that. State
// carries across calls, so a stream can be processed in blocks of any length,
// and Reset returns the processor to the state its constructor left it in.
//
// OutputLen depends on the current state for a rate changer, since a partial
// output period carries across calls, so ask it rather than dividing by the
// ratio. A 1:1 processor returns its argument. dst may be the same slice as src
// wherever OutputLen(n) <= n.
type Processor[In, Out any] interface {
	Process(dst []Out, src []In) int
	OutputLen(inputs int) int
	Reset()
}

// Every block processor satisfies Processor at every width it is defined for.
func _[T Sample, F Float, C Complex]() {
	var (
		_ Processor[T, T] = (*IIRFilter[T])(nil)
		_ Processor[T, T] = (*FIRFilter[T])(nil)
		_ Processor[T, T] = (*FIRDecimator[T])(nil)
		_ Processor[T, T] = (*FIRInterpolator[T])(nil)
		_ Processor[T, T] = (*BoxcarDecimator[T])(nil)
		_ Processor[T, T] = (*RationalBoxcarDecimator[T])(nil)
		_ Processor[F, F] = (*BiQuadFilter[F])(nil)
		_ Processor[F, F] = (*DCFilter[F])(nil)
		_ Processor[C, F] = (*FMDemod[C, F])(nil)
		_ Processor[C, C] = (*Rotator90[C])(nil)
		_ Processor[C, C] = (*SDFT[C])(nil)
	)
}

// checkDst panics unless dst has room for need samples.
func checkDst(typ string, dst, need int) {
	if dst < need {
		panic("dsp: " + typ + ".Process dst is shorter than OutputLen(len(src))")
	}
}
