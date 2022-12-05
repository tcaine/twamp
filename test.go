package twamp

import (
	"bytes"
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
	"unsafe"
)

/*
TWAMP test connection used for running TWAMP tests.
*/
type TwampTest struct {
	session *TwampSession
	conn    *net.UDPConn
	seq     uint32
	results map[int]*TwampResults
}

/*
Function header called when a test package arrived back.
Can be used to show some progress
 */
type TwampTestCallbackFunction func(result *TwampResults);

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
Get the configured timeout from the underlying TWAMP control session.
*/
func (t *TwampTest) GetTimeout() int {
	return t.GetSession().GetTimeout()
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

type MeasurementPacket struct {
	Sequence uint32
	Timestamp TwampTimestamp
	ErrorEstimate uint16
	MBZ uint16
	ReceiveTimeStamp TwampTimestamp
	SenderSequence uint32
	SenderTimeStamp TwampTimestamp
	SenderErrorEstimate uint16
	Mbz uint16
	SenderTtl byte
	//Padding []byte
}

/*
Run a TWAMP test and return a pointer to the TwampResults.
*/
func (t *TwampTest) Run() (*TwampResults, error) {
	paddingSize := t.GetSession().config.Padding
	senderSeqNum := t.seq

	size := t.sendTestMessage(true)

	// receive test packets - allocate a receive buffer of a size we expect to receive plus a bit to know if we get some garbage
	buffer, err := readFromSocket(t.GetConnection(), (int(unsafe.Sizeof(MeasurementPacket{}))+paddingSize)*2, t.GetTimeout())
	if err != nil {
		return nil, err
	}

	finished := time.Now()

	responseHeader := MeasurementPacket{}
	err = binary.Read(&buffer, binary.BigEndian, &responseHeader)
	if err != nil {
		log.Fatalf("Failed to deserialize measurement package. %v", err)
	}

	responsePadding := make([]byte, paddingSize, paddingSize)
	receivedPaddignSize, err := buffer.Read(responsePadding)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error when receivin padding. %v\n", err))
	}

	if receivedPaddignSize != paddingSize {
		return nil, errors.New(fmt.Sprintf("Incorrect padding. Expected padding size was %d but received %d.\n", paddingSize, receivedPaddignSize))
	}

	// process test results
	r := &TwampResults{}
	r.SenderSize = size
	r.SeqNum = responseHeader.Sequence
	r.Timestamp = NewTimestamp(responseHeader.Timestamp)
	r.ErrorEstimate = responseHeader.ErrorEstimate
	r.ReceiveTimestamp = NewTimestamp(responseHeader.ReceiveTimeStamp)
	r.SenderSeqNum = responseHeader.SenderSequence
	r.SenderTimestamp = NewTimestamp(responseHeader.SenderTimeStamp)
	r.SenderErrorEstimate = responseHeader.SenderErrorEstimate
	r.SenderTTL = responseHeader.SenderTtl
	r.FinishedTimestamp = finished

	if senderSeqNum != r.SenderSeqNum {
		return nil, errors.New(
			fmt.Sprintf("Expected seq # %d but received %d.\n", senderSeqNum, r.SeqNum),
		)
	}

	return r, nil
}

func (t *TwampTest) sendTestMessage(use_all_zeroes bool) int {
	packetHeader := MeasurementPacket{
		Sequence:            t.seq,
		Timestamp:           *NewTwampTimestamp(time.Now()),
		ErrorEstimate:       0x0101,
		MBZ:                 0x0000,
		ReceiveTimeStamp:    TwampTimestamp{},
		SenderSequence:      0,
		SenderTimeStamp:     TwampTimestamp{},
		SenderErrorEstimate: 0x0000,
		Mbz:                 0x0000,
		SenderTtl:           87,
	}

	// seed psuedo-random number generator if requested
	if !use_all_zeroes {
		rand.NewSource(int64(time.Now().Unix()))
	}

	paddingSize := t.GetSession().config.Padding
	padding := make([]byte, paddingSize, paddingSize)

	for x := 0; x < paddingSize; x++ {
		if use_all_zeroes {
			padding[x] = 0
		} else {
			padding[x] = byte(rand.Intn(255))
		}
	}

	var binaryBuffer bytes.Buffer
	err := binary.Write(&binaryBuffer, binary.BigEndian, packetHeader)
	if err != nil {
		log.Fatalf("Failed to serialize measurement package. %v", err)
	}

	headerBytes := binaryBuffer.Bytes()
	headerSize := binaryBuffer.Len()
	totalSize := headerSize+paddingSize
	var pdu []byte = make([]byte, totalSize)
	copy(pdu[0:], headerBytes)
	copy(pdu[headerSize:], padding)

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

func (t *TwampTest) ReturnJSON(r *PingResults) string {
	doc, err := json.Marshal(r)
	if err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%s\n", string(doc))
}

func (t *TwampTest) Ping(count int, interval time.Duration, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	isRapid := interval == time.Duration(0)
	var TotalRTT time.Duration = 0

	// Calculate summaries upon returning
	defer func() {
		Stats.Avg = time.Duration(int64(TotalRTT) / int64(Stats.Transmitted))
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
	}()

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	tcpTestTicker := time.NewTicker(1 * time.Second)
	defer tcpTestTicker.Stop()
	var ticker *time.Ticker
	if !isRapid {
		ticker = time.NewTicker(interval)
	} else {
		// Rapid should run as fast as it can, but in practice having a ticker tick every nanosecond is
		// sufficient, as the code doesn't run fast enough for there to be a difference
		ticker = time.NewTicker(1 * time.Nanosecond)
	}
	defer ticker.Stop()
	firstTick := make(chan bool, 1)
	firstTick <- true
	iterations := 0
	for continuous || iterations < count {
		// Wait until next scheduled run or done signal
		select {
		case <-done:
			return Results, nil
		case <-tcpTestTicker.C:
			if err := t.GetSession().TestConnection(); err != nil {
				return Results, err
			}
			continue
		case <-firstTick:
		case <-ticker.C:
		}
		Stats.Transmitted++
		results, err := t.Run()
		if err != nil {
			// TODO Do we need error logging here? I guess not because dot represents the sort error message here but should be double checked.
			if isRapid {
				fmt.Printf(".")
			}
		} else {
			if iterations == 0 {
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
		iterations += 1
	}

	if isRapid {
		fmt.Printf("\n")
	}

	return Results, nil
}

func (t *TwampTest) RunX(count int, callback TwampTestCallbackFunction, interval time.Duration, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	isRapid := interval == time.Duration(0)
	var TotalRTT time.Duration = 0

	// Calculate totals upon returning
	defer func() {
		Stats.Avg = time.Duration(int64(TotalRTT) / int64(Stats.Transmitted))
		Stats.Loss = float64(float64(Stats.Transmitted-Stats.Received)/float64(Stats.Transmitted)) * 100.0
		Stats.StdDev = Results.stdDev(Stats.Avg)
	}()

	tcpTestTicker := time.NewTicker(1 * time.Second)
	defer tcpTestTicker.Stop()
	var ticker *time.Ticker
	if !isRapid {
		ticker = time.NewTicker(interval)
	} else {
		// Rapid should run as fast as it can, but in practice having a ticker tick every nanosecond is
		// sufficient, as the code doesn't run fast enough for there to be a difference
		ticker = time.NewTicker(1 * time.Nanosecond)
	}
	defer ticker.Stop()
	firstTick := make(chan bool, 1)
	firstTick <- true
	iterations := 0
	for continuous || iterations < count {
		// Wait until next scheduled run or done signal
		select {
		case <-done:
			return Results, nil
		case <-tcpTestTicker.C:
			if err := t.GetSession().TestConnection(); err != nil {
				return Results, err
			}
			continue
		case <-firstTick:
		case <-ticker.C:
		}

		Stats.Transmitted++
		results, err := t.Run()
		if err != nil {
			//log.Printf("%v\n", err)
		} else {
			if iterations == 0 {
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
			if callback != nil {
				callback(results)
			}
		}
		iterations += 1
	}

	return Results, nil
}
