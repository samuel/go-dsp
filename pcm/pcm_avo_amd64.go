//go:build ignore

// Generator for the amd64 assembly behind the decode and encode kernels with
// a vector path: U8, I8 and I16LE into float, and float32 into I16LE. It is
// used whenever the archsimd kernels are unavailable: builds without
// GOEXPERIMENT=simd, and CPUs without AVX.
//
// Each CPU feature is a separate leaf; the dispatch is in Go
// (pcm_asm_amd64.go) so that tests can drive each leaf directly. The AVX
// leaves use AVX1 only: integer widening stays in 128-bit VEX registers, and
// the halves are joined with VINSERTF128 for the 256-bit convert and multiply.
package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	. "github.com/mmcloughlin/avo/reg"
)

type isa int

const (
	sse2 isa = iota
	sse4
	avx
)

func (i isa) suffix() string  { return [...]string{"SSE2", "SSE4", "AVX"}[i] }
func (i isa) feature() string { return [...]string{"SSE2", "SSE4.1", "AVX"}[i] }

// Constants, defined once: ConstData emits the symbol at each call.
var (
	bias8     Mem // 0x80 in every byte, the U8 bias
	inv128    Mem // 1/128 as float32
	inv32768  Mem // 1/32768 as float32
	inv32768D Mem // 1/32768 as float64
	mul32768  Mem // 32768 as float32
	i16Min    Mem // -32768 as float32
	i16Max    Mem // 32767 as float32
)

func main() {
	// No Package() call: the signatures use only builtin types, and loading the
	// package would make the generator depend on the build constraints of the
	// files it generates for.
	ConstraintExpr("amd64")

	bias8 = ConstData("bias8", U32(0x80808080))
	inv128 = ConstData("inv128F32", F32(1.0/128))
	inv32768 = ConstData("inv32768F32", F32(1.0/32768))
	inv32768D = ConstData("inv32768F64", F64(1.0/32768))
	mul32768 = ConstData("mul32768F32", F32(32768))
	i16Min = ConstData("i16MinF32", F32(-32768))
	i16Max = ConstData("i16MaxF32", F32(32767))

	for _, i := range []isa{avx, sse4, sse2} {
		decodeLeaf(byteDecoder("u8ToF32", true, i))
		decodeLeaf(byteDecoder("i8ToF32", false, i))
		decodeLeaf(i16Decoder(i))
		decodeLeaf(i16F64Decoder(i))
	}
	for _, i := range []isa{avx, sse2} {
		encodeLeaf(i)
	}

	Generate()
}

// decoder describes one decode leaf: its source width, the samples one
// iteration of the vector loop converts, and the emitters for that loop and
// for one scalar sample.
type decoder struct {
	name  string
	isa   isa
	width int  // source bytes per sample
	f64   bool // float64 destination
	block int  // samples per vector iteration

	// setup loads the constants; scale is returned in a register whose low
	// lane serves the scalar tail.
	setup func() (scale VecVirtual)
	// vector converts block samples at in to out.
	vector func(in, out Register, scale VecVirtual)
	// load reads one sample at in into a 64-bit register, bias removed.
	load func(in Mem, r Register)
}

// decodeLeaf emits one leaf: n = min(len(dst), len(src)/width), the vector
// loop while a whole block remains, then the scalar steps.
func decodeLeaf(d decoder) {
	dstType := "float32"
	if d.f64 {
		dstType = "float64"
	}
	name := d.name + d.isa.suffix()
	TEXT(name, NOSPLIT, "func(dst []"+dstType+", src []byte)")
	Pragma("noescape")
	Doc(name + " decodes to full scale using " + d.isa.feature() + ".")
	in := Load(Param("src").Base(), GP64())
	inLen := Load(Param("src").Len(), GP64())
	out := Load(Param("dst").Base(), GP64())
	outLen := Load(Param("dst").Len(), GP64())
	if d.width == 2 {
		SHRQ(Imm(1), inLen)
	}
	n := minLen(outLen, inLen)

	scale := d.setup()
	outSize := 4
	if d.f64 {
		outSize = 8
	}

	Label(name + "_loop")
	CMPQ(n, Imm(uint64(d.block)))
	JLT(LabelRef(name + "_tail"))
	d.vector(in, out, scale)
	ADDQ(Imm(uint64(d.block*d.width)), in)
	ADDQ(Imm(uint64(d.block*outSize)), out)
	SUBQ(Imm(uint64(d.block)), n)
	JMP(LabelRef(name + "_loop"))

	Label(name + "_tail")
	if d.isa == avx {
		// The scalar steps are legacy SSE; clearing the upper halves keeps
		// the low lane of scale and avoids the transition penalty.
		VZEROUPPER()
	}
	TESTQ(n, n)
	JZ(LabelRef(name + "_done"))
	Label(name + "_step")
	t := GP64()
	x := XMM()
	d.load(Mem{Base: in}, t)
	if d.f64 {
		CVTSQ2SD(t, x)
		MULSD(scale.AsX(), x)
		MOVSD(x, Mem{Base: out})
	} else {
		CVTSQ2SS(t, x)
		MULSS(scale.AsX(), x)
		MOVSS(x, Mem{Base: out})
	}
	ADDQ(Imm(uint64(d.width)), in)
	ADDQ(Imm(uint64(outSize)), out)
	DECQ(n)
	JNZ(LabelRef(name + "_step"))

	Label(name + "_done")
	RET()
}

// minLen leaves the smaller of the two lengths in a new register.
func minLen(a, b Register) Register {
	n := GP64()
	MOVQ(a, n)
	CMPQ(b, n)
	CMOVQLT(b, n)
	return n
}

// broadcastF32 loads a float32 constant into every lane.
func broadcastF32(c Mem, i isa) VecVirtual {
	if i == avx {
		y := YMM()
		VBROADCASTSS(c, y) // memory form: AVX1
		return y
	}
	x := XMM()
	MOVSS(c, x)
	SHUFPS(Imm(0), x, x)
	return x
}

func broadcastF64(c Mem, i isa) VecVirtual {
	if i == avx {
		y := YMM()
		VBROADCASTSD(c, y) // memory form: AVX1
		return y
	}
	x := XMM()
	MOVSD(c, x)
	MOVLHPS(x, x)
	return x
}

// byteDecoder converts sixteen bytes per iteration. U8's bias comes off with
// one byte-wise XOR of 0x80, after which both formats are signed bytes.
func byteDecoder(name string, biased bool, i isa) decoder {
	var bias VecVirtual
	d := decoder{name: name, isa: i, width: 1, block: 16}
	d.setup = func() VecVirtual {
		if biased {
			bias = XMM()
			if i == avx {
				VBROADCASTSS(bias8, bias)
			} else {
				MOVSS(bias8, bias)
				PSHUFL(Imm(0), bias, bias)
			}
		}
		return broadcastF32(inv128, i)
	}
	d.load = func(m Mem, r Register) {
		if biased {
			MOVBQZX(m, r)
			SUBQ(Imm(128), r)
		} else {
			MOVBQSX(m, r)
		}
	}
	d.vector = func(in, out Register, scale VecVirtual) {
		src := XMM()
		if i == avx {
			VMOVDQU(Mem{Base: in}, src)
			if biased {
				VPXOR(bias, src, src)
			}
			for q := range 2 {
				y := YMM()
				hi := XMM()
				if q == 0 {
					VPMOVSXBD(src, y.AsX())
				} else {
					VPSHUFD(Imm(2), src, y.AsX())
					VPMOVSXBD(y.AsX(), y.AsX())
				}
				VPSHUFD(Imm(uint64(1+2*q)), src, hi)
				VPMOVSXBD(hi, hi)
				VINSERTF128(Imm(1), hi, y, y)
				VCVTDQ2PS(y, y)
				VMULPS(scale, y, y)
				VMOVUPS(y, Mem{Base: out, Disp: 32 * q})
			}
			return
		}
		MOVOU(Mem{Base: in}, src)
		if biased {
			PXOR(bias, src)
		}
		var quads [4]VecVirtual
		if i == sse4 {
			for q := range 4 {
				quads[q] = XMM()
				if q == 0 {
					PMOVSXBD(src, quads[q])
				} else {
					PSHUFL(Imm(uint64(q)), src, quads[q])
					PMOVSXBD(quads[q], quads[q])
				}
			}
		} else {
			// Each byte is duplicated up to the top of a 32-bit lane and
			// shifted back down arithmetically.
			lo, hi := XMM(), XMM()
			MOVO(src, lo)
			PUNPCKLBW(lo, lo)
			PUNPCKHBW(src, src)
			MOVO(src, hi)
			for q, v := range []VecVirtual{lo, lo, hi, hi} {
				quads[q] = XMM()
				MOVO(v, quads[q])
				if q%2 == 0 {
					PUNPCKLWL(quads[q], quads[q])
				} else {
					PUNPCKHWL(quads[q], quads[q])
				}
				PSRAL(Imm(24), quads[q])
			}
		}
		for q, v := range quads {
			CVTPL2PS(v, v)
			MULPS(scale, v)
			MOVUPS(v, Mem{Base: out, Disp: 16 * q})
		}
	}
	return d
}

// widenI16 sign-extends the eight int16 in src to two vectors of four int32.
func widenI16(src VecVirtual, i isa) (lo, hi VecVirtual) {
	lo, hi = XMM(), XMM()
	switch i {
	case avx:
		VPMOVSXWD(src, lo)
		VPSHUFD(Imm(0xee), src, hi)
		VPMOVSXWD(hi, hi)
	case sse4:
		PMOVSXWD(src, lo)
		PSHUFL(Imm(0xee), src, hi)
		PMOVSXWD(hi, hi)
	default:
		MOVO(src, lo)
		PUNPCKLWL(lo, lo)
		PSRAL(Imm(16), lo)
		MOVO(src, hi)
		PUNPCKHWL(hi, hi)
		PSRAL(Imm(16), hi)
	}
	return lo, hi
}

func loadI16(m Mem, r Register) { MOVWQSX(m, r) }

func loadVec(in Register, i isa) VecVirtual {
	src := XMM()
	if i == avx {
		VMOVDQU(Mem{Base: in}, src)
	} else {
		MOVOU(Mem{Base: in}, src)
	}
	return src
}

// i16Decoder converts eight samples per iteration to float32.
func i16Decoder(i isa) decoder {
	return decoder{
		name: "i16LEToF32", isa: i, width: 2, block: 8, load: loadI16,
		setup: func() VecVirtual { return broadcastF32(inv32768, i) },
		vector: func(in, out Register, scale VecVirtual) {
			src := loadVec(in, i)
			if i == avx {
				y, hi := YMM(), XMM()
				VPMOVSXWD(src, y.AsX())
				VPSHUFD(Imm(0xee), src, hi)
				VPMOVSXWD(hi, hi)
				VINSERTF128(Imm(1), hi, y, y)
				VCVTDQ2PS(y, y)
				VMULPS(scale, y, y)
				VMOVUPS(y, Mem{Base: out})
				return
			}
			lo, hi := widenI16(src, i)
			for q, v := range []VecVirtual{lo, hi} {
				CVTPL2PS(v, v)
				MULPS(scale, v)
				MOVUPS(v, Mem{Base: out, Disp: 16 * q})
			}
		},
	}
}

// i16F64Decoder converts eight samples per iteration to float64.
func i16F64Decoder(i isa) decoder {
	return decoder{
		name: "i16LEToF64", isa: i, width: 2, f64: true, block: 8, load: loadI16,
		setup: func() VecVirtual { return broadcastF64(inv32768D, i) },
		vector: func(in, out Register, scale VecVirtual) {
			lo, hi := widenI16(loadVec(in, i), i)
			if i == avx {
				for q, v := range []VecVirtual{lo, hi} {
					y := YMM()
					VCVTDQ2PD(v, y)
					VMULPD(scale, y, y)
					VMOVUPD(y, Mem{Base: out, Disp: 32 * q})
				}
				return
			}
			for q, v := range []VecVirtual{lo, hi} {
				a := XMM()
				CVTPL2PD(v, a)
				PSHUFL(Imm(0xee), v, v)
				CVTPL2PD(v, v)
				MULPD(scale, a)
				MULPD(scale, v)
				MOVUPD(a, Mem{Base: out, Disp: 32 * q})
				MOVUPD(v, Mem{Base: out, Disp: 32*q + 16})
			}
		},
	}
}

// encodeLeaf emits f32ToI16LE: multiply by 32768, NaN to zero, clip to the
// int16 rails, convert rounding to nearest even (MXCSR's default, which Go
// keeps), and pack. NaN is masked first because MAXPS and MINPS return their
// second operand when either is NaN; after the clip the pack never saturates.
func encodeLeaf(i isa) {
	name := "f32ToI16LE" + i.suffix()
	TEXT(name, NOSPLIT, "func(dst []byte, src []float32)")
	Pragma("noescape")
	Doc(name + " encodes full-scale samples as little-endian int16 using " + i.feature() + ".")
	in := Load(Param("src").Base(), GP64())
	inLen := Load(Param("src").Len(), GP64())
	out := Load(Param("dst").Base(), GP64())
	outLen := Load(Param("dst").Len(), GP64())
	SHRQ(Imm(1), outLen)
	n := minLen(outLen, inLen)

	mul := broadcastF32(mul32768, i)
	lo := broadcastF32(i16Min, i)
	hi := broadcastF32(i16Max, i)

	block := 8
	if i == avx {
		block = 16
	}
	Label(name + "_loop")
	CMPQ(n, Imm(uint64(block)))
	JLT(LabelRef(name + "_tail"))
	if i == avx {
		for q := range 2 {
			v, m := YMM(), YMM()
			VMULPS(Mem{Base: in, Disp: 32 * q}, mul, v)
			VCMPPS(Imm(0), v, v, m)
			VANDPS(m, v, v)
			VMAXPS(lo, v, v)
			VMINPS(hi, v, v)
			VCVTPS2DQ(v, v)
			top := XMM()
			VEXTRACTF128(Imm(1), v, top)
			VPACKSSDW(top, v.AsX(), top)
			VMOVDQU(top, Mem{Base: out, Disp: 16 * q})
		}
	} else {
		var vs [2]VecVirtual
		for q := range 2 {
			v, m := XMM(), XMM()
			MOVUPS(Mem{Base: in, Disp: 16 * q}, v)
			MULPS(mul, v)
			MOVAPS(v, m)
			CMPPS(m, m, Imm(0))
			ANDPS(m, v)
			MAXPS(lo, v)
			MINPS(hi, v)
			CVTPS2PL(v, v)
			vs[q] = v
		}
		PACKSSLW(vs[1], vs[0])
		MOVOU(vs[0], Mem{Base: out})
	}
	ADDQ(Imm(uint64(4*block)), in)
	ADDQ(Imm(uint64(2*block)), out)
	SUBQ(Imm(uint64(block)), n)
	JMP(LabelRef(name + "_loop"))

	Label(name + "_tail")
	if i == avx {
		VZEROUPPER()
	}
	TESTQ(n, n)
	JZ(LabelRef(name + "_done"))
	Label(name + "_step")
	v, m := XMM(), XMM()
	t := GP32()
	MOVSS(Mem{Base: in}, v)
	MULSS(mul.AsX(), v)
	MOVAPS(v, m)
	CMPSS(m, m, Imm(0))
	ANDPS(m, v)
	MAXSS(lo.AsX(), v)
	MINSS(hi.AsX(), v)
	CVTSS2SL(v, t)
	MOVW(t.As16(), Mem{Base: out})
	ADDQ(Imm(4), in)
	ADDQ(Imm(2), out)
	DECQ(n)
	JNZ(LabelRef(name + "_step"))

	Label(name + "_done")
	RET()
}
