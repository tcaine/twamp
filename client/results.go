package client

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"
)

type TwampResults struct {
	SeqNum              uint32         `json:"seqnum"`
	Timestamp           TwampTimestamp `json:"timestamp"`
	ErrorEstimate       uint16         `json:"errorEstimate"`
	ReceiveTimestamp    TwampTimestamp `json:"receiveTimestamp"`
	SenderSeqNum        uint32         `json:"senderSeqnum"`
	SenderTimestamp     TwampTimestamp `json:"senderTimestamp"`
	SenderErrorEstimate uint16         `json:"senderErrorEstimate"`
	SenderTTL           byte           `json:"senderTTL"`
	FinishedTimestamp   TwampTimestamp `json:"finishedTimestamp"`
	SenderSize          int            `json:"senderSize"`
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
	Min         time.Duration `json:"min"`
	Max         time.Duration `json:"max"`
	Avg         time.Duration `json:"avg"`
	StdDev      time.Duration `json:"stddev"`
	Transmitted int           `json:"tx"`
	Received    int           `json:"rx"`
	Loss        float64       `json:"loss"`
}

type PingResults struct {
	Results []*TwampResults  `json:"results"`
	Stat    *PingResultStats `json:"stats"`
}

func (r *PingResults) stdDev(mean time.Duration) time.Duration {
	total := float64(0)
	for _, result := range r.Results {
		total += math.Pow(float64(result.GetAbsoluteRTT()-mean), 2)
	}
	variance := total / float64(len(r.Results)-1)
	return time.Duration(math.Sqrt(variance))
}

func (t *TwampTest) FormatJSON(r *PingResults) {
	doc, err := json.Marshal(r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", string(doc))
}

func (t *TwampTest) FormatPing(r *PingResults) {
	if len(r.Results) < 1 {
		return
	}

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

	for i := 0; i < len(r.Results); i++ {
		result := r.Results[i]
		fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms\n",
			result.SenderSize,
			t.GetRemoteTestHost(),
			result.SenderSeqNum,
			result.SenderTTL,
			(float64(result.GetAbsoluteRTT()) / float64(time.Millisecond)),
		)
	}

	stat := r.Stat
	fmt.Printf("--- %s twamp ping statistics ---\n", t.GetRemoteTestHost())
	fmt.Printf("%d packets transmitted, %d packets received, %0.1f%% packet loss\n", stat.Transmitted, stat.Received, stat.Loss)
	fmt.Printf("round-trip min/avg/max/stddev = %0.3f/%0.3f/%0.3f/%0.3f ms\n",
		(float64(stat.Min) / float64(time.Millisecond)),
		(float64(stat.Avg) / float64(time.Millisecond)),
		(float64(stat.Max) / float64(time.Millisecond)),
		(float64(stat.StdDev) / float64(time.Millisecond)),
	)
}

func (t *TwampTest) Ping(count int) *PingResults {
	Stats := &PingResultStats{}
	Results := &PingResults{Stat: Stats}
	var TotalRTT time.Duration = 0

	packetSize := 14 + t.GetSession().GetConfig().Padding

	fmt.Printf("TWAMP PING %s: %d data bytes\n", t.GetRemoteTestHost(), packetSize)

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
			Results.Results = append(Results.Results, results)

			fmt.Printf("%d bytes from %s: twamp_seq=%d ttl=%d time=%0.03f ms\n",
				packetSize,
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
			Results.Results = append(Results.Results, results)
		}
	}

	Stats.Avg = time.Duration(int64(TotalRTT) / int64(count))
	Stats.Loss = float64(float64(Stats.Transmitted-Stats.Received)/float64(Stats.Transmitted)) * 100.0
	Stats.StdDev = Results.stdDev(Stats.Avg)

	return Results
}

/* end */
