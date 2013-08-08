
// TEXT ·fmDemodulateAsm(SB),7,$0
// 	// B ·fmDemodulate(SB)

// 	MOVW   	fi+0(FP),R5
// // 0253 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) FUNCDATA	$0,gc·6+0(SB)
// // 0254 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	fi+0(FP){*"".FMDemodFilter},$4
// // 0255 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	input+4(FP){[]complex64},$12
// // 0256 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	output+16(FP){[]float32},$12
// // 0257 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	~anon3+28(FP){int},$4
// // 0258 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	~anon4+32(FP){error},$8
// // 0259 (/home/pi/workspace/go/src/github.com/samuel/go-sdr/sdr/demod.go:26) TYPE   	i+-4(SP){int},$4
// 	MOVW   	$0,R2
// 	B      	fmDemod_loopStart
// fmDemod_loop:
// 	ADD    	$1,R3,R2
// fmDemod_loopStart:
// 	MOVW   	input+8(FP),R4
// 	MOVW   	R2,R3
// 	CMP    	R2,R4,
// 	BLE    	fmDemod_end
// 	MOVW   	$input+4(FP),R0
// 	MOVW   	R2,i+-4(SP)
// 	MOVW   	R2,R1
// 	MOVW   	0(R0),R0
// 	ADD    	R1<<3,R0
// 	MOVF   	0(R0),F3
// 	MOVF   	4(R0),F2
// 	MOVF   	F3,4(R13)
// 	MOVF   	F2,8(R13)
// 	MOVF   	0(R5),F0
// 	MOVF   	F0,12(R13)
// 	MOVF   	4(R5),F0
// 	MOVF   	F0,16(R13)
// 	BL     	·PolarDiscriminator32(SB)
// 	MOVW   	fi+0(FP),R5
// 	MOVW   	i+-4(SP),R3
// 	MOVF   	20(R13),F2
// 	MOVF   	F2,F4
// 	MOVW   	$input+4(FP),R0
// 	MOVW   	R3,R1
// 	MOVW   	0(R0),R0
// 	ADD    	R1<<3,R0
// 	MOVF   	0(R0),F3
// 	MOVF   	4(R0),F2
// 	MOVF   	F3,0(R5)
// 	MOVF   	F2,4(R5)
// 	MOVW   	$output+16(FP),R0
// 	MOVW   	R3,R1
// 	MOVW   	0(R0),R0
// 	ADD    	R1<<2,R0
// 	MOVF   	F4,0(R0)
// 	B      	fmDemod_loop
// fmDemod_end:
// 	MOVW   	input+8(FP), R0
// 	MOVW   	R0,res+28(FP)
// 	MOVW   	$0, R0
// 	MOVW   	R0, err+32(FP)
// 	MOVW   	R0, err+36(FP)
// 	RET
