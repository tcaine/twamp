package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/tcaine/twamp"
)

const maxuint64 = ^uint(0)

func main() {
	intervalFlag := flag.Float64("interval", 1, "Delay between TWAMP-test requests (seconds). For sub-second intervals, use floating points")
	count := flag.Uint64("count", 5, fmt.Sprintf("Number of requests to send (0..%d packets, 0 being continuous)", maxuint64))
	rapid := flag.Bool("rapid", false, "Send requests as rapidly as possible (default count of 5, ignores interval and sends next packet as soon as we have a response/timeout)")
	size := flag.Int("size", 42, "Size of request packets (0..65468 bytes)")
	tos := flag.Int("tos", 0, "IP type-of-service value (0..255)")
	timeout := flag.Int("timeout", 1, "Maximum wait time for a response for the last packet (seconds). If rapid is set, this is a timeout for every packet")
	port := flag.Int("port", 6666, "UDP port to send request packets")
	remotePort := flag.Int("remote-port", 862, "Remote host port")
	mode := flag.String("mode", "ping", "Mode of operation (ping, json)")

	flag.Parse()

	args := flag.Args()

	interval := time.Duration(*intervalFlag * float64(time.Second))

	if *rapid == true {
		interval = 0
	}

	if len(args) < 1 {
		fmt.Println("No hostname or IP address was specified.")
		os.Exit(1)
	}

	remoteIP := args[0]
	remoteServer := fmt.Sprintf("%s:%d", remoteIP, *remotePort)

	c := twamp.NewClient()
	connection, err := c.Connect(remoteServer)
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()

	session, err := connection.CreateSession(
		twamp.TwampSessionConfig{
			ReceiverPort: *port,
			SenderPort:   *port,
			Timeout:      *timeout,
			Padding:      *size,
			TOS:          *tos,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer session.Stop()

	if err := session.TestConnection(); err != nil {
		log.Fatalf("Unable to initialize TWAMP TCP session: %s\n", err)
	}

	test, err := session.CreateTest()
	if err != nil {
		log.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	done := make(chan bool, 1)
	wrapup := make(chan bool, 1)

	if *mode != "json" && *mode != "ping" {
		log.Fatal("Invalid run mode. Supported modes are 'json' and 'ping'")
	}

	switch *mode {
	case "json":
		go func() {
			results, err := test.RunMultiple(*count, nil, interval, done)
			if err != nil {
				log.Println(err)
			}
			test.FormatJSON(results)
			close(wrapup)
		}()
	case "ping":
		go func() {
			var err error
			if *rapid {
				_, err = test.PingRapid(*count, done)
			} else {
				_, err = test.Ping(*count, interval, done)
			}
			if err != nil {
				log.Println(err)
			}
			close(wrapup)
		}()
	}
	select {
	case <-sig:
		close(done)
		<-wrapup // Wait for test results
	case <-wrapup:
	}
}
