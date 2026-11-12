package fsk

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"samuelks.com/dsp/ax25"
)

// TestAX25EndToEnd runs the whole packet radio chain: frames through an
// ax25.Encoder and NRZI into a Modulator, a little noise, then a Demodulator,
// NRZI and an ax25.Decoder, which has to hand back every frame.
func TestAX25EndToEnd(t *testing.T) {
	frames := []ax25.Frame{
		{Destination: ax25.Address{Callsign: "APRS"}, Source: ax25.Address{Callsign: "N0CALL", SSID: 9},
			Repeaters: []ax25.Address{{Callsign: "WIDE1", SSID: 1}}, V1: true,
			Type: ax25.UFrame, UnnumberedType: ax25.UI, PID: ax25.NoLayer3Protocol, Info: []byte("!4903.50N/07201.75W-Test")},
		{Destination: ax25.Address{Callsign: "BEACON"}, Source: ax25.Address{Callsign: "K1ABC"}, V1: true,
			Type: ax25.UFrame, UnnumberedType: ax25.UI, PID: ax25.NoLayer3Protocol, Info: []byte{0x7e, 0x7e, 0xff, 0x00, 0xff}},
		{Destination: ax25.Address{Callsign: "K1ABC", SSID: 2}, Source: ax25.Address{Callsign: "N0CALL"}, Command: true,
			Type: ax25.IFrame, SendSeq: 2, RecvSeq: 6, PID: ax25.NoLayer3Protocol, Info: make([]byte, 200)},
	}
	enc := ax25.Encoder{Preamble: 16}
	var bits []byte
	for i := range frames {
		var err error
		if bits, err = enc.AppendFrame(bits, &frames[i]); err != nil {
			t.Fatal(err)
		}
	}
	var txNRZI ax25.NRZI
	levels := txNRZI.Encode(nil, bits)

	for _, tc := range []struct {
		name string
		c    Config
	}{
		{"Bell 202 at 8000", at(bell202, 8000)},
		{"Bell 202 at 44100", at(bell202, 44100)},
		{"Bell 202 at 48000", at(bell202, 48000)},
		{"Bell 103 at 8000", at(bell103, 8000)},
		{"Bell 103 at 11025", at(bell103, 11025)},
		{"HF packet at 11025", Config{Mark: 1600, Space: 1800, Baud: 300, Rate: 11025}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := modulate[float32](tc.c, levels)
			rng := rand.New(rand.NewPCG(14, 14))
			for i := range x {
				x[i] = 0.5*x[i] + float32(rng.NormFloat64()*0.05)
			}
			var rxNRZI ax25.NRZI
			dec := ax25.NewDecoder()
			var got []*ax25.Frame
			for _, b := range rxNRZI.Decode(nil, must(NewDemodulator[float32](tc.c)).Demodulate(nil, x)) {
				if f := dec.Feed(b); f != nil {
					got = append(got, f)
				}
			}
			if len(got) != len(frames) {
				t.Fatalf("decoded %d frames, want %d", len(got), len(frames))
			}
			for i := range frames {
				if !reflect.DeepEqual(*got[i], frames[i]) {
					t.Errorf("frame %d:\n%+v\nwant\n%+v", i, *got[i], frames[i])
				}
			}
		})
	}
}

// TestDiscriminatorDelay checks that a change of tone reaches the output's
// zero crossing Delay samples later.
func TestDiscriminatorDelay(t *testing.T) {
	for _, rate := range []float64{9600, 11025, 48000} {
		c := at(bell202, rate)
		d := must(NewDiscriminator[float64, float64](c))
		m := must(NewModulator[float64](c))
		levels := []byte{1, 1, 1, 0, 0, 0}
		x := make([]float64, m.OutputLen(len(levels)))
		m.Process(x, levels)
		soft := make([]float64, len(x))
		d.Process(soft, x)
		change := int(m.end(3)) // the first sample of the fourth symbol
		cross := -1
		for i := change; i < len(soft); i++ {
			if soft[i] < 0 {
				cross = i
				break
			}
		}
		if diff := cross - change - d.Delay(); diff < -1 || diff > 1 {
			t.Errorf("%v Hz: the output crosses zero %d samples after the change, Delay says %d",
				rate, cross-change, d.Delay())
		}
		t.Logf("%v Hz: crossing after %d samples, Delay %d", rate, cross-change, d.Delay())
	}
}
