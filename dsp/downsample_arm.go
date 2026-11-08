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
	_ = -uint(unsafe.Offsetof(BoxcarDecimator[complex64]{}.factor))
	_ = -uint(unsafe.Offsetof(BoxcarDecimator[complex64]{}.now) - 4)
	_ = -uint(unsafe.Offsetof(BoxcarDecimator[complex64]{}.prevIndex) - 12)

	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator[float32]{}.fast))
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator[float32]{}.slow) - 4)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator[float32]{}.sum) - 8)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator[float32]{}.count) - 12)
	_ = -uint(unsafe.Offsetof(RationalBoxcarDecimator[float32]{}.prevIndex) - 16)

	_ = -uint(unsafe.Offsetof(FMDemod[complex64, float32]{}.pre))
)
