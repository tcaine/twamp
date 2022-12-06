package twamp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"
)

type TwampConnectionError struct{
	origErr error
}

func (e *TwampConnectionError) Error() string {
	return fmt.Sprintf("TWAMP Connection Lost: %s", e.origErr)
}

type TwampConnection struct {
	conn    net.Conn
	timeout int
}

func NewTwampConnection(conn net.Conn) *TwampConnection {
	return &TwampConnection{conn: conn, timeout: 5}
}

func isNetConnClosedErr(err error) bool {
	switch {
	case
		errors.Is(err, net.ErrClosed),
		errors.Is(err, io.EOF),
		errors.Is(err, syscall.EPIPE):
		return true
	default:
		return false
	}
}

func (c *TwampConnection) TestConnection() error {
	oneByte := make([]byte, 1)
	c.conn.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	if _, err := c.conn.Read(oneByte); err != nil {
		if isNetConnClosedErr(err) {
			c.conn.Close()
			return &TwampConnectionError{origErr: err}
		}
	}
	c.conn.SetReadDeadline(time.Time{})
	return nil
}

func (c *TwampConnection) GetConnection() net.Conn {
	return c.conn
}

func (c *TwampConnection) Close() {
	c.GetConnection().Close()
}

func (c *TwampConnection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *TwampConnection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

/*
TWAMP client session negotiation message.
*/
type TwampClientSetUpResponse struct {
	Mode     uint32
	KeyID    [80]byte
	Token    [64]byte
	ClientIV [16]byte
}

/*
TWAMP server greeting message.
*/
type TwampServerGreeting struct {
	Mode      uint32   // modes (4 bytes)
	Challenge [16]byte // challenge (16 bytes)
	Salt      [16]byte // salt (16 bytes)
	Count     uint32   // count (4 bytes)
}

func (c *TwampConnection) sendTwampClientSetupResponse() {
	// negotiate TWAMP session configuration
	response := &TwampClientSetUpResponse{}
	response.Mode = ModeUnauthenticated
	binary.Write(c.GetConnection(), binary.BigEndian, response)
}

func (c *TwampConnection) getTwampServerGreetingMessage() (*TwampServerGreeting, error) {
	// check the greeting message from TWAMP server
	buffer, err := readFromSocket(c.conn, 64, c.timeout)
	if err != nil {
		log.Printf("Cannot read: %s\n", err)
		return nil, err
	}

	// decode the TwampServerGreeting PDU
	greeting := &TwampServerGreeting{}
	_ = buffer.Next(12)
	greeting.Mode = binary.BigEndian.Uint32(buffer.Next(4))
	copy(greeting.Challenge[:], buffer.Next(16))
	copy(greeting.Salt[:], buffer.Next(16))
	greeting.Count = binary.BigEndian.Uint32(buffer.Next(4))

	return greeting, nil
}

type TwampServerStart struct {
	Accept    byte
	ServerIV  [16]byte
	StartTime TwampTimestamp
}

type TwampSessionConfig struct {
	// According to RFC 4656, if Conf-Receiver is not set, Receiver Port
	// is the UDP port OWAMP-Test to which packets are
	// requested to be sent.
	ReceiverPort int
	// According to RFC 4656, if Conf-Sender is not set, Sender Port is the
	// UDP port from which OWAMP-Test packets will be sent.
	SenderPort int
	// According to RFC 4656, Padding length is the number of octets to be
	// appended to the normal OWAMP-Test packet (see more on
	// padding in discussion of OWAMP-Test).
	Padding int
	// According to RFC 4656, Timeout (or a loss threshold) is an interval of time
	// (expressed as a timestamp). A packet belonging to the test session
	// that is being set up by the current Request-Session command will
	// be considered lost if it is not received during Timeout seconds
	// after it is sent.
	Timeout int
	TOS     int
}

func (c *TwampConnection) getTwampServerStartMessage() (*TwampServerStart, error) {
	// check the start message from TWAMP server
	buffer, err := readFromSocket(c.conn, 48, c.timeout)
	if err != nil {
		log.Printf("Cannot read: %s\n", err)
		return nil, err
	}

	// decode the TwampServerStart PDU
	start := &TwampServerStart{}
	_ = buffer.Next(15)
	start.Accept = byte(buffer.Next(1)[0])
	copy(start.ServerIV[:], buffer.Next(16))
	start.StartTime.Integer = binary.BigEndian.Uint32(buffer.Next(4))
	start.StartTime.Fraction = binary.BigEndian.Uint32(buffer.Next(4))

	return start, nil
}

/* Byte offsets for Request-TW-Session TWAMP PDU */
const (	// TODO these constants should be removed as part of a refactor when control changel messages are refactored to use "struct based" messaging which is clearer
	offsetRequestTwampSessionCommand         = 0
	offsetRequestTwampSessionIpVersion       = 1
	offsetRequestTwampSessionSenderPort      = 12
	offsetRequestTwampSessionReceiverPort    = 14
	offsetRequestTwampSessionPaddingLength   = 64
	offsetRequestTwampSessionStartTime       = 68
	offsetRequestTwampSessionTimeout         = 76
	offsetRequestTwampSessionTypePDescriptor = 84
)

type RequestTwSession []byte

func (b RequestTwSession) Encode(c TwampSessionConfig) {
	start_time := NewTwampTimestamp(time.Now())
	b[offsetRequestTwampSessionCommand] = byte(5)
	b[offsetRequestTwampSessionIpVersion] = byte(4) // As per RFC, this value can be 4 (IPv4) or 6 (IPv6).
	binary.BigEndian.PutUint16(b[offsetRequestTwampSessionSenderPort:], uint16(c.SenderPort))
	binary.BigEndian.PutUint16(b[offsetRequestTwampSessionReceiverPort:], uint16(c.ReceiverPort))
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionPaddingLength:], uint32(c.Padding))
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionStartTime:], start_time.Integer)
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionStartTime+4:], start_time.Fraction)
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionTimeout:], uint32(c.Timeout))
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionTimeout+4:], 0)
	binary.BigEndian.PutUint32(b[offsetRequestTwampSessionTypePDescriptor:], uint32(c.TOS))
}

func (c *TwampConnection) CreateSession(config TwampSessionConfig) (*TwampSession, error) {
	var pdu RequestTwSession = make(RequestTwSession, 112)

	pdu.Encode(config)

	c.GetConnection().Write(pdu)

	acceptBuffer, err := readFromSocket(c.GetConnection(), 48, 5)
	if err != nil {
		log.Printf("Cannot read: %s\n", err)
		return nil, err
	}

	acceptSession := NewTwampAcceptSession(acceptBuffer)

	err = checkAcceptStatus(int(acceptSession.accept), "session")
	if err != nil {
		return nil, err
	}

	session := &TwampSession{conn: c, port: acceptSession.port, config: config}

	return session, nil
}

type TwampAcceptSession struct {
	accept byte
	port   uint16
	sid    [16]byte
}

func NewTwampAcceptSession(buf bytes.Buffer) *TwampAcceptSession {
	message := &TwampAcceptSession{}
	message.accept = byte(buf.Next(1)[0])
	_ = buf.Next(1) // mbz
	message.port = binary.BigEndian.Uint16(buf.Next(2))
	copy(message.sid[:], buf.Next(16))
	return message
}
