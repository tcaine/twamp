package twamp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
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

func NewClient() *TwampClient {
	return &TwampClient{}
}

func (c *TwampClient) Connect(hostname string) (*TwampConnection, error) {
	// connect to remote host
	conn, err := net.Dial("tcp", hostname)
	if err != nil {
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
		return nil, errors.New("The TWAMP server is not interested in communicating with you.")
	case ModeUnauthenticated:
	case ModeAuthenticated:
		return nil, errors.New("Authentication is not currently supported.")
	case ModeEncypted:
		return nil, errors.New("Encyption is not currently supported.")
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
	bytesRead, err := reader.Read(buf)

	if err != nil && bytesRead < size {
		return buffer, errors.New(fmt.Sprintf("readFromSocket: expected %d bytes, got %d", size, bytesRead))
	}

	return buffer, err
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
