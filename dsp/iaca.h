#define IACA_SSC_MARK(MARK_ID) \
	BYTE $0xBB; BYTE MARK_ID; BYTE $0x00; BYTE $0x00; BYTE $0x00 \
	BYTE $0x64; BYTE $0x67; BYTE $0x90
#define IACA_UD_BYTES BYTE $0x0F; BYTE $0x0B
#define IACA_START IACA_UD_BYTES; IACA_SSC_MARK($111)
#define IACA_END IACA_SSC_MARK($222); IACA_UD_BYTES
