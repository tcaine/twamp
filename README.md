# twamp

TWAMP client for go

## Client Library Synopsis

```
c := client.NewClient()
connection, err := c.Connect("10.1.1.200:862")
if err != nil {
	log.Fatal(err)
}

session, err := connection.CreateSession(
	twamp.TwampSessionConfig{
		Port:    6666,
		Timeout: 1,
		Padding: 42,
		TOS:     twamp.EF,
		},
	)
if err != nil {
	log.Fatal(err)
}

test, err := session.CreateTest()
if err != nil {
	log.Fatal(err)
}

results := test.RunX(count)

session.Stop()
connection.Close()
```

## TWAMP ping command line utility

### CLI Usage Message

```
sigsegv:twamp tcaine$ ./twamp --help
Usage of ./twamp:
  -count int
    	Number of requests to send (1..2000000000 packets) (default 5)
  -interval int
    	Delay between TWAMP-test requests (seconds) (default 1)
  -mode string
    	Mode of operation (ping, json) (default "ping")
  -port int
    	UDP port to send request packets (default 6666)
  -rapid
    	Send requests rapidly (default count of 5)
  -size int
    	Size of request packets (0..65468 bytes) (default 42)
  -tos int
    	IP type-of-service value (0..255)
  -wait int
    	Maximum wait time after sending final packet (seconds) (default 1)
```

### Twamp Ping

```
sigsegv:twamp tcaine$ ./twamp 10.1.1.200
TWAMP PING 10.1.1.200: 56 data bytes
56 bytes from 10.1.1.200: twamp_seq=0 ttl=250 time=45.252 ms
56 bytes from 10.1.1.200: twamp_seq=1 ttl=250 time=484.722 ms
56 bytes from 10.1.1.200: twamp_seq=2 ttl=250 time=134.527 ms
56 bytes from 10.1.1.200: twamp_seq=3 ttl=250 time=41.005 ms
56 bytes from 10.1.1.200: twamp_seq=4 ttl=250 time=46.791 ms
--- 10.1.1.200 twamp ping statistics ---
5 packets transmitted, 5 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 41.005/150.459/484.722/190.907 ms
```

### Twamp Rapid Ping

```
sigsegv:twamp tcaine$ ./twamp --count=100 --rapid 10.1.1.200 
TWAMP PING 10.1.1.200: 56 data bytes
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
--- 10.1.1.200 twamp ping statistics ---
100 packets transmitted, 100 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 27.456/81.008/924.369/115.346 ms
```

### Twamp Ping JSON

```
sigsegv:twamp tcaine$ ./twamp --count=5 --mode=json 10.1.1.200 | json_pp
{
   "stats" : {
      "tx" : 5,
      "stddev" : 61171928,
      "rx" : 5,
      "avg" : 104719892,
      "min" : 29052824,
      "loss" : 0,
      "max" : 155204310
   },
   "results" : [
      {
         "senderTimestamp" : "2016-12-12T21:12:13.475143032-08:00",
         "finishedTimestamp" : "2016-12-12T21:12:13.523426063-08:00",
         "seqnum" : 0,
         "receiveTimestamp" : "2016-12-12T21:12:14.982226191-08:00",
         "senderTTL" : 250,
         "errorEstimate" : 1,
         "timestamp" : "2016-12-12T21:12:14.982780242-08:00",
         "senderSeqnum" : 0,
         "senderErrorEstimate" : 257,
         "senderSize" : 56
      },
      {
         "senderTimestamp" : "2016-12-12T21:12:13.523434427-08:00",
         "finishedTimestamp" : "2016-12-12T21:12:13.678393972-08:00",
         "seqnum" : 1,
         "receiveTimestamp" : "2016-12-12T21:12:15.662510356-08:00",
         "senderTTL" : 250,
         "errorEstimate" : 1,
         "timestamp" : "2016-12-12T21:12:15.662776644-08:00",
         "senderSeqnum" : 1,
         "senderErrorEstimate" : 257,
         "senderSize" : 56
      },
      {
         "senderTimestamp" : "2016-12-12T21:12:13.678401499-08:00",
         "finishedTimestamp" : "2016-12-12T21:12:13.833605809-08:00",
         "seqnum" : 2,
         "receiveTimestamp" : "2016-12-12T21:12:15.774115081-08:00",
         "senderTTL" : 250,
         "errorEstimate" : 1,
         "timestamp" : "2016-12-12T21:12:15.774321239-08:00",
         "senderSeqnum" : 2,
         "senderErrorEstimate" : 257,
         "senderSize" : 56
      },
      {
         "senderTimestamp" : "2016-12-12T21:12:13.833613447-08:00",
         "finishedTimestamp" : "2016-12-12T21:12:13.862666271-08:00",
         "seqnum" : 3,
         "receiveTimestamp" : "2016-12-12T21:12:16.454055649-08:00",
         "senderTTL" : 250,
         "errorEstimate" : 1,
         "timestamp" : "2016-12-12T21:12:16.454300462-08:00",
         "senderSeqnum" : 3,
         "senderErrorEstimate" : 257,
         "senderSize" : 56
      },
      {
         "senderTimestamp" : "2016-12-12T21:12:13.862672913-08:00",
         "finishedTimestamp" : "2016-12-12T21:12:13.998772663-08:00",
         "seqnum" : 4,
         "receiveTimestamp" : "2016-12-12T21:12:16.580967637-08:00",
         "senderTTL" : 250,
         "errorEstimate" : 1,
         "timestamp" : "2016-12-12T21:12:16.581173796-08:00",
         "senderSeqnum" : 4,
         "senderErrorEstimate" : 257,
         "senderSize" : 56
      }
   ]
}
sigsegv:twamp tcaine$ 
```

