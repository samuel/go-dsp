# go-dsp

[![CI](https://github.com/samuel/go-dsp/actions/workflows/ci.yml/badge.svg)](https://github.com/samuel/go-dsp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/samuelks.com/dsp.svg)](https://pkg.go.dev/samuelks.com/dsp)

Go packages and command-line tools for digital signal processing (DSP).
It is useful for Software Defined Radio (SDR), audio, and general signal
processing. It includes primitives, a sample-format codec, protocol
decoders, and analysis tooling. The performance-critical functions have
hand-written and generated assembly for 386, amd64, arm and arm64, with
a pure-Go scalar fallback everywhere else, so the same code runs on a
Raspberry Pi 1, an AVX desktop or a wasm module.

## Packages

Paths are relative to the module, `samuelks.com/dsp`, whose root is package
`dsp`.

| Path | Contents |
| --- | --- |
| `.` (`dsp`) | The main API: generic vector operations (`VScale`, `VAdd`, ...), FM demodulation, a phase-continuous `Oscillator` and `Mixer`, IIR/biquad/FIR filters, boxcar and FIR decimators, Goertzel tone detection, sliding DFT, fixed-size FFT, transfer-function evaluation, window functions, Kaiser-windowed sinc low-pass design (`LowPassFIR`), FIR interpolation (`FIRInterpolator`, zero-phase `Upsample`), interpolators including a polyphase `SincInterpolator`, interleave/deinterleave, `FastAtan2`. |
| `pcm` | The sample codec: `Format` names an encoding (`U8`, `I16LE`, `F32BE`, ...), and `Decode` and `Encode` move between bytes and full-scale samples of any width, real or interleaved complex. |
| `ax25` | AX.25/HDLC frame encoding and decoding, NRZI line coding and CRC-16/X-25 `FCS`/`CheckFCS`. |
| `fsk` | Binary FSK modulation and demodulation: Bell 202 and Bell 103 AFSK, or complex baseband, at any samples per symbol. |
| `dtmf` | DTMF decoding built on Goertzel. |
| `sampleio` | Reading and writing WAV/RF64 audio and headerless I/Q captures as full-scale samples. |
| `firpm` | Parks-McClellan (Remez exchange) linear-phase FIR design. |
| `analysis` | Measuring filters (frequency response, impulse, step, tone spectrum, sweep) and recordings (PSD, spectrogram, level, histogram, statistics, tone tracking) as single-pass builders. |
| `plot` | Charts over `analysis` results: axes, palettes, stdlib-only SVG and PNG. |
| `cmd/dspviz` | Command line over both: `dspviz filter` measures and draws a filter, with a local interactive page; `dspviz signal` measures and draws a recording (spectrogram, spectrum, waveform, level, histogram, tones, stats). |

## Installation

```sh
go get samuelks.com/dsp
```

Release notes are in [CHANGELOG.md](CHANGELOG.md). Before 1.0, a minor release
may change the API.

Supported Go versions are the minimum in `go.mod`, currently 1.27, through
the latest release; CI builds and tests both ends. Assembly is for the four
architectures with vector hardware: 386, amd64, arm (NEON/VFP) and arm64
(NEON). On every other target (riscv64, ppc64le, s390x, wasm) the same
packages build and run against the Go scalar implementations. On amd64 the
leaves dispatch on SSE2, SSE4.1 and AVX1 at runtime.

Optionally build with `GOEXPERIMENT=simd` to use the `archsimd` Go
implementations instead. That path follows a Go experiment whose API can
change between Go releases, so it is only supported on the Go versions CI
tests it with, and a new Go release may need a new release of this module.

## Usage

### Filtering

Constructors validate their arguments and return an error. The sample type is
inferred from the arguments; constructors whose arguments are `float64` need it
written out.

```go
lp, err := dsp.NewBiquadLowPass[float32](48000, 1000, 0.7071)
if err != nil {
    return err
}
out := make([]float32, len(in))
lp.Process(out, in)
```

Every filter, rate changer, `FMDemod`, `Rotator90`, `Mixer` and `SDFT` implements
`dsp.Processor`: `Process(dst, src)` consumes all of `src` and returns the
count it wrote, which is `OutputLen(len(src))`, and panics if `dst` is shorter.
`dst` may alias `src` wherever the output is no longer than the input, so
`x = x[:d.Process(x, x)]` decimates in place. Rate changers report
`Ratio() (in, out int)`: a decimator by 4 is `(4, 1)`, an interpolator by 4 is
`(1, 4)`.

A direct-form IIR filter, widths generic across `Sample`:

```go
f, err := dsp.NewIIRFilter(b, a) // b, a inferred []float32 or []float64
f.ProcessOne(x)                  // single-sample form
f.Reset()

// Real coefficients over a complex signal: two real multiplies per tap.
c, err := dsp.NewRealCoefIIRFilter[complex64](b, a)
```

### Vector operations

Elementwise binary operations are three-operand `f(dst, a, b)` and may alias,
so `VAdd(x, x, y)` accumulates in place. Reductions return a value.

```go
dsp.VScale(dst, src, 0.5)
dsp.VAdd(dst, a, b)
peak := dsp.VMax(src)
dsp.VMulReal(iq, iq, window) // real window over complex samples
```

### Fractional positions

`InterpLinear`, `InterpHermite4p3o`, `InterpOptimal2x4p4o` and
`InterpOptimal2x6p5o` read a real signal between samples. `SincInterpolator`
is a polyphase windowed sinc whose type parameter is the coefficient width;
its methods take real or complex samples at either width.

```go
s, err := dsp.NewSincInterpolator[float32](16, 256, 0.45, 6)
y := s.At(iq, 10.25) // iq is []complex64
```

### FM demodulation

```go
fmd := dsp.NewFMDemod[complex64, float32]() // complex64 uses FastAtan2
out := make([]float32, len(iq))
fmd.Process(out, iq)
```

### FSK and packet radio

`fsk` turns AFSK or baseband FSK into line levels and back. A `Discriminator`
correlates each sample with the mark and space tones over one symbol, a
`Slicer` recovers the symbol clock from its zero crossings, and a
`Demodulator` is the two together; a `Modulator` is the transmit side.
`ax25.NRZI` converts levels to the bits `ax25.Decoder` takes, and
`ax25.Encoder` produces those bits from a `Frame`.

```go
demod, err := fsk.NewDemodulator[float32](fsk.Config{Mark: 1200, Space: 2200, Baud: 1200, Rate: 48000})
var nrzi ax25.NRZI
dec := ax25.NewDecoder()
levels := demod.Demodulate(nil, samples)
for _, b := range nrzi.Decode(nil, levels) {
    if frame := dec.Feed(b); frame != nil {
        fmt.Println(frame.Source, "to", frame.Destination)
    }
}
```

The demodulator runs about 36 million samples a second on an M-series Mac, and
its bit error rate under white noise is within a small factor of an ideal
noncoherent detector's.

### Oscillator and mixer

`dsp.Oscillator` generates a phase-continuous tone, a cosine for a real width
and `exp(+j*2*pi*f*n)` for a complex one; `dsp.Mixer` multiplies a stream by
that tone, so a positive frequency shifts the spectrum up. Both take cycles per
sample, carry their phase across calls and change frequency without a phase
jump. The phase is kept in cycles, so a binary-fraction frequency such as 0.25
steps exactly.

```go
// Tune an I/Q capture 100 kHz down to baseband.
mix, err := dsp.NewMixer[complex64, complex64](-100e3 / 2.4e6)
mix.Process(iq, iq) // in place
```

### Tone detection

`dsp.Goertzel` measures a fixed set of frequencies per block; `dtmf`
builds a DTMF decoder on top of it. Over complex samples it tells a positive
frequency from its negative image.

```go
g, err := dsp.NewGoertzel[float32]([]float64{697, 770, 852, 941, 1209, 1336, 1477, 1633}, 8000, 205)
g.Feed(block)
pows := g.Power()
```

```go
d, err := dtmf.NewStandardDecoder[float32](8000, 205)
key, _ := d.Feed(block)
if key != dtmf.NoKey {
    fmt.Printf("%c", dtmf.Key(key))
}
```

### Reading a capture

`sampleio` reads a WAV/RF64 file, or a headerless stream that must be told its
format and rate. Integer samples are scaled so the most negative code is
exactly -1 (full scale is `2^(bits-1)`); float samples pass through.

```go
rd, err := sampleio.Open("fm.wav", sampleio.Options{})
defer rd.Close()
samples := make([]float64, 4096)
for {
    n, err := rd.ReadSamples(samples) // any of float32, float64, complex64, complex128
    // process samples[:n]
    if err == io.EOF {
        break
    }
}
```

An rtl-sdr `cu8` I/Q capture:

```go
f, iq, err := sampleio.ParseFormatSpec("cu8") // rtl-sdr: u8 with IQ set
rd, err := sampleio.Open("iq.raw", sampleio.Options{Format: f, IQ: iq, Rate: 2_500_000})
```

`ReadSamples` decodes straight to the width of the slice it is handed: real for
a real stream, complex for an I/Q one. Read every channel, or every I/Q pair,
at once with `ReadPlanar`, which decodes the interleaved run once and splits it
in a single pass. `Info` reports the stream's `Layout` (format, rate,
channels, I/Q and `Container`), the same struct `Options` takes.

### Writing a capture

`sampleio.Writer` takes full-scale samples at any width and writes a WAV,
RF64 past 4 GiB, or a headerless stream. `Close` fills in the WAV sizes when
the destination can seek; a WAV down a pipe keeps the streaming placeholders.

```go
w, err := sampleio.Create("out.wav", sampleio.WriterOptions{
    Format: pcm.I16LE, Rate: 48000, Channels: 2,
})
_, err = w.WritePlanar([][]float32{left, right})
err = w.Close()
```

Writing with a Reader's `Info().Layout` reproduces a capture's shape. An
integer sample past full scale is clipped, and `Info` records a Note the first
time.

### Sample formats

`pcm.Decode` turns bytes of any `pcm.Format` into samples of any width, at
full scale: an integer is divided by 2^(bits-1), so the codes run [-1, 1).
`pcm.Encode` multiplies back, rounds to nearest even with no dither, clips to
the rails and encodes NaN as zero, so every integer code round-trips. A
complex slice reads and writes interleaved I/Q pairs. Float formats pass
through unscaled.

```go
f, err := pcm.ParseFormat("i24be")
n := pcm.Decode(dst, raw, f)      // n == min(len(dst), len(raw)/f.Width())
iq := make([]complex64, len(raw)/2)
pcm.Decode(iq, raw, pcm.U8)       // rtl-sdr cu8
pcm.Encode(out, samples, pcm.I16LE)
```

### FIR design

A Kaiser-windowed sinc low-pass, normalized to a DC gain of 1, and a 2x
interpolator built on it (the taps scaled to a gain of 2 for the stuffed
zeros). `FIRInterpolator` streams like `FIRDecimator`; `Upsample` filters a
whole buffer at zero phase.

```go
h, err := dsp.LowPassFIR[float32](143, 0.25, 9) // cutoff in cycles per sample
dsp.NormalizeTaps(h, 2)
it, err := dsp.NewFIRInterpolator(h, 2)
n := it.Process(dst, src) // n == it.OutputLen(len(src)) == 2*len(src)
```

Parks-McClellan equiripple design:

```go
h, dev, err := firpm.Design(firpm.Spec{
    NumTaps: 31,
    Bands: []firpm.Band{
        {Lower: 0, Upper: 0.125, Response: 1, Weight: 1},
        {Lower: 0.25, Upper: 0.5, Response: 0, Weight: 10},
    },
})
// h is the impulse response; dev is the achieved Chebyshev deviation.
```

## Command-line tools

`cmd/dspviz` is one binary with two groups of subcommands.

### dspviz filter

One subcommand per filter kind in its catalog, writing SVGs, plus
`serve` for a local interactive page:

```sh
go run ./cmd/dspviz filter -h
go run ./cmd/dspviz filter biquad -type lowpass -freq 1000 -q 0.7071 -out /tmp/lp
# or:
go run ./cmd/dspviz filter serve
```

### dspviz signal

One read of a file, many views:

```sh
go run ./cmd/dspviz signal capture.wav                     # the report, no flags
go run ./cmd/dspviz signal spectrogram -o /tmp/wf.png capture.wav
go run ./cmd/dspviz signal spectrum -o - capture.wav      # an SVG on standard output
go run ./cmd/dspviz signal analyze -out /tmp/look capture.wav  # six views in one pass
go run ./cmd/dspviz signal tones -rate 11025 -format i16le -mark 1600 -space 1800 -baud 300 packet.raw
go run ./cmd/dspviz signal formats                         # the sample formats
```

A headerless capture requires `-format` and `-rate`. `-o` picks the writer by
extension; `analyze` writes a set and takes `-out <dir>`.

## Examples

```sh
go run ./examples/dtmf dtmf/testdata/1223445-50ms-8000.wav      # -> 1223445
go run ./examples/dtmf -rate 44100 dtmf/testdata/0123456789-50ms-44100.wav
go run ./examples/ax25 -rate 11025 -baud 300 -mark 1600 -space 1800 radio-paket300-11025.raw
```

## Conventions

- **Destination before source** (`io.Copy` order): `f(dst, src, ...)` everywhere,
  down to the assembly argument offsets.
- **`V` prefix** marks elementwise slice arithmetic, generic over the sample
  type with no width or domain in the name: `VScale`, `VAdd`, `VSub` and
  `VMul` take real or complex samples, and the complex-only operations
  (`VConj`, `VAbs`, `VMagSq`, `VMulReal`, `VScaleReal`) are named the same
  way. A width survives in a name only when it is part of the contract:
  `FastAtan2` carries a float32 error bound.
- **Real coefficients over a complex signal** come from the `NewRealCoef...`
  constructors, which store the coefficients at the sample's component width
  and run two real multiplies per coefficient instead of a complex one.
  `RealCoefficients() (b, a []float64, ok bool)` is the transfer function of
  every linear time-invariant type, normalized so `a[0]` is 1 (an FIR
  filter's `a` is nil), with `ok` false when any coefficient has a non-zero
  imaginary part; `FIRFilter.Taps` and `IIRFilter.Coefficients` return them
  at the sample type.
- **Samples are full scale everywhere**: `pcm.Decode` divides an integer
  format by 2^(bits-1) and `pcm.Encode` multiplies by it, so nothing
  downstream carries a format's range. A `LE` or `BE` suffix on a `Format` is
  the byte order of its samples.
- **Stateless 1:1 operations process `min(dst, src)`**: the `V` functions
  return nothing, and `pcm.Decode` and `pcm.Encode` return the samples
  converted.
- **Stateful block processors implement `Processor`**: `Process` consumes all
  of `src`, returns `OutputLen(len(src))` and panics if `dst` is shorter.
  Rate changers report `Ratio() (in, out int)`, input rate to output rate.
  Accumulators (`Goertzel`, `dtmf.Decoder`, `ax25.Decoder`) take `Feed`.
- **Stateful types** have a validating constructor and a `Reset`; none are safe
  for concurrent use.
- **Normalized frequency is cycles per sample**, so 0.5 is the Nyquist
  frequency: `Freqz`, `GroupDelay`, `LowPassFIR` and `SincInterpolator` take
  it, as does `firpm` unless `Spec.SampleRate` says the band edges are in Hz.
  Constructors that model analog prototypes (the biquads, `Goertzel`) take Hz
  and a sample rate instead.

## Performance

The same source builds four optimization paths per function, chosen by build
target and, on amd64, by runtime CPU features:

1. `archsimd` Go with `GOEXPERIMENT=simd` (arm64, amd64 with AVX)
2. avo-generated assembly (amd64, AVX1 and SSE2 leaves), hand-written assembly
   for 32-bit arm (NEON/VFP) and arm64 and 386
3. scalar Go for 386 and any future port
4. the pure-Go reference, which every other path reproduces bit for bit and
   which runs on architectures with no assembly at all

The FIR filters, decimators and interpolators share one block FIR kernel, and
`SincInterpolator` one dot-product kernel, vectorized on every architecture
with assembly. Each product is rounded before it is added, so results are
identical across architectures and vector widths, except that 32-bit arm's
NEON flushes subnormal values to zero.

Benchmarks live with the code: `go test ./internal/f32 -bench .` and
`go test ./pcm -bench .` sweep sizes from 64 to 64K floats so
loop-overhead changes stay visible.

## Validation

The assembly is validated against the pure-Go reference implementations it
falls back to, including unaligned slices and mismatched lengths, and the
codec kernels are swept for out-of-bounds writes at every length from 1 to 200.
`sampleio` fixtures are generated with sox/ffmpeg and cross-checked by
`go test ./sampleio -oracle`; `firpm` is checked against the Fortran original it
was transliterated from (`go generate ./firpm`); and `plot` line charts have
byte-exact SVG goldens regenerated by `go test ./plot -update`.

## License

MIT

`firpm` is a transliteration of the Parks-McClellan FIR design program by
McClellan, Parks and Rabiner, kept alongside as `firpm/remez.fortran`.