// This file is a local addition to the vendored copy of the runtime's CPU
// detection, not part of upstream.

//go:build amd64

package cpu

import "testing"

// TestFamilyModel covers the extended family and model folding, which
// determines the result avx512Downclocks in the dsp package picks (256- against
// 512-bit vectors): a model number that came out as 5 rather than 0x55 would
// silently change which vector width every archsimd path uses.
func TestFamilyModel(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		eax                     uint32
		family, model, stepping uint32
	}{
		// Skylake-SP / Cascade Lake: family 6, model 0x55.
		{"skylake-sp", 0x00050654, 6, 0x55, 4},
		// Ice Lake server: family 6, model 0x6a.
		{"icelake-sp", 0x000606a6, 6, 0x6a, 6},
		// Nehalem, before the extended model mattered much: family 6, model 26.
		{"nehalem", 0x000106a5, 6, 0x1a, 5},
		// AMD Zen 4: family 0xf + extended family 0x19 - 0xf, model 0x61.
		{"zen4", 0x00a60f12, 0x19, 0x61, 2},
		// Family 5 uses neither extended field, so both are dropped.
		{"pentium", 0x00f1f52a, 5, 2, 10},
		// Family 0xf without extended bits stays 0xf.
		{"netburst", 0x00000f24, 0xf, 2, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := familyModel(tc.eax)
			if got.Family != tc.family || got.Model != tc.model || got.Stepping != tc.stepping {
				t.Errorf("familyModel(%#08x) = family %#x model %#x stepping %d, want family %#x model %#x stepping %d",
					tc.eax, got.Family, got.Model, got.Stepping, tc.family, tc.model, tc.stepping)
			}
			if got.Vendor != "" {
				t.Errorf("Vendor = %q, want empty", got.Vendor)
			}
		})
	}
}
