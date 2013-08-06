package borip

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
)

var ErrUnexpectedResponse = errors.New("borip: unexpected resposne from server")

type ErrResponse struct {
	errorType string
	msg       string
}

func (e ErrResponse) Error() string {
	return fmt.Sprintf("borip: %s %s", e.errorType, e.msg)
}

func makeErrorResponse(parts []string) ErrResponse {
	if len(parts) == 1 {
		return ErrResponse{parts[0], ""}
	}
	return ErrResponse{parts[0], strings.Join(parts[1:], " ")}
}

type Device struct {
	Name, Serial               string
	MinGain, MaxGain, GainStep float64
	FPGAFreq                   float64 // Hz
	SamplesPerPacket           int     // complex 4-byte samples (16-bit I/Q) per packet
	ValidAntennas              []string
}

func parseDeviceString(st string) (*Device, error) {
	// Terratec NOXON (rev 3)|-5.000000|30.000000|1.000000|3200000.000000|16256|(Default)|Terratec NOXON (rev 3)
	deviceParts := strings.Split(st, "|")
	if len(deviceParts) < 8 {
		return nil, ErrUnexpectedResponse
	}
	d := &Device{
		Name:   deviceParts[0],
		Serial: deviceParts[7],
	}
	var err error
	if d.MinGain, err = strconv.ParseFloat(deviceParts[1], 64); err != nil {
		return nil, err
	}
	if d.MaxGain, err = strconv.ParseFloat(deviceParts[2], 64); err != nil {
		return nil, err
	}
	if d.GainStep, err = strconv.ParseFloat(deviceParts[3], 64); err != nil {
		return nil, err
	}
	if d.FPGAFreq, err = strconv.ParseFloat(deviceParts[4], 64); err != nil {
		return nil, err
	}
	if val, err := strconv.ParseInt(deviceParts[5], 10, 32); err != nil {
		return nil, err
	} else {
		d.SamplesPerPacket = int(val)
	}
	d.ValidAntennas = strings.Split(deviceParts[6], ",")
	return d, nil
}

type BorIP struct {
	conn    net.Conn
	rd      *bufio.Reader
	wr      *bufio.Writer
	running bool // After a successful "GO" is sent to the server

	device *Device
}

func Dial(addr string) (*BorIP, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	bor := &BorIP{
		conn: conn,
		rd:   bufio.NewReader(conn),
		wr:   bufio.NewWriter(conn),
	}
	line, err := bor.rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line != "DEVICE -" {
		log.Printf("Unexpected hello from server: %s", line)
	}
	return bor, nil
}

func (bor *BorIP) SelectDevice(hint string) (*Device, error) {
	res, err := bor.command("DEVICE", hint)
	if err != nil {
		return nil, err
	}
	resParts := strings.SplitN(res, " ", 2)
	if resParts[0] != "DEVICE" || len(resParts) < 2 {
		return nil, ErrUnexpectedResponse
	}
	if resParts[1][0] == '-' {
		if len(resParts[1]) == 1 {
			// Probably selected ! which is not an error. Just deselects the device.
			bor.device = nil
			return nil, nil
		}
		return nil, errors.New("borip: " + strings.TrimSpace(resParts[1][1:]))
	}
	dev, err := parseDeviceString(resParts[1])
	if err != nil {
		return nil, err
	}
	bor.device = dev
	return dev, nil
}

func (bor *BorIP) Device() *Device {
	return bor.device
}

func (bor *BorIP) SetFrequency(freq float64) (targetIF, actualIF, targetDDC, actualDDC float64, err error) {
	res, e := bor.command("FREQ", strconv.FormatFloat(freq, 'f', -1, 64))
	if e != nil {
		err = e
		return
	}
	resParts, e := parseAndCheck(res, 1)
	if e != nil {
		err = e
		return
	}
	if len(resParts) >= 3 {
		targetIF, _ = strconv.ParseFloat(resParts[2], 64)
	}
	if len(resParts) >= 4 {
		actualIF, _ = strconv.ParseFloat(resParts[3], 64)
	}
	if len(resParts) >= 5 {
		targetDDC, _ = strconv.ParseFloat(resParts[4], 64)
	}
	if len(resParts) >= 6 {
		actualDDC, _ = strconv.ParseFloat(resParts[5], 64)
	}
	return
}

func (bor *BorIP) Frequency() (float64, error) {
	res, err := bor.command("FREQ")
	if err != nil {
		return 0.0, err
	}
	if len(res) < 6 {
		return 0.0, ErrUnexpectedResponse
	}
	return strconv.ParseFloat(res[5:], 64)
}

func (bor *BorIP) SetAntenna(ant string) error {
	res, err := bor.command("ANTENNA", ant)
	if err != nil {
		return err
	}
	_, err = parseAndCheck(res, 1)
	return err
}

func (bor *BorIP) Antenna() (string, error) {
	res, err := bor.command("ANTENNA")
	if err != nil {
		return "", err
	}
	if len(res) < 8 {
		return "", ErrUnexpectedResponse
	}
	return res[8:], nil
}

// Return the actual sampling rate (closest)
func (bor *BorIP) SetRate(rate float64) (float64, error) {
	res, err := bor.command("RATE", strconv.FormatFloat(rate, 'f', -1, 64))
	if err != nil {
		return 0.0, err
	}
	parts, err := parseAndCheck(res, 1)
	if err != nil {
		return 0.0, err
	}
	return strconv.ParseFloat(parts[2], 64)
}

func (bor *BorIP) Rate() (float64, error) {
	res, err := bor.command("RATE")
	if err != nil {
		return 0.0, err
	}
	if len(res) < 6 {
		return 0.0, ErrUnexpectedResponse
	}
	return strconv.ParseFloat(res[5:], 64)
}

// Return the actual gain (closest)
func (bor *BorIP) SetGain(rate float64) error {
	res, err := bor.command("GAIN", strconv.FormatFloat(rate, 'f', -1, 64))
	if err != nil {
		return err
	}
	_, err = parseAndCheck(res, 1)
	return err
}

func (bor *BorIP) Gain() (float64, error) {
	res, err := bor.command("GAIN")
	if err != nil {
		return 0.0, err
	}
	if len(res) < 6 {
		return 0.0, ErrUnexpectedResponse
	}
	return strconv.ParseFloat(res[5:], 64)
}

func (bor *BorIP) SetDestination(dest string) (string, error) {
	res, err := bor.command("DEST", dest)
	if err != nil {
		return "", err
	}
	parts, err := parseAndCheck(res, 2)
	if err != nil {
		return "", err
	}
	return parts[2], nil
}

func (bor *BorIP) Destination() (string, error) {
	res, err := bor.command("DEST")
	if err != nil {
		return "", err
	}
	if len(res) < 5 {
		return "", ErrUnexpectedResponse
	}
	return res[5:], nil
}

func (bor *BorIP) SetHeaderEnabled(enabled bool) error {
	enabledStr := "OFF"
	if enabled {
		enabledStr = "ON"
	}
	res, err := bor.command("HEADER", enabledStr)
	if err != nil {
		return err
	}
	_, err = parseAndCheck(res, 1)
	return err
}

func (bor *BorIP) HeaderEnabled() (bool, error) {
	res, err := bor.command("HEADER")
	if err != nil {
		return false, err
	}
	if len(res) < 8 {
		return false, ErrUnexpectedResponse
	}
	switch res[7:] {
	case "ON":
		return true, nil
	case "OFF":
		return false, nil
	}
	return false, ErrUnexpectedResponse
}

func (bor *BorIP) Go() error {
	res, err := bor.command("GO")
	if err != nil {
		return err
	}
	_, err = parseAndCheck(res, 1)
	if err == nil {
		bor.running = true
	}
	return err
}

func (bor *BorIP) Stop() error {
	res, err := bor.command("STOP")
	if err != nil {
		return err
	}
	_, err = parseAndCheck(res, 1)
	if err == nil {
		bor.running = false
	}
	return err
}

func parseAndCheck(res string, minArgs int) ([]string, error) {
	resParts := strings.Split(res, " ")
	if len(resParts) < 2 {
		return nil, ErrUnexpectedResponse
	}
	if resParts[1] != "OK" {
		return nil, makeErrorResponse(resParts[1:])
	}
	if len(resParts) < 1+minArgs {
		return nil, ErrUnexpectedResponse
	}
	return resParts, nil
}

func (bor *BorIP) command(cmd string, args ...string) (string, error) {
	argString := ""
	if len(args) > 0 {
		argString = " " + strings.Join(args, " ")
	}
	if _, err := bor.wr.WriteString(cmd + argString + "\n"); err != nil {
		return "", err
	}
	if err := bor.wr.Flush(); err != nil {
		return "", err
	}
	line, err := bor.rd.ReadString('\n')
	return strings.TrimSpace(line), err
}

func (bor *BorIP) Close() {
	if bor.running {
		bor.Stop()
	}
	if bor.device != nil {
		bor.SelectDevice("!")
	}
	bor.conn.Close()
}
