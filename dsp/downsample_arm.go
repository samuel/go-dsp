//go:build arm

package dsp

import "unsafe"

// downsample_arm.s and demod_arm.s read these structs at fixed offsets off the
// pointer they are handed, so the field layout is part of that assembly's ABI:
// reordering a field, or widening one, silently changes which word the
// assembly loads. The offsets are therefore asserted here rather than left to
// a test that would have to run on the architecture to say anything.
//
// A wrong offset makes the difference below non-zero, and -uint of a non-zero
// constant does not fit a uint, so the package stops compiling for arm.
const (
	_ = -uint(unsafe.Offsetof(BoxcarDecimator{}.downsample))
	_ = -uint(unsafe.Offsetof(BoxcarDecimator{}.now) - 4)
	_ = -uint(unsafe.Offsetof(BoxcarDecimator{}.prevIndex) - 12)

	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator{}.fast))
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator{}.slow) - 4)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator{}.sum) - 8)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator{}.count) - 12)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator{}.prevIndex) - 16)

	_ = -uint(unsafe.Offsetof(FMDemod{}.pre))
)
