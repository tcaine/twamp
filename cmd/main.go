package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/orn1983/twamp-client"
)

func main() {
	intervalFlag := flag.Float64("interval", 1, "Delay between TWAMP-test requests (seconds). For sub-second intervals, use floating points")
	count := flag.Int("count", 5, "Number of requests to send (0..2000000000 packets, 0 being continuous)")
	rapid := flag.Bool("rapid", false, "Send requests as rapidly as possible (default count of 5, ignores interval)")
	size := flag.Int("size", 42, "Size of request packets (0..65468 bytes)")
	tos := flag.Int("tos", 0, "IP type-of-service value (0..255)")
	timeout := flag.Int("timeout", 1, "Maximum wait time for a packet response (seconds)")
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

	switch *mode {
	case "json":
		go func() {
			results := test.RunX(*count, nil, interval, done)
			test.FormatJSON(results)
			close(wrapup)
		}()
	case "ping":
		go func() {
			test.Ping(*count, interval, done)
			close(wrapup)
		}()
	}
	select {
	case <-sig:
		done<-true // Signal tester that we're stopping
		<-wrapup   // Wait for test results
	case <-wrapup:
	}
}
