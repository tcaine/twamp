package client

import (
	"bytes"
	"encoding/binary"
	"log"
	//	"errors"
	"fmt"
	"golang.org/x/net/ipv4"
	"math/rand"
	"net"
	"strings"
	"time"
)

/*
	TWAMP test connection used for running TWAMP tests.
*/
type TwampTest struct {
	session *TwampSession
	conn    *net.UDPConn
	seq     uint32
}

/*

 */
func (t *TwampTest) SetConnection(conn *net.UDPConn) {
	c := ipv4.NewConn(conn)

	err := c.SetTTL(255)
	if err != nil {
		log.Fatal(err)
	}

	err = c.SetTOS(t.GetSession().GetConfig().TOS)
	if err != nil {
		log.Fatal(err)
	}

	t.conn = conn
}

/*
	Get TWAMP Test UDP connection.
*/
func (t *TwampTest) GetConnection() *net.UDPConn {
	return t.conn
}

/*
	Get the underlying TWAMP control session for the TWAMP test.
*/
func (t *TwampTest) GetSession() *TwampSession {
	return t.session
}

/*
	Get the remote TWAMP IP/UDP address.
*/
func (t *TwampTest) RemoteAddr() (*net.UDPAddr, error) {
	address := fmt.Sprintf("%s:%d", t.GetRemoteTestHost(), t.GetRemoteTestPort())
	return net.ResolveUDPAddr("udp", address)
}

/*
	Get the remote TWAMP UDP port number.
*/
func (t *TwampTest) GetRemoteTestPort() uint16 {
	return t.GetSession().port
}

/*
	Get the local IP address for the TWAMP control session.
*/
func (t *TwampTest) GetLocalTestHost() string {
	localAddress := t.session.GetConnection().LocalAddr()
	return strings.Split(localAddress.String(), ":")[0]
}

/*
	Get the remote IP address for the TWAMP control session.
*/
func (t *TwampTest) GetRemoteTestHost() string {
	remoteAddress := t.session.GetConnection().RemoteAddr()
	return strings.Split(remoteAddress.String(), ":")[0]
}

/*
	Run a TWAMP test and return a pointer to the TwampResults.
*/
func (t *TwampTest) Run() (r *TwampResults, err error) {
	size := t.sendTestMessage(false)

	// receive test packets
	buffer, err := t.readFromSocket(64)
	if err != nil {
		//		log.Printf("Read error: %s\n", err)
		return nil, err
	}

	finished := NewTwampTimestamp(time.Now())

	// process test results
	r = &TwampResults{}
	r.SenderSize = size
	r.SeqNum = binary.BigEndian.Uint32(buffer.Next(4))
	r.Timestamp.Integer = binary.BigEndian.Uint32(buffer.Next(4))
	r.Timestamp.Fraction = binary.BigEndian.Uint32(buffer.Next(4))
	r.ErrorEstimate = binary.BigEndian.Uint16(buffer.Next(2))
	_ = buffer.Next(2)
	r.ReceiveTimestamp.Integer = binary.BigEndian.Uint32(buffer.Next(4))
	r.ReceiveTimestamp.Fraction = binary.BigEndian.Uint32(buffer.Next(4))
	r.SenderSeqNum = binary.BigEndian.Uint32(buffer.Next(4))
	r.SenderTimestamp.Integer = binary.BigEndian.Uint32(buffer.Next(4))
	r.SenderTimestamp.Fraction = binary.BigEndian.Uint32(buffer.Next(4))
	r.SenderErrorEstimate = binary.BigEndian.Uint16(buffer.Next(2))
	_ = buffer.Next(2)
	r.SenderTTL = byte(buffer.Next(1)[0])
	r.FinishedTimestamp = *finished

	return
}

func (t *TwampTest) sendTestMessage(use_all_zeroes bool) int {
	now := NewTwampTimestamp(time.Now())
	totalSize := 14 + int(t.GetSession().config.Padding)
	var pdu []byte = make([]byte, totalSize)

	binary.BigEndian.PutUint32(pdu[0:], t.seq)        // sequence number
	binary.BigEndian.PutUint32(pdu[4:], now.Integer)  // timestamp (integer)
	binary.BigEndian.PutUint32(pdu[8:], now.Fraction) // timestamp (fraction)
	pdu[12] = 1<<7 | 0<<6 | 0                         // Synchronized, MBZ, Scale
	pdu[13] = byte(1)                                 // multiplier

	rand.NewSource(int64(time.Now().Unix()))

	for x := 14; x < totalSize; x++ {
		if use_all_zeroes {
			pdu[x] = 0
		} else {
			pdu[x] = byte(rand.Intn(255))
		}
	}

	t.GetConnection().Write(pdu)
	t.seq++
	return totalSize
}

func (t *TwampTest) readFromSocket(size int) (bytes.Buffer, error) {
	buf := make([]byte, size)
	buffer := *bytes.NewBuffer(buf)

	// timeout the UDP socket read
	timeout := time.Duration(t.GetSession().GetConfig().Timeout)
	t.GetConnection().SetReadDeadline(
		time.Now().Add(time.Second * timeout),
	)

	_, err := t.GetConnection().Read(buf)
	if err != nil {
		return buffer, err
	}

	return buffer, nil
}
