# twamp

TWAMP client for go

## TWAMP ping command line utility

```
# build twamp command line utility
sigsegv:twamp tcaine$ go build -o twamp main.go 

# display usage message
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

# twamp ping
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

# rapid twamp ping
sigsegv:twamp tcaine$ ./twamp --count=100 --rapid 10.1.1.200 
TWAMP PING 10.1.1.200: 56 data bytes
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
--- 10.1.1.200 twamp ping statistics ---
100 packets transmitted, 100 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 27.456/81.008/924.369/115.346 ms
```

## SYNOPSIS

```

c := client.NewClient()
connection, err := c.Connect("10.1.1.200:862")
if err != nil {
	log.Fatal(err)
}

session, err := connection.CreateSession(
	client.TwampSessionConfig{
		Port:    6666,
		Timeout: 1,
		Padding: 42,
		TOS:     0,
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

