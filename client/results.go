package client

import (
	"fmt"
	"log"
	"math"
	"time"
)

type TwampResults struct {
	SeqNum              uint32
	Timestamp           TwampTimestamp
	ErrorEstimate       uint16
	ReceiveTimestamp    TwampTimestamp
	SenderSeqNum        uint32
	SenderTimestamp     TwampTimestamp
	SenderErrorEstimate uint16
	SenderTTL           byte
	FinishedTimestamp   TwampTimestamp
	Size                uint32
}

func (r *TwampResults) GetWait() time.Duration {
	return r.Timestamp.GetTime().Sub(r.ReceiveTimestamp.GetTime())
}

func (r *TwampResults) GetRTT() time.Duration {
	return r.FinishedTimestamp.GetTime().Sub(r.SenderTimestamp.GetTime())
}

func (r *TwampResults) GetAbsoluteRTT() time.Duration {
	return r.GetRTT() - r.GetWait()
}

func (r *TwampResults) PrintResults() {
	log.Printf("TWAMP test took %s.\n", r.GetAbsoluteRTT())
	log.Printf("Sender Sequence Number: %d", r.SenderSeqNum)
	log.Printf("Receiver Sequence Number: %d", r.SeqNum)
}

type PingResultStats struct {
	Min         time.Duration
	Max         time.Duration
	Avg         time.Duration
	StdDev      time.Duration
	Transmitted int
	Received    int
	Loss        float64
}

type PingResults struct {
	results []*TwampResults
	stat    *PingResultStats
}

func (r *PingResults) stdDev(mean time.Duration) time.Duration {
	total := float64(0)
	for _, result := range r.results {
		total += math.Pow(float64(result.GetAbsoluteRTT()-mean), 2)
	}
	variance := total / float64(len(r.results)-1)
	return time.Duration(math.Sqrt(variance))
}

func (t *TwampTest) Ping(count int) *PingResults {
	Stats := &PingResultStats{}
	Results := &PingResults{stat: Stats}
	var TotalRTT time.Duration = 0

	PacketSize := 14 + t.GetSession().config.Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), PacketSize)

	for i := 0; i < count; i++ {
		Stats.Transmitted++
		results, err := t.Run()
		if err != nil {
		} else {
			if i == 0 {
				Stats.Min = results.GetAbsoluteRTT()
				Stats.Max = results.GetAbsoluteRTT()
			}
			if Stats.Min > results.GetAbsoluteRTT() {
				Stats.Min = results.GetAbsoluteRTT()
			}
			if Stats.Max < results.GetAbsoluteRTT() {
				Stats.Max = results.GetAbsoluteRTT()
			}

			TotalRTT += results.GetAbsoluteRTT()
			Stats.Received++
			Results.results = append(Results.results, results)

			fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms\n",
				PacketSize,
				t.GetRemoteTestHost(),
				results.SenderSeqNum,
				results.SenderTTL,
				(float64(results.GetAbsoluteRTT()) / float64(time.Millisecond)),
			)
		}
	}

	Stats.Avg = time.Duration(int64(TotalRTT) / int64(count))
	Stats.Loss = float64(float64(Stats.Transmitted-Stats.Received)/float64(Stats.Transmitted)) * 100.0
	Stats.StdDev = Results.stdDev(Stats.Avg)

	fmt.Printf("--- %s twamp ping statistics ---\n", "74.40.22.3")
	fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n", Stats.Transmitted, Stats.Received, Stats.Loss)
	fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
		(float64(Stats.Min) / float64(time.Millisecond)),
		(float64(Stats.Avg) / float64(time.Millisecond)),
		(float64(Stats.Max) / float64(time.Millisecond)),
		(float64(Stats.StdDev) / float64(time.Millisecond)),
	)

	return Results
}

/*
PING www.google.com (216.58.193.68): 56 data bytes
64 bytes from 216.58.193.68: icmp_seq=0 ttl=54 time=362.691 ms
64 bytes from 216.58.193.68: icmp_seq=1 ttl=54 time=394.204 ms
64 bytes from 216.58.193.68: icmp_seq=2 ttl=54 time=347.931 ms
64 bytes from 216.58.193.68: icmp_seq=3 ttl=54 time=407.690 ms
64 bytes from 216.58.193.68: icmp_seq=4 ttl=54 time=232.563 ms
64 bytes from 216.58.193.68: icmp_seq=5 ttl=54 time=354.313 ms
64 bytes from 216.58.193.68: icmp_seq=6 ttl=54 time=381.693 ms
64 bytes from 216.58.193.68: icmp_seq=7 ttl=54 time=284.524 ms
^C
--- www.google.com ping statistics ---
8 packets transmitted, 8 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 232.563/345.701/407.690/55.228 ms
*/

/* end */
