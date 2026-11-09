// This file is a local addition to the vendored copy of the runtime's CPU
// detection, not part of upstream.

//go:build amd64

package cpu

// Signature identifies the CPU model. Family and Model already have the
// extended family and model fields from CPUID leaf 1 folded in, so they match
// the numbers Intel and AMD use in their documentation (and the ones lscpu
// reports).
type Signature struct {
	// Vendor is the CPUID leaf 0 vendor string, e.g. "GenuineIntel" or
	// "AuthenticAMD".
	Vendor   string
	Family   uint32
	Model    uint32
	Stepping uint32
}

// X86Signature returns the signature of the CPU this is running on.
func X86Signature() Signature {
	maxID, ebx, ecx, edx := cpuid(0, 0)

	var vendor []byte
	vendor = appendBytes(vendor, ebx, edx, ecx) // note the ebx, edx, ecx order
	sig := Signature{Vendor: string(vendor)}
	if maxID < 1 {
		return sig
	}

	eax, _, _, _ := cpuid(1, 0)
	fam := familyModel(eax)
	sig.Family, sig.Model, sig.Stepping = fam.Family, fam.Model, fam.Stepping
	return sig
}

// familyModel decodes the family, model and stepping out of CPUID leaf 1's EAX,
// folding in the extended fields. The Vendor of the returned Signature is
// empty; X86Signature fills it in.
func familyModel(eax uint32) Signature {
	var sig Signature
	sig.Stepping = eax & 0xf
	sig.Model = (eax >> 4) & 0xf
	sig.Family = (eax >> 8) & 0xf
	extModel := (eax >> 16) & 0xf
	extFamily := (eax >> 20) & 0xff

	// Intel SDM Vol. 2A, CPUID: the extended model is used for the 06H and
	// 0FH families, and the extended family only for 0FH.
	if sig.Family == 0x6 || sig.Family == 0xf {
		sig.Model += extModel << 4
	}
	if sig.Family == 0xf {
		sig.Family += extFamily
	}
	return sig
}

// appendBytes appends the little-endian bytes of each argument to b.
func appendBytes(b []byte, args ...uint32) []byte {
	for _, arg := range args {
		b = append(b,
			byte(arg>>0),
			byte(arg>>8),
			byte(arg>>16),
			byte(arg>>24))
	}
	return b
}
