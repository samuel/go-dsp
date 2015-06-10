package ax25

import (
	"fmt"
)

type PID byte

const (
	ISO8208CCITTX25PLP     PID = 0x01 // ISO 8208/CCITT X.25 PLP
	CompressedTCPIP        PID = 0x06 // Compressed TCP/IP. RFC 1144
	UncompressedTCPIP      PID = 0x07 // Uncompressed TCP/IP
	SegmentationFragment   PID = 0x08 // Segmentation fragment
	TEXNETDatagramProtocol PID = 0xc3 // TEXNET database protocol
	LinkQualityProtocol    PID = 0xc4 // Link Quality Protocol
	AppleTalk              PID = 0xca // Appletalk
	AppletalkARP           PID = 0xcb // Appletalk ARP
	ARPAInternetProtocol   PID = 0xcc // ARPA Internet Protocol
	ARPAAddressResolution  PID = 0xcd // ARPA Address Resolution
	FlexNet                PID = 0xce // FlexNet
	NETROM                 PID = 0xcf // NET/ROM
	NoLayer3Protocol       PID = 0xf0 // No Layer 3 Protocol Implemented
)

var pidToString = map[PID]string{
	ISO8208CCITTX25PLP:     "ISO 8208/CCITT X.25 PLP",
	CompressedTCPIP:        "Compressed TCP/IP. RFC 1144",
	UncompressedTCPIP:      "Uncompressed TCP/IP",
	SegmentationFragment:   "Segmentation fragment",
	TEXNETDatagramProtocol: "TEXNET database protocol",
	LinkQualityProtocol:    "Link Quality Protocol",
	AppleTalk:              "Appletalk",
	AppletalkARP:           "Appletalk ARP",
	ARPAInternetProtocol:   "ARPA Internet Protocol",
	ARPAAddressResolution:  "ARPA Address Resolution",
	FlexNet:                "FlexNet",
	NETROM:                 "NET/ROM",
	NoLayer3Protocol:       "No Layer 3 Protocol Implemented",
}

func (pid PID) String() string {
	if s := pidToString[pid]; s != "" {
		return s
	}
	return fmt.Sprintf("%02x", int(pid))
}

type FrameType byte

const (
	IFrame FrameType = 0 // Information frame
	SFrame FrameType = 1 // Supervisory frame
	UFrame FrameType = 2 // Unnumbered frame
)

func (t FrameType) String() string {
	switch t {
	case IFrame:
		return "I"
	case SFrame:
		return "S"
	case UFrame:
		return "U"
	}
	return fmt.Sprintf("%02x", int(t))
}

type UnnumberedType byte

const (
	SABME UnnumberedType = 0x6f // Set Async Balanced Mode
	SABM  UnnumberedType = 0x2f // Set Async Balanced Mode
	DISC  UnnumberedType = 0x43 // Disconnect
	DM    UnnumberedType = 0x0f // Disconnect Mode
	UA    UnnumberedType = 0x63 // Unnumbered Acknowledge
	FRMR  UnnumberedType = 0x87 // Frame Reject
	UI    UnnumberedType = 0x03 // Unnumbered Information
	XID   UnnumberedType = 0xaf // Exchange Identification
	TEST  UnnumberedType = 0xe3 // Test
)

var (
	UnnumberedTypeName = map[UnnumberedType]string{
		SABME: "SABME",
		SABM:  "SABM",
		DISC:  "DISC",
		DM:    "DM",
		UA:    "UA",
		FRMR:  "FRMR",
		UI:    "UI",
		XID:   "XID",
		TEST:  "TEST",
	}
)

func (t UnnumberedType) String() string {
	if s := UnnumberedTypeName[t]; s != "" {
		return s
	}
	return fmt.Sprintf("%02x", int(t))
}

type SupervisoryType byte

const (
	RR   SupervisoryType = 0x1 // Receive Ready
	RNR  SupervisoryType = 0x5 // Receive Not Ready
	REJ  SupervisoryType = 0x9 // Reject
	SREJ SupervisoryType = 0xd // Selective Reject
)

func (t SupervisoryType) String() string {
	switch t {
	case RR:
		return "RR"
	case RNR:
		return "RNR"
	case REJ:
		return "REJ"
	case SREJ:
		return "SREJ"
	}
	return fmt.Sprintf("%02x", int(t))
}
