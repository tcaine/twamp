package twamp

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"
)

/*
Security modes for TWAMP session.
*/
const (
	ModeUnspecified     = 0
	ModeUnauthenticated = 1
)

type TwampClient struct{}

func NewClient() *TwampClient {
	return &TwampClient{}
}

func (c *TwampClient) Connect(hostname string) (*TwampConnection, error) {
	// connect to remote host
	conn, err := net.DialTimeout("tcp", hostname, time.Second*5)
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

	if greeting.Mode == ModeUnspecified {
		return nil, fmt.Errorf("The TWAMP server is not interested in communicating with you.")
	}

	if greeting.Mode&ModeUnauthenticated == 0 {
		return nil, errors.New("only unauthenticated mode is currently supported")
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

func readFromSocket(conn net.Conn, size int, timeoutSeconds int) (bytes.Buffer, error) {
	timeout := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	buf := make([]byte, size)
	buffer := *bytes.NewBuffer(buf)
	conn.SetReadDeadline(timeout)
	bytesRead, err := conn.Read(buf)

	if err != nil {
		return buffer, err
	}

	if bytesRead < size {
		return buffer, fmt.Errorf(fmt.Sprintf("readFromSocket: expected %d bytes, got %d", size, bytesRead))
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
		return fmt.Errorf("ERROR: The %s failed.", context)
	case InternalError:
		return fmt.Errorf("ERROR: The %s failed: internal error.", context)
	case NotSupported:
		return fmt.Errorf("ERROR: The %s failed: not supported.", context)
	case PermanentResourceLimitation:
		return fmt.Errorf("ERROR: The %s failed: permanent resource limitation.", context)
	case TemporaryResourceLimitation:
		return fmt.Errorf("ERROR: The %s failed: temporary resource limitation.", context)
	}
	return nil
}
