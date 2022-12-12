package twamp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"golang.org/x/net/ipv4"
	"log"
	"math/rand"
	"net"
	"sync"
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
	results map[uint32]*TwampResults
	mutex   sync.RWMutex
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

func (t *TwampTest) dispatch() {
	paddingSize := t.GetSession().config.Padding
	t.mutex.Lock()
	senderSeqNum := t.seq
	r := &TwampResults{
		SenderSeqNum:      senderSeqNum,
		SenderPaddingSize: paddingSize,
	}
	t.results[senderSeqNum] = r
	size, ttl, timestamp := t.ssendTestMessage(true)
	r.SenderSize = size
	r.SenderTTL = byte(ttl)
	r.SenderTimestamp = timestamp
	t.seq++
	t.mutex.Unlock()
}

func (t *TwampTest) runTest(count int, interval time.Duration, done <-chan bool, numTransmitted *int, replyChan chan TwampResults) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	// Setup a ticker that tries to read from the TCP socket every second to make
	// sure it is still alive, so we can re-initialize or stop if it goes down
	tcpTestTicker := time.NewTicker(1 * time.Second)
	defer tcpTestTicker.Stop()
	var ticker *time.Ticker
	ticker = time.NewTicker(interval)
	firstTick := make(chan bool, 1)
	firstTick <- true
	tcpError := make(chan error, 1)
	go func() {
		t.readReplies(replyChan, done)
	}()
	for continuous || *numTransmitted < count {
		// Wait until we are either done (can be via signal), have a TCP error, or it
		// is time to send a new request
		select {
		case <-done:
			return
		case <-tcpTestTicker.C:
			go func() {
				if err := t.GetSession().TestConnection(); err != nil {
					tcpError <- err
				}
			}()
		case err := <- tcpError:
			log.Println(err)
			close(replyChan)
			return
		case <-firstTick:
			*numTransmitted++
			t.dispatch()
		case <-ticker.C:
			if continuous || *numTransmitted < count {
				*numTransmitted++
				t.dispatch()
			}
		}
	}
	select {
	case <- time.After(time.Second * time.Duration(t.GetTimeout())):
		close(replyChan)
	case <-done:
	}
	return
}

/*
Read replies into a *TwampResults reply channel. Run until done signal
*/
func (t *TwampTest) readReplies(results chan TwampResults, done <-chan bool) {
	paddingSize := t.GetSession().config.Padding
	for {
		select {
		case <-done:
			return
		default:
		}
		buffer, err := readFromSocket(t.GetConnection(), (int(unsafe.Sizeof(MeasurementPacket{}))+paddingSize)*2, t.GetTimeout())
		if err != nil {
			continue
		}

		finished := time.Now()

		responseHeader := MeasurementPacket{}
		err = binary.Read(&buffer, binary.BigEndian, &responseHeader)
		if err != nil {
			log.Printf("Failed to deserialize measurement package. %v", err)
		}

		responsePadding := make([]byte, paddingSize, paddingSize)
		receivedPaddignSize, err := buffer.Read(responsePadding)
		if err != nil {
			log.Printf("Error when receiving padding. %v\n", err)
			continue
		}

		if receivedPaddignSize != paddingSize {
			log.Printf("Incorrect padding. Expected padding size was %d but received %d.\n", paddingSize, receivedPaddignSize)
			continue
		}

		// process test results
		t.mutex.Lock()
		r := t.results[responseHeader.Sequence]
		if r == nil {
			log.Printf("Received response with sequence %d, but haven't sent request with that sequence ID\n", responseHeader.Sequence)
			continue
		}
		if !r.FinishedTimestamp.IsZero() {
			r.IsDuplicate = true
		}

		r.SeqNum = responseHeader.Sequence
		r.Timestamp = NewTimestamp(responseHeader.Timestamp)
		r.ErrorEstimate = responseHeader.ErrorEstimate
		r.ReceiveTimestamp = NewTimestamp(responseHeader.ReceiveTimeStamp)
		r.SenderSeqNum = responseHeader.SenderSequence
		r.SenderTimestamp = NewTimestamp(responseHeader.SenderTimeStamp)
		r.SenderErrorEstimate = responseHeader.SenderErrorEstimate
		r.SenderTTL = responseHeader.SenderTtl
		r.FinishedTimestamp = finished
		results <- *r
		t.mutex.Unlock()
	}
}

func (t *TwampTest) SendTest() int {
	return t.sendTestMessage(true)
}

/*
Read a single reply and return it as a *TwampResults
*/
func (t *TwampTest) readReply(size int) (*TwampResults, error) {
	paddingSize := t.GetSession().config.Padding
	// receive test packets - allocate a receive buffer of a size we expect to receive plus a bit to know if we get some garbage
	buffer, err := readFromSocket(t.GetConnection(), (int(unsafe.Sizeof(MeasurementPacket{}))+paddingSize)*2, t.GetTimeout())
	if err != nil {
		return nil, err
	}

	finished := time.Now()

	responseHeader := MeasurementPacket{}
	err = binary.Read(&buffer, binary.BigEndian, &responseHeader)
	if err != nil {
		return nil, fmt.Errorf("Failed to deserialize measurement package. %v", err)
	}

	responsePadding := make([]byte, paddingSize, paddingSize)
	receivedPaddignSize, err := buffer.Read(responsePadding)
	if err != nil {
		return nil, fmt.Errorf("Error when receivin padding. %v\n", err)
	}

	if receivedPaddignSize != paddingSize {
		return nil, fmt.Errorf("Incorrect padding. Expected padding size was %d but received %d.\n", paddingSize, receivedPaddignSize)
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
	return r, nil
}

/*
Run a TWAMP test and return a pointer to the TwampResults.
*/
func (t *TwampTest) Run() (*TwampResults, error) {
	senderSeqNum := t.seq
	size := t.SendTest()
	r, err := t.readReply(size)
	if err != nil {
		return nil, err
	}
	if senderSeqNum > r.SenderSeqNum {
		// Likely just received a packet that has timed out. Read until we are up to date
		for senderSeqNum > r.SenderSeqNum {
			r, err = t.readReply(size)
			if err != nil {
				return nil, err
			}
		}
	}
	if senderSeqNum < r.SenderSeqNum {
		return nil, fmt.Errorf("Expected seq # %d but received %d.\n", senderSeqNum, r.SeqNum)
	}
	return r, nil
}

func (t *TwampTest) ssendTestMessage(padZero bool) (int, byte, time.Time) {
	timestamp := time.Now()
	ttl := byte(87)
	packetHeader := MeasurementPacket{
		Sequence:            t.seq,
		Timestamp:           *NewTwampTimestamp(timestamp),
		ErrorEstimate:       0x0101,
		MBZ:                 0x0000,
		ReceiveTimeStamp:    TwampTimestamp{},
		SenderSequence:      0,
		SenderTimeStamp:     TwampTimestamp{},
		SenderErrorEstimate: 0x0000,
		Mbz:                 0x0000,
		SenderTtl:           ttl,
	}

	// seed psuedo-random number generator if requested
	var padder func() byte
	if !padZero {
		rand.NewSource(int64(time.Now().Unix()))
		padder = func() byte { return byte(rand.Intn(255)) }
	} else {
		padder = func() byte { return 0 }
	}

	paddingSize := t.GetSession().config.Padding
	padding := make([]byte, paddingSize, paddingSize)

	for x := 0; x < paddingSize; x++ {
		padding[x] = padder()
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
	return totalSize, ttl, timestamp
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

func (t *TwampTest) printPingReply(twampResults *TwampResults) {
	duplicateNotice := ""
	packetSize := 14 + t.GetSession().GetConfig().Padding
	if twampResults.IsDuplicate {
		duplicateNotice = " (DUP!)"
	}
	fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms%s\n",
		packetSize,
		t.GetRemoteTestHost(),
		twampResults.SenderSeqNum,
		twampResults.SenderTTL,
		(float64(twampResults.GetRTT()) / float64(time.Millisecond)),
		duplicateNotice,
	)
}

func (t *TwampTest) PingZ(count int, interval time.Duration, done <-chan bool) (*PingResults, error) {
	var totalRTT time.Duration = 0
	var pingResults *PingResults
	var err error
	// Calculate summaries upon returning
	defer func() {
		stats := pingResults.Stat
		stats.Avg = time.Duration(int64(totalRTT) / int64(stats.Transmitted))
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)

		fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
		fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n",
			stats.Transmitted,
			stats.Received,
			stats.Loss)
		fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
			(float64(stats.Min) / float64(time.Millisecond)),
			(float64(stats.Avg) / float64(time.Millisecond)),
			(float64(stats.Max) / float64(time.Millisecond)),
			(float64(stats.StdDev) / float64(time.Millisecond)),
		)
	}()

	// TODO what is this magic 14 constant? Give it a name at least
	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	pingResults, err = t.RunY(count, t.printPingReply, interval, done)
	return pingResults, err
}

func (t *TwampTest) PingY(count int, interval time.Duration, done <-chan bool) (*PingResults, error) {
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate summaries upon returning
	defer func() {
		stats.Avg = time.Duration(int64(totalRTT) / int64(stats.Transmitted))
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)

		fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
		fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n",
			stats.Transmitted,
			stats.Received,
			stats.Loss)
		fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
			(float64(stats.Min) / float64(time.Millisecond)),
			(float64(stats.Avg) / float64(time.Millisecond)),
			(float64(stats.Max) / float64(time.Millisecond)),
			(float64(stats.StdDev) / float64(time.Millisecond)),
		)
	}()

	// TODO what is this magic 14 constant? Give it a name at least
	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	replyChan := make(chan TwampResults, 64)
	receivedEverything := false

	// Run a TWAMP test count times, yield results to replyChan
	go func() {
		t.runTest(count, interval, done, &stats.Transmitted, replyChan)
	}()

	// Run until done signal or we've received everything/timed out
	for !receivedEverything {
		select {
		case <-done:
			return pingResults, nil
		case twampResults, ok := <-replyChan:
			if !ok {
				// Reply channel has been closed
				return pingResults, nil
			}
			duplicateNotice := ""
			if !twampResults.IsDuplicate {
				stats.Received++
			} else {
				duplicateNotice = " (DUP!)"
			}
			if stats.Received == 1 {
				stats.Min = twampResults.GetRTT()
				stats.Max = twampResults.GetRTT()
			}
			if stats.Min > twampResults.GetRTT() {
				stats.Min = twampResults.GetRTT()
			}
			if stats.Max < twampResults.GetRTT() {
				stats.Max = twampResults.GetRTT()
			}

			totalRTT += twampResults.GetRTT()
			pingResults.Results = append(pingResults.Results, &twampResults)

			fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms%s\n",
				packetSize,
				t.GetRemoteTestHost(),
				twampResults.SenderSeqNum,
				twampResults.SenderTTL,
				(float64(twampResults.GetRTT()) / float64(time.Millisecond)),
				duplicateNotice,
			)

			if stats.Transmitted == stats.Received && stats.Transmitted == count {
				receivedEverything = true
			}
		}
	}

	return pingResults, nil
}

func (t *TwampTest) PingX(count int, interval time.Duration, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate summaries upon returning
	defer func() {
		stats.Avg = time.Duration(int64(totalRTT) / int64(stats.Transmitted))
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)

		fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
		fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n",
			stats.Transmitted,
			stats.Received,
			stats.Loss)
		fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
			(float64(stats.Min) / float64(time.Millisecond)),
			(float64(stats.Avg) / float64(time.Millisecond)),
			(float64(stats.Max) / float64(time.Millisecond)),
			(float64(stats.StdDev) / float64(time.Millisecond)),
		)
	}()

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	tcpTestTicker := time.NewTicker(1 * time.Second)
	defer tcpTestTicker.Stop()
	var ticker *time.Ticker
	ticker = time.NewTicker(interval)
	firstTick := make(chan bool, 1)
	firstTick <- true
	tcpError := make(chan error, 1)
	replyChan := make(chan TwampResults, 64)
	inProgress := true
	receivedEverything := false
	go func() {
		t.readReplies(replyChan, done)
	}()
	for continuous || inProgress || !receivedEverything {
		// Wait until next scheduled run or done signal
		select {
		case <-done:
			return pingResults, nil
		case <-tcpTestTicker.C:
			go func() {
				if err := t.GetSession().TestConnection(); err != nil {
					tcpError <- err
				}
			}()
		case err := <- tcpError:
			return pingResults, err
		case <-firstTick:
			stats.Transmitted++
			t.dispatch()
		case <-ticker.C:
			if continuous || stats.Transmitted < count {
				stats.Transmitted++
				t.dispatch()
				continue
			}
			// We've iterated over everything. Are we waiting for a timeout
			// or have we not triggered that yet?
			if inProgress {
				ticker = time.NewTicker(time.Duration(t.GetTimeout()) * time.Second)
				inProgress = false
			} else {
				receivedEverything = true
				ticker.Stop()
			}
		case twampResults := <-replyChan:
			stats.Received++
			if stats.Received == 1 {
				stats.Min = twampResults.GetRTT()
				stats.Max = twampResults.GetRTT()
			}
			if stats.Min > twampResults.GetRTT() {
				stats.Min = twampResults.GetRTT()
			}
			if stats.Max < twampResults.GetRTT() {
				stats.Max = twampResults.GetRTT()
			}

			totalRTT += twampResults.GetRTT()
			pingResults.Results = append(pingResults.Results, &twampResults)

			fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms\n",
				packetSize,
				t.GetRemoteTestHost(),
				twampResults.SenderSeqNum,
				twampResults.SenderTTL,
				(float64(twampResults.GetRTT()) / float64(time.Millisecond)),
			)

			if stats.Transmitted == stats.Received && stats.Transmitted == count {
				receivedEverything = true
				inProgress = false
			}
		}
	}

	return pingResults, nil
}

// Use a blocking ping, pinging as soon as a reply or timeout is hit.
// TODO listen for done signal even while waiting for a reply/timeout as
// opposed to having to check for the signal at the start of each iteration
func (t *TwampTest) PingRapid(count int, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate summaries upon returning
	defer func() {
		stats.Avg = time.Duration(int64(totalRTT) / int64(stats.Transmitted))
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)

		fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
		fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n",
			stats.Transmitted,
			stats.Received,
			stats.Loss)
		fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
			(float64(stats.Min) / float64(time.Millisecond)),
			(float64(stats.Avg) / float64(time.Millisecond)),
			(float64(stats.Max) / float64(time.Millisecond)),
			(float64(stats.StdDev) / float64(time.Millisecond)),
		)
	}()

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	tcpTestTicker := time.NewTicker(1 * time.Second)
	defer tcpTestTicker.Stop()
	tcpError := make(chan error, 1)
	iterations := 0
	for continuous || iterations < count {
		// Wait until next scheduled run or done signal
		select {
		case <-done:
			return pingResults, nil
		case <-tcpTestTicker.C:
			go func() {
				if err := t.GetSession().TestConnection(); err != nil {
					tcpError <- err
				}
			}()
			continue
		case err := <- tcpError:
			return pingResults, err
		default:
		}
		stats.Transmitted++
		twampResults, err := t.Run()
		if err != nil {
			// TODO Do we need error logging here? I guess not because dot represents the sort error message here but should be double checked.
			fmt.Printf(".")
		} else {
			if iterations == 0 {
				stats.Min = twampResults.GetRTT()
				stats.Max = twampResults.GetRTT()
			}
			if stats.Min > twampResults.GetRTT() {
				stats.Min = twampResults.GetRTT()
			}
			if stats.Max < twampResults.GetRTT() {
				stats.Max = twampResults.GetRTT()
			}

			totalRTT += twampResults.GetRTT()
			stats.Received++
			pingResults.Results = append(pingResults.Results, twampResults)

			fmt.Printf("!")
		}
		iterations += 1
	}

	fmt.Printf("\n")
	return pingResults, nil
}

func (t *TwampTest) RunX(count int, callback TwampTestCallbackFunction, interval time.Duration, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	isRapid := interval == time.Duration(0)
	var totalRTT time.Duration = 0

	// Calculate totals upon returning
	defer func() {
		Stats.Avg = time.Duration(int64(totalRTT) / int64(Stats.Transmitted))
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
	tcpError := make(chan error, 1)
	iterations := 0
	for continuous || iterations < count {
		// Wait until next scheduled run or done signal
		select {
		case <-done:
			return Results, nil
		case <-tcpTestTicker.C:
			go func() {
				if err := t.GetSession().TestConnection(); err != nil {
					tcpError <- err
				}
			}()
			continue
		case err := <- tcpError:
			return Results, err
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

			totalRTT += results.GetRTT()
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

func (t *TwampTest) RunY(count int, callback TwampTestCallbackFunction, interval time.Duration, done <-chan bool) (*PingResults, error) {
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate totals upon returning
	defer func() {
		stats.Avg = time.Duration(int64(totalRTT) / int64(stats.Transmitted))
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)
	}()


	// We must use a struct chan instead of a struct pointer chan to
	// make sure that we have a snapshot of the reply received, in case
	// we get a duplicate reply that gets processed before we have a
	// chance to process the last reply, as the underlying map that
	// dispatches requests uses the sequence number as an index and thus
	// we might flag a response as a duplicate before we have a chance
	// to handle the previous one in the loop
	replyChan := make(chan TwampResults, 64)
	receivedEverything := false

	// Run a TWAMP test count times, yield results to replyChan
	go func() {
		t.runTest(count, interval, done, &stats.Transmitted, replyChan)
	}()

	// Run until done signal or we've received everything/timed out
	for !receivedEverything {
		select {
		case <-done:
			return pingResults, nil
		case twampResults, ok := <-replyChan:
			if !ok {
				// Reply channel has been closed
				return pingResults, nil
			}
			if !twampResults.IsDuplicate {
				stats.Received++
			}
			if stats.Received == 1 {
				stats.Min = twampResults.GetRTT()
				stats.Max = twampResults.GetRTT()
			}
			if stats.Min > twampResults.GetRTT() {
				stats.Min = twampResults.GetRTT()
			}
			if stats.Max < twampResults.GetRTT() {
				stats.Max = twampResults.GetRTT()
			}

			totalRTT += twampResults.GetRTT()
			pingResults.Results = append(pingResults.Results, &twampResults)

			if callback != nil {
				callback(&twampResults)
			}
			if stats.Transmitted == stats.Received && stats.Transmitted == count {
				receivedEverything = true
			}
		}
	}

	return pingResults, nil
}
