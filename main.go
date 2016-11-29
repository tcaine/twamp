package main

import (
	"fmt"
	"github.com/tcaine/twamp/client"
	"log"
	"os"
)

func main() {

	if len(os.Args) != 2 {
		log.Fatal("No remote IP address was specified.")
	}

	remoteIP := os.Args[1]

	c := client.New()

	connection, err := c.Connect(fmt.Sprintf("%s:%d", remoteIP, 862))
	//	connection, err := c.Connect("74.40.22.3:862")
	if err != nil {
		log.Fatal(err)
	}

	config := client.TwampSessionConfig{
		Port:    6666,
		Timeout: 30,
		Padding: 42,
		DSCP:    0,
	}

	session, err := connection.CreateSession(config)
	if err != nil {
		log.Fatal(err)
	}

	test, err := session.CreateTest()
	if err != nil {
		log.Fatal(err)
	}

	/*
		results, err := test.Run()
		if err != nil {
			log.Fatal(err)
		}
		results.PrintResults()

		results, err = test.Run()
		if err != nil {
			log.Fatal(err)
		}
		results.PrintResults()

		results, err = test.Run()
		if err != nil {
			log.Fatal(err)
		}
		results.PrintResults()
	*/

	_ = test.Ping(10)

	//	log.Printf("Results: %+v\n", results)

	//	log.Printf("Sender Timestamp: %s\n", &results.SenderTimestamp)
	//	log.Printf("Receive Timestamp: %s\n", &results.ReceiveTimestamp)
	//	log.Printf("Timestamp: %s\n", &results.Timestamp)
	//	log.Printf("Finished Timestamp: %s\n", &results.FinishedTimestamp)

	session.Stop()
	connection.Close()
}
