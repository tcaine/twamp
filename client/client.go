package client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

/*
	Default TCP port for remote TWAMP server.
*/
const TwampControlPort int = 862

/*
	Security modes for TWAMP session.
*/
const (
	ModeUnspecified     = 0
	ModeUnauthenticated = 1
	ModeAuthenticated   = 2
	ModeEncypted        = 4
)

type TwampClient struct{}

func New() *TwampClient {
	return &TwampClient{}
}

func (c *TwampClient) Connect(hostname string) (*TwampConnection, error) {
	//	log.Printf("Connecting to %s....", hostname)

	// connect to remote host
	conn, err := net.Dial("tcp", hostname)
	if err != nil {
		log.Printf("Could not connect to %s: %s\n", hostname, err)
		return nil, err
	}

	// create a new TwampConnection
	twampConnection := NewTwampConnection(conn)

	// check for greeting message from TWAMP server
	greeting, err := twampConnection.getTwampServerGreetingMessage()
	if err != nil {
		return nil, err
	}

	// check greeting mode for errors
	switch greeting.Mode {
	case ModeUnspecified:
		log.Println("The TWAMP server is not interested in communicating with you.")
		os.Exit(1)
	case ModeUnauthenticated:
		// yep
	case ModeAuthenticated:
		log.Println("Authentication is not currently supported.")
		os.Exit(1)
	case ModeEncypted:
		log.Println("Encyption is not currently supported.")
		os.Exit(1)
	}

	// negotiate TWAMP session configuration
	twampConnection.sendTwampClientSetupResponse()

	// check the start message from TWAMP server
	serverStartMessage, err := twampConnection.getTwampServerStartMessage()
	if err != nil {
		return nil, err
	}

	err = checkAcceptStatus(int(serverStartMessage.Accept), "connection")
	if err != nil {
		return nil, err
	}

	return twampConnection, nil
}

func readFromSocket(reader io.Reader, size int) (bytes.Buffer, error) {
	buf := make([]byte, size)
	buffer := *bytes.NewBuffer(buf)
	_, error := reader.Read(buf)
	return buffer, error
}

/*
	TWAMP Accept Field Status Code
*/
const (
	OK                          = 0
	Failed                      = 1
	InternalError               = 2
	NotSupported                = 3
	PermanentResourceLimitation = 4
	TemporaryResourceLimitation = 5
)

/*
	Convenience function for checking the accept code contained in various TWAMP server
	response messages.
*/
func checkAcceptStatus(accept int, context string) error {
	switch accept {
	case OK:
		return nil
	case Failed:
		return errors.New(fmt.Sprintf("ERROR: The ", context, " failed."))
	case InternalError:
		return errors.New(fmt.Sprintf("ERROR: The ", context, " failed: internal error."))
	case NotSupported:
		return errors.New(fmt.Sprintf("ERROR: The ", context, " failed: not supported."))
	case PermanentResourceLimitation:
		return errors.New(fmt.Sprintf("ERROR: The ", context, " failed: permanent resource limitation."))
	case TemporaryResourceLimitation:
		return errors.New(fmt.Sprintf("ERROR: The ", context, " failed: temporary resource limitation."))
	}
	return nil
}
