// Package fsk modulates and demodulates binary frequency-shift keying: the
// AFSK of Bell 202 packet radio and APRS, Bell 103 on HF, and FSK at complex
// baseband from an SDR.
//
// A Config names the two tones, the symbol rate and the sample rate, and every
// type takes one. Samples per symbol need not be a whole number, so 1200 baud
// works at 44.1 kHz as well as at 48 kHz.
//
// The receive side is two stages. A Discriminator correlates the input with
// the mark and space tones over a sliding window of one symbol and outputs the
// difference of the two magnitudes, a soft decision per sample. A Slicer
// recovers the symbol clock from that stream's zero crossings and takes one
// level per symbol at mid-symbol. A Demodulator is the two together. The
// Modulator is the transmit side, a phase-continuous tone switched between the
// two frequencies at each symbol.
//
// The correlations are not dsp.SDFT bins. An SDFT tracks an integer bin of an
// n-point transform, and the mark and space tones are rarely whole bins of a
// one-symbol window: 2200 Hz at 1200 baud is 1.83 cycles a symbol. The
// Discriminator keeps a running sum of each tone's products instead, which
// costs the same per sample at any frequency.
//
// There is no squelch or carrier detect. A caller that wants one thresholds the
// Discriminator's output, whose magnitude follows the signal's; a link layer
// with a checksum, such as ax25, rejects the bits noise produces on its own.
//
// Levels are what the line carries, 1 for mark and 0 for space. A line code
// such as AX.25's NRZI, where a bit is a change of level or none, is the link
// layer's: see ax25.NRZI.
package fsk
