package twamp

import (
	"log"
	"math"
	"time"
)

/*
	TWAMP test result timestamps have been normalized to UNIX epoch time.
*/
type TwampResults struct {
	SeqNum              uint32    `json:"seqnum"`
	Timestamp           time.Time `json:"timestamp"`
	ErrorEstimate       uint16    `json:"errorEstimate"`
	ReceiveTimestamp    time.Time `json:"receiveTimestamp"`
	SenderSeqNum        uint32    `json:"senderSeqnum"`
	SenderTimestamp     time.Time `json:"senderTimestamp"`
	SenderErrorEstimate uint16    `json:"senderErrorEstimate"`
	SenderTTL           byte      `json:"senderTTL"`
	FinishedTimestamp   time.Time `json:"finishedTimestamp"`
	SenderSize          int       `json:"senderSize"`
}

func (r *TwampResults) GetWait() time.Duration {
	return r.Timestamp.Sub(r.ReceiveTimestamp)
}

func (r *TwampResults) GetRTT() time.Duration {
	return r.FinishedTimestamp.Sub(r.SenderTimestamp)
}

func (r *TwampResults) PrintResults() {
	log.Printf("TWAMP test took %s.\n", r.GetRTT())
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
		total += math.Pow(float64(result.GetRTT()-mean), 2)
	}
	variance := total / float64(len(r.Results)-1)
	return time.Duration(math.Sqrt(variance))
}
