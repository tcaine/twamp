package twamp

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"time"
)

type TwampConnection struct {
	conn net.Conn
}

func NewTwampConnection(conn net.Conn) *TwampConnection {
	return &TwampConnection{conn: conn}
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
	buffer, err := readFromSocket(c.conn, 64)
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
	Port    int
	Padding int
	Timeout int
	TOS     int
}

func (c *TwampConnection) getTwampServerStartMessage() (*TwampServerStart, error) {
	// check the start message from TWAMP server
	buffer, err := readFromSocket(c.conn, 48)
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
const (
	command         = 0
	senderPort      = 12
	receiverPort    = 14
	paddingLength   = 64
	startTime       = 68
	timeout         = 76
	typePDescriptor = 84
)

type RequestTwSession []byte

func (b RequestTwSession) Encode(c TwampSessionConfig) {
	start_time := NewTwampTimestamp(time.Now())
	b[command] = byte(5)
	binary.BigEndian.PutUint16(b[senderPort:], 6666)
	binary.BigEndian.PutUint16(b[receiverPort:], uint16(c.Port))
	binary.BigEndian.PutUint32(b[paddingLength:], uint32(c.Padding))
	binary.BigEndian.PutUint32(b[startTime:], start_time.Integer)
	binary.BigEndian.PutUint32(b[startTime+4:], start_time.Fraction)
	binary.BigEndian.PutUint32(b[timeout:], uint32(c.Timeout))
	binary.BigEndian.PutUint32(b[timeout+4:], 0)
	binary.BigEndian.PutUint32(b[typePDescriptor:], uint32(c.TOS))
}

func (c *TwampConnection) CreateSession(config TwampSessionConfig) (*TwampSession, error) {
	var pdu RequestTwSession = make(RequestTwSession, 112)

	pdu.Encode(config)

	c.GetConnection().Write(pdu)

	acceptBuffer, err := readFromSocket(c.GetConnection(), 48)
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
