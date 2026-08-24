package ax25

import (
	"fmt"
)

// PID is the AX.25 protocol identifier, which says what layer 3 protocol an
// information frame carries.
type PID byte

const (
	ISO8208CCITTX25PLP     PID = 0x01 // ISO 8208/CCITT X.25 PLP
	CompressedTCPIP        PID = 0x06 // Compressed TCP/IP. RFC 1144
	UncompressedTCPIP      PID = 0x07 // Uncompressed TCP/IP
	SegmentationFragment   PID = 0x08 // Segmentation fragment
	TEXNETDatagramProtocol PID = 0xc3 // TEXNET datagram protocol
	LinkQualityProtocol    PID = 0xc4 // Link Quality Protocol
	AppleTalk              PID = 0xca // AppleTalk
	AppleTalkARP           PID = 0xcb // AppleTalk ARP
	ARPAInternetProtocol   PID = 0xcc // ARPA Internet Protocol
	ARPAAddressResolution  PID = 0xcd // ARPA Address Resolution
	FlexNet                PID = 0xce // FlexNet
	NETROM                 PID = 0xcf // NET/ROM
	NoLayer3Protocol       PID = 0xf0 // No Layer 3 Protocol Implemented
)

// String returns the protocol name, or the identifier in hex if it is unknown.
//
// A switch rather than a package-level map, as FrameType.String already is:
// nothing an importer can reorder or rewrite for the whole process, and no
// allocation at init.
func (pid PID) String() string {
	switch pid {
	case ISO8208CCITTX25PLP:
		return "ISO 8208/CCITT X.25 PLP"
	case CompressedTCPIP:
		return "Compressed TCP/IP. RFC 1144"
	case UncompressedTCPIP:
		return "Uncompressed TCP/IP"
	case SegmentationFragment:
		return "Segmentation fragment"
	case TEXNETDatagramProtocol:
		return "TEXNET datagram protocol"
	case LinkQualityProtocol:
		return "Link Quality Protocol"
	case AppleTalk:
		return "AppleTalk"
	case AppleTalkARP:
		return "AppleTalk ARP"
	case ARPAInternetProtocol:
		return "ARPA Internet Protocol"
	case ARPAAddressResolution:
		return "ARPA Address Resolution"
	case FlexNet:
		return "FlexNet"
	case NETROM:
		return "NET/ROM"
	case NoLayer3Protocol:
		return "No Layer 3 Protocol Implemented"
	}
	return fmt.Sprintf("%02x", int(pid))
}

// FrameType is which of the three AX.25 frame formats a frame uses.
type FrameType byte

const (
	IFrame FrameType = 0 // Information frame
	SFrame FrameType = 1 // Supervisory frame
	UFrame FrameType = 2 // Unnumbered frame
)

// String returns "I", "S" or "U".
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

// UnnumberedType is the command or response an unnumbered frame carries.
type UnnumberedType byte

const (
	SABME UnnumberedType = 0x6f // Set Async Balanced Mode Extended, for modulo-128 sequence numbers
	SABM  UnnumberedType = 0x2f // Set Async Balanced Mode
	DISC  UnnumberedType = 0x43 // Disconnect
	DM    UnnumberedType = 0x0f // Disconnect Mode
	UA    UnnumberedType = 0x63 // Unnumbered Acknowledge
	FRMR  UnnumberedType = 0x87 // Frame Reject
	UI    UnnumberedType = 0x03 // Unnumbered Information
	XID   UnnumberedType = 0xaf // Exchange Identification
	TEST  UnnumberedType = 0xe3 // Test
)

// String returns the abbreviation for the type, or its value in hex if it is
// unknown. A switch, for the reason PID.String gives.
func (t UnnumberedType) String() string {
	switch t {
	case SABME:
		return "SABME"
	case SABM:
		return "SABM"
	case DISC:
		return "DISC"
	case DM:
		return "DM"
	case UA:
		return "UA"
	case FRMR:
		return "FRMR"
	case UI:
		return "UI"
	case XID:
		return "XID"
	case TEST:
		return "TEST"
	}
	return fmt.Sprintf("%02x", int(t))
}

// SupervisoryType is the flow control message a supervisory frame carries.
type SupervisoryType byte

const (
	RR   SupervisoryType = 0x1 // Receive Ready
	RNR  SupervisoryType = 0x5 // Receive Not Ready
	REJ  SupervisoryType = 0x9 // Reject
	SREJ SupervisoryType = 0xd // Selective Reject
)

// String returns the abbreviation for the type, or its value in hex if it is
// unknown.
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
