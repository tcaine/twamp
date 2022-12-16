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
	"sync/atomic"
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
This can be called to stop the reply reader instantly because we
have already read everything we want and don't want to block
further reads until timeout is hit, or read from a new iteration
utilizing the same session
*/
func (t *TwampTest) Reset() error {
	localAddr := t.GetConnection().LocalAddr()
	remoteAddr := t.GetConnection().RemoteAddr()
	t.GetConnection().Close()
	conn, err := net.DialUDP("udp", localAddr.(*net.UDPAddr), remoteAddr.(*net.UDPAddr))
	if err != nil {
		return err
	}
	t.SetConnection(conn)
	return nil
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

func (t *TwampTest) sendTestMessageWithMutex() {
	paddingSize := t.GetSession().config.Padding
	t.mutex.Lock()
	r := &TwampResults{
		SenderSeqNum:      t.seq,
		SenderPaddingSize: paddingSize,
	}
	t.results[t.seq] = r
	size, ttl, timestamp := t.putMessageOnWire(true)
	r.SenderSize = size
	r.SenderTTL = byte(ttl)
	r.SenderTimestamp = timestamp
	t.seq++
	t.mutex.Unlock()
}

func (t *TwampTest) runTest(count uint64, interval time.Duration, done <-chan bool, notifyError chan<- error, numTransmitted *uint64, replyChan chan TwampResults, wg *sync.WaitGroup) {
	defer wg.Done()
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
	defer ticker.Stop()
	firstTick := make(chan bool, 1)
	firstTick <- true
	tcpError := make(chan error, 1)
	go func() {
		wg.Add(1)
		t.readReplies(replyChan, done, wg)
	}()
	for continuous || atomic.LoadUint64(numTransmitted) < count {
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
			notifyError<-err
			return
		case <-firstTick:
			t.sendTestMessageWithMutex()
			atomic.AddUint64(numTransmitted, 1)
		case <-ticker.C:
			if continuous || atomic.LoadUint64(numTransmitted) < count {
				t.sendTestMessageWithMutex()
				atomic.AddUint64(numTransmitted, 1)
			}
		}
	}
	return
}

/*
Read replies into a *TwampResults reply channel. Run until done signal
*/
func (t *TwampTest) readReplies(results chan TwampResults, done <-chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
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
			t.mutex.Unlock()
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
		rCopy := *r
		t.mutex.Unlock()
		results <- rCopy
	}
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
Run a single TWAMP test and return a pointer to the TwampResults.
*/
func (t *TwampTest) RunSingle() (*TwampResults, error) {
	senderSeqNum := t.seq
	size, _, _ := t.putMessageOnWire(true)
	t.seq++
	r, err := t.readReply(size)
	if err != nil {
		return nil, err
	}
	if senderSeqNum > r.SenderSeqNum {
		// Likely just received a packet that has timed out or a duplicate. Read until we are up to date
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

func (t *TwampTest) putMessageOnWire(padZero bool) (int, byte, time.Time) {
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
	fmt.Printf("%d bytes from %s: send_seq=%d refl_seq=%d ttl=%d time=%0.03f ms%s\n",
		packetSize,
		t.GetRemoteTestHost(),
		twampResults.SenderSeqNum,
		twampResults.SeqNum,
		twampResults.SenderTTL,
		(float64(twampResults.GetRTT()) / float64(time.Millisecond)),
		duplicateNotice,
	)
}

func (t *TwampTest) Ping(count uint64, interval time.Duration, done chan bool) (*PingResults, error) {
	var pingResults *PingResults
	var err error
	// Calculate summaries upon returning
	defer func() {
		stats := pingResults.Stat
		duplicates := ""
		if stats.Duplicates > 0 {
			duplicates = fmt.Sprintf(" +%d duplicates,", stats.Duplicates)
		}
		fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
		fmt.Printf("%d packets transmitted, %d packets received,%s %0.1f%% packet loss\n",
			stats.Transmitted,
			stats.Received,
			duplicates,
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

	pingResults, err = t.RunMultiple(count, t.printPingReply, interval, done)
	return pingResults, err
}

// Use a blocking ping, pinging as soon as a reply or timeout is hit.
// TODO listen for done signal even while waiting for a reply/timeout as
// opposed to having to check for the signal at the start of each iteration
func (t *TwampTest) PingRapid(count uint64, done <-chan bool) (*PingResults, error) {
	continuous := false
	if count == 0 {
		continuous = true
	}
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate summaries upon returning
	defer func() {
		stats.Avg = time.Duration(uint64(totalRTT) / stats.Transmitted)
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
	var iterations uint64 = 0
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
		// TODO count duplicates and display at end -- requires rewrite of sending method to use
		// the same or similar method as RunMultiple
		twampResults, err := t.RunSingle()
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

func (t *TwampTest) RunMultiple(count uint64, callback TwampTestCallbackFunction, interval time.Duration, done <-chan bool) (*PingResults, error) {
	stats := &PingResultStats{}
	pingResults := &PingResults{Stat: stats}
	var totalRTT time.Duration = 0

	// Calculate totals upon returning
	defer func() {
		t.mutex.RLock()
		stats.Avg = time.Duration(uint64(totalRTT) / stats.Transmitted)
		stats.Loss = float64(float64(stats.Transmitted-stats.Received)/float64(stats.Transmitted)) * 100.0
		stats.StdDev = pingResults.stdDev(stats.Avg)
		t.mutex.RUnlock()
	}()


	// We must use a struct chan instead of a struct pointer chan to
	// make sure that we have a snapshot of the reply received, in case
	// we get a duplicate reply that gets processed before we have a
	// chance to process the last reply, as the underlying map that
	// sends test requests uses the sequence number as an index and thus
	// we might flag a response as a duplicate before we have a chance
	// to handle the previous one in the loop
	replyChan := make(chan TwampResults, 4096)

	receivedEverything := false
	var wg sync.WaitGroup
	defer wg.Wait()
	// Reset the UDP session upon returning so the reading channel will
	// stop waiting for a timeout and immediately return
	defer t.Reset()
	stopChildren := make(chan bool, 1)
	defer close(stopChildren)
	childError := make(chan error, 1)
	defer close(childError)
	// Run a TWAMP test count times, yield results to replyChan
	go func() {
		wg.Add(1)
		t.runTest(count, interval, stopChildren, childError, &stats.Transmitted, replyChan, &wg)
	}()

	// Run until done signal or we've received everything/timed out
	for !receivedEverything {
		select {
		case <-done:
			return pingResults, nil
		case err := <-childError:
			return pingResults, err
		case twampResults, ok := <-replyChan:
			if !ok {
				// Reply channel has been closed
				return pingResults, fmt.Errorf("problem with reply handling routine")
			}
			if !twampResults.IsDuplicate {
				stats.Received++
			} else {
				stats.Duplicates++
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
			if atomic.LoadUint64(&stats.Transmitted) == count && atomic.LoadUint64(&stats.Transmitted) == atomic.LoadUint64(&stats.Received) {
				receivedEverything = true
			}
		case <-time.After(time.Duration(t.GetTimeout()) * time.Second):
			if atomic.LoadUint64(&stats.Transmitted) == count {
				receivedEverything = true
			}
		}
	}
	return pingResults, nil
}
