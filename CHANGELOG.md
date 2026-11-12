# Changelog

Notable changes to this module, in the format of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
[Semantic Versioning](https://semver.org/); before 1.0.0, a minor release may
break the API and a patch release does not.

Each release is an annotated `vX.Y.Z` tag with a section here. CI checks both
when the tag is pushed, and reports the API changes since the last tag on
every push to `main`.

## [Unreleased]

The first tagged release of the module as `samuelks.com/dsp`. It requires
Go 1.27.

### Added

- `dsp`: generic vector operations (`VScale`, `VAdd`, `VMul`, ...), FM
  demodulation, IIR, biquad and FIR filters, FIR and boxcar decimators, FIR
  interpolation, Goertzel tone detection, a sliding DFT, a fixed-size FFT,
  `Freqz` and `GroupDelay`, window functions, Kaiser-windowed sinc low-pass
  design, fractional-position interpolators including a polyphase
  `SincInterpolator`, interleaving and `FastAtan2`. Everything is generic over
  `float32`, `float64`, `complex64` and `complex128`.
- `dsp.Oscillator` and `dsp.Mixer`: a phase-continuous tone generator and a
  frequency shifter, in cycles per sample at every width, with the phase held
  in cycles so a binary-fraction frequency is exact. `Mixer` implements
  `Processor` and takes a real IF to complex baseband.
- `dsp.Processor`: one block contract for every filter, decimator and
  interpolator, `FMDemod`, `Rotator90` and `SDFT`. `Process(dst, src)`
  consumes all of `src`, returns `OutputLen(len(src))` and panics if `dst` is
  shorter; rate changers report `Ratio() (in, out int)`.
- `NewRealCoef...` constructors for real coefficients over complex samples,
  stored at the sample's component width so each tap costs two real
  multiplies, and `RealCoefficients` on every linear time-invariant type.
- Assembly for 386, amd64 (SSE2, SSE4.1 and AVX, chosen at run time), arm
  (NEON and VFP) and arm64, with a pure-Go implementation on every other
  architecture and an optional `GOEXPERIMENT=simd` path. The FIR filters,
  decimators, interpolators and `SincInterpolator` run vectorized block FIR
  and dot-product kernels whose results are identical on every architecture
  apart from subnormals, which 32-bit arm's NEON flushes to zero.
- `pcm`: the sample codec. `Format` names an encoding, and `Decode` and
  `Encode` convert between its bytes and full-scale samples of any width,
  real or interleaved complex: an integer is scaled by 2^(bits-1), and
  encoding rounds to nearest even, clips and encodes NaN as zero.
- `ax25`: AX.25/HDLC frame decoding and encoding (`Encoder`,
  `Frame.AppendBinary`), NRZI line coding and the CRC-16/X-25 frame check.
- `fsk`: binary FSK. `Discriminator`, `Slicer` and `Demodulator` receive AFSK
  or complex baseband at any samples per symbol, recovering the symbol clock;
  `Modulator` transmits phase-continuous FSK.
- `dtmf`: DTMF decoding.
- `firpm`: Parks-McClellan equiripple FIR design.
- `sampleio`: reading WAV, RF64 and headerless captures, including rtl-sdr
  `cu8`, as normalized samples. `Reader.ReadSamples` and `ReadPlanar` are
  generic over the sample width and decode straight to it; `Options` and
  `Info` share a `Layout` of format, rate, channels, I/Q and `Container`.
  `Writer` writes WAV, RF64 and headerless streams from samples of any width,
  filling in the WAV sizes on `Close` when the destination can seek.
- `analysis`: measuring a `dsp.Processor[float64, float64]` at a given input
  rate, exactly from its transfer function where it has a real one, and
  analyzing recordings in one pass. `Adapt` measures a filter at any sample
  width and forwards `RealCoefficients` and `Ratio`.
- `plot`: charts over `analysis` results, written as SVG or PNG with no
  dependencies outside the standard library.
- `cmd/dspviz`: one command line over both analyses, with `filter` and
  `signal` subcommand groups and a local interactive page.

[Unreleased]: https://github.com/samuel/go-dsp/commits/main
