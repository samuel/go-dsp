
TEXT ·lowPassDownsampleComplexFilterAsm(SB),7,$0
	MOVW	fi+0(FP), R3
	MOVW	0(R3), R8	// fi.Downsample
	MOVW	12(R3), R7	// fi.prevIndex
	MOVW   	samples_len+8(FP), R2
	MOVW	samples_data+4(FP), R5 // input
	MOVW	R5, R6		// output
	MOVF	4(R3), F0	// real(fi.now)
	MOVF	8(R3), F1	// imag(fi.now)
	MOVW	$0, R4		// i
	B	loopStart
loop:
	ADD    	$1, R4
loopStart:
	CMP    	R4, R2
	BLE    	loopEnd

	// samples[i]
	MOVF   	0(R5), F2	// real
	MOVF   	4(R5), F3	// imag
	ADD	$8, R5

	// fi.now += samples[i]
	ADDF   	F2, F0
	ADDF   	F3, F1

	// fi.prevIndex++
	ADD    	$1, R7

	// if prevIndex < downsample: continue
	CMP    	R8, R7
	BLT    	loop

	// samples[i2] = fi.now
	MOVF   	F0, 0(R6)
	MOVF   	F1, 4(R6)
	ADD	$8, R6

	// fi.prevIndex = 0
	MOVW   	$0, R7

	// fi.now = 0.0
	MOVF   	$0.0, F0
	MOVF   	$0.0, F1

	B      	loop
loopEnd:
	MOVW   	R7, 12(R3)	// fi.prevIndex
	MOVF   	F0, 4(R3)	// real(fi.now)
	MOVF   	F1, 8(R3)	// imag(fi.now)

	MOVW	samples_data+4(FP), R0
	SUB	R0, R6
	MOVW	R6>>3, R6
	MOVW   	R6, ret_len+20(FP)
	MOVW   	samples_cap+12(FP),R4
	MOVW   	R4, ret_cap+24(FP)
	MOVW   	samples_data+4(FP),R0
	MOVW   	R0, ret_data+16(FP)
	MOVW   	$0, R0
	MOVW   	R0, err+28(FP)
	MOVW   	R0, err+32(FP)
	RET
