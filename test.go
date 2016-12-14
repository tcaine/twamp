package twamp

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/ipv4"
	"log"
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

	// RFC recommends IP TTL of 255
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
func (t *TwampTest) Run() (*TwampResults, error) {

	senderSeqNum := t.seq

	size := t.sendTestMessage(false)

	// receive test packets
	buffer, err := readFromSocket(t.GetConnection(), 64)
	if err != nil {
		//		log.Printf("Read error: %s\n", err)
		return nil, err
	}

	finished := time.Now()

	// process test results
	r := &TwampResults{}
	r.SenderSize = size
	r.SeqNum = binary.BigEndian.Uint32(buffer.Next(4))

	r.Timestamp = NewTimestamp(
		binary.BigEndian.Uint32(buffer.Next(4)),
		binary.BigEndian.Uint32(buffer.Next(4)),
	)

	r.ErrorEstimate = binary.BigEndian.Uint16(buffer.Next(2))
	_ = buffer.Next(2)
	r.ReceiveTimestamp = NewTimestamp(
		binary.BigEndian.Uint32(buffer.Next(4)),
		binary.BigEndian.Uint32(buffer.Next(4)),
	)
	r.SenderSeqNum = binary.BigEndian.Uint32(buffer.Next(4))
	r.SenderTimestamp = NewTimestamp(
		binary.BigEndian.Uint32(buffer.Next(4)),
		binary.BigEndian.Uint32(buffer.Next(4)),
	)
	r.SenderErrorEstimate = binary.BigEndian.Uint16(buffer.Next(2))
	_ = buffer.Next(2)
	r.SenderTTL = byte(buffer.Next(1)[0])
	r.FinishedTimestamp = finished

	if senderSeqNum != r.SeqNum {
		return nil, errors.New(
			fmt.Sprintf("Expected seq # %d but received %d.\n", senderSeqNum, r.SeqNum),
		)
	}

	return r, nil
}

func (t *TwampTest) sendTestMessage(use_all_zeroes bool) int {
	now := NewTwampTimestamp(time.Now())
	totalSize := 14 + int(t.GetSession().config.Padding)
	var pdu []byte = make([]byte, totalSize)

	binary.BigEndian.PutUint32(pdu[0:], t.seq)        // sequence number
	binary.BigEndian.PutUint32(pdu[4:], now.Integer)  // timestamp (integer)
	binary.BigEndian.PutUint32(pdu[8:], now.Fraction) // timestamp (fraction)
	pdu[12] = byte(1)                                 // Synchronized, MBZ, Scale
	pdu[13] = byte(1)                                 // multiplier

	// seed psuedo-random number generator if requested
	if !use_all_zeroes {
		rand.NewSource(int64(time.Now().Unix()))
	}

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

func (t *TwampTest) FormatJSON(r *PingResults) {
	doc, err := json.Marshal(r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", string(doc))
}

func (t *TwampTest) Ping(count int, isRapid bool, interval int) *PingResults {
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	var TotalRTT time.Duration = 0

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	for i := 0; i < count; i++ {
		Stats.Transmitted++
		results, err := t.Run()
		if err != nil {
			if isRapid {
				fmt.Printf(".")
			}
		} else {
			if i == 0 {
				Stats.Min = results.GetRTT()
				Stats.Max = results.GetRTT()
			}
			if Stats.Min > results.GetRTT() {
				Stats.Min = results.GetRTT()
			}
			if Stats.Max < results.GetRTT() {
				Stats.Max = results.GetRTT()
			}

			TotalRTT += results.GetRTT()
			Stats.Received++
			Results.Results = append(Results.Results, results)

			if isRapid {
				fmt.Printf("!")
			} else {
				fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms\n",
					packetSize,
					t.GetRemoteTestHost(),
					results.SenderSeqNum,
					results.SenderTTL,
					(float64(results.GetRTT()) / float64(time.Millisecond)),
				)
			}
		}

		if !isRapid {
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}

	if isRapid {
		fmt.Printf("\n")
	}

	Stats.Avg = time.Duration(int64(TotalRTT) / int64(count))
	Stats.Loss = float64(float64(Stats.Transmitted-Stats.Received)/float64(Stats.Transmitted)) * 100.0
	Stats.StdDev = Results.stdDev(Stats.Avg)

	fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
	fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n",
		Stats.Transmitted,
		Stats.Received,
		Stats.Loss)
	fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
		(float64(Stats.Min) / float64(time.Millisecond)),
		(float64(Stats.Avg) / float64(time.Millisecond)),
		(float64(Stats.Max) / float64(time.Millisecond)),
		(float64(Stats.StdDev) / float64(time.Millisecond)),
	)

	return Results
}

func (t *TwampTest) RunX(count int) *PingResults {
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	var TotalRTT time.Duration = 0

	for i := 0; i < count; i++ {
		Stats.Transmitted++
		results, err := t.Run()
		if err != nil {
		} else {
			if i == 0 {
				Stats.Min = results.GetRTT()
				Stats.Max = results.GetRTT()
			}
			if Stats.Min > results.GetRTT() {
				Stats.Min = results.GetRTT()
			}
			if Stats.Max < results.GetRTT() {
				Stats.Max = results.GetRTT()
			}

			TotalRTT += results.GetRTT()
			Stats.Received++
			Results.Results = append(Results.Results, results)
		}
	}

	Stats.Avg = time.Duration(int64(TotalRTT) / int64(count))
	Stats.Loss = float64(float64(Stats.Transmitted-Stats.Received)/float64(Stats.Transmitted)) * 100.0
	Stats.StdDev = Results.stdDev(Stats.Avg)

	return Results
}
