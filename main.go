package main

import (
	"flag"
	"fmt"
	"github.com/tcaine/twamp/client"
	"log"
)

func main() {
	countPtr := flag.Int("count", 10, "number of TWAMP tests")
	waitPtr := flag.Int("wait", 30, "test timeout in seconds")
	sizePtr := flag.Int("size", 42, "size of TWAMP test packet")
	portPtr := flag.Int("port", 6666, "remote UDP port for TWAMP test")
	tosPtr := flag.Int("tos", client.BE, "IP TOS of test packet")

	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		log.Fatal("No remote IP address was specified.")
	}

	remoteIP := args[0]
	remoteServer := fmt.Sprintf("%s:%d", remoteIP, 862)

	c := client.New()
	connection, err := c.Connect(remoteServer)
	if err != nil {
		log.Fatal(err)
	}

	config := client.TwampSessionConfig{
		Port:    *portPtr,
		Timeout: *waitPtr,
		Padding: *sizePtr,
		TOS:     *tosPtr,
	}

	session, err := connection.CreateSession(config)
	if err != nil {
		log.Fatal(err)
	}

	test, err := session.CreateTest()
	if err != nil {
		log.Fatal(err)
	}

	results := test.RunX(*countPtr)
	//test.FormatJSON(results)
	test.FormatPing(results)

	session.Stop()
	connection.Close()
}
