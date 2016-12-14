package twamp

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

const (
	BE   = 0x00
	CS1  = 0x20
	AF11 = 0x28
	AF12 = 0x30
	AF13 = 0x38
	CS2  = 0x40
	AF21 = 0x48
	AF22 = 0x50
	AF23 = 0x58
	CS3  = 0x60
	AF31 = 0x68
	AF32 = 0x70
	AF33 = 0x78
	CS4  = 0x80
	AF41 = 0x88
	AF42 = 0x90
	AF43 = 0x98
	CS5  = 0xA0
	EF   = 0xB8
	CS6  = 0xC0
	CS7  = 0xE0
)

type TwampSession struct {
	conn   *TwampConnection
	port   uint16
	config TwampSessionConfig
}

func (s *TwampSession) GetConnection() net.Conn {
	return s.conn.GetConnection()
}

func (s *TwampSession) GetConfig() TwampSessionConfig {
	return s.config
}

func (s *TwampSession) Write(buf []byte) {
	s.GetConnection().Write(buf)
}

func (s *TwampSession) CreateTest() (*TwampTest, error) {
	var pdu []byte = make([]byte, 32)
	pdu[0] = 2

	s.Write(pdu)

	startAckBuffer, err := readFromSocket(s.GetConnection(), 32)
	if err != nil {
		log.Printf("Cannot read: %s\n", err)
		return nil, err
	}

	accept, err := startAckBuffer.ReadByte()
	if err != nil {
		log.Printf("Cannot read: %s\n", err)
		return nil, err
	}

	err = checkAcceptStatus(int(accept), "test setup")
	if err != nil {
		return nil, err
	}

	test := &TwampTest{session: s}
	remoteAddr, err := test.RemoteAddr()
	if err != nil {
		return nil, err
	}
	localAddress := fmt.Sprintf("%s:%d", test.GetLocalTestHost(), s.GetConfig().Port)
	localAddr, err := net.ResolveUDPAddr("udp", localAddress)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	test.SetConnection(conn)

	if err != nil {
		log.Printf("Some error %+v", err)
		return nil, err
	}

	return test, nil
}

func (s *TwampSession) Stop() {
	//	log.Println("Stopping test sessions.")
	var pdu []byte = make([]byte, 32)
	pdu[0] = byte(3)                       // Stop-Sessions Command Number
	pdu[1] = byte(0)                       // Accept Status (0 = OK)
	binary.BigEndian.PutUint16(pdu[4:], 1) // Number of Sessions
	s.GetConnection().Write(pdu)
}
