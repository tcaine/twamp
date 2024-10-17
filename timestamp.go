package twamp

import (
	"time"
)

type TwampTimestamp struct {
	Integer  uint32
	Fraction uint32
}

// offset from UNIX epoch to NTP epoch
const epochOffset = time.Duration(0x83aa7e80 * time.Second)

/*
Converts a UNIX epoch time time.Time object into an RFC 1305 compliant time.
*/
func NewTwampTimestamp(t time.Time) *TwampTimestamp {
	// convert epoch from 1970 to 1900 per RFC 1305
	t = t.Add(epochOffset)
	return &TwampTimestamp{
		Integer:  uint32(t.Unix()),
		Fraction: uint32(t.Nanosecond()),
	}
}

func NewTimestamp(twampTimestamp TwampTimestamp) time.Time {
	t := time.Unix(int64(twampTimestamp.Integer), int64(twampTimestamp.Fraction))
	// convert epoch from 1970 to 1900 per RFC 1305
	t = t.Add(-1 * epochOffset)
	return t
}

/*
Return a time.Time object representing Unix Epoch time since January 1st, 1970.
*/
func (t *TwampTimestamp) GetTime() time.Time {
	// convert epoch from 1900 back to 1970
	return time.Unix(int64(t.Integer), int64(t.Fraction)).Add(-1 * epochOffset)
}

func (t *TwampTimestamp) String() string {
	return t.GetTime().String()
}
