// Contains the implementation of a LSP client.

package lsp

import (
	// "fmt"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"errors"
	// "container/list"
)

type client struct {
	// TODO: implement this!
	connId int
	maxSeqNum int
	connAddr *lspnet.UDPConn
	closeReadRoutine chan bool
	newMessage chan *Message
	sendMessage chan *Message
	messageQueue []*Message
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	remoteAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil, err
	}

	conn, err := lspnet.DialUDP("udp", nil, remoteAddr)

	if err != nil {
		return nil, err
	}
	client := client{
		connAddr: conn,
		connId: 0,
		maxSeqNum: 1,
		closeReadRoutine: make(chan bool),
		sendMessage: make(chan *Message),
		messageQueue: make([]*Message, 0),
	}
	
	go client.ReadRoutine()
	go client.Main()
	return &client, nil
}

func (c *client) ConnID() int {
	return c.connId
}

func (c *client) ReadRoutine() {
	for {
		select {
		case <- c.closeReadRoutine:
			return
		default:
			var buffer [1000]byte
			n, err := c.connAddr.Read(buffer[0:])
			if err != nil {
				errors.New("Error When ReadFrom UDP")
			}
			var msg Message
			json.Unmarshal(buffer[:n],&msg)
			if err != nil {
				errors.New("Error during unmarshaling")
			}
			c.newMessage <- &msg
		}
	}

}

func (c *client) Main() {
	msg := NewConnect()
	c.sendMessage <- msg
	for {
		select {
		case sendMessage := <- c.sendMessage:
			data, err := json.Marshal(sendMessage)
				if err != nil {
					errors.New("Error during Marshaling in ReadRoutine")
				}
			_, err = c.connAddr.Write(data)
			if err != nil {
				errors.New("Error during writing to Server")
			}
		case newMessage := <- c.newMessage:
			//check type and 
			if newMessage.Type == MsgConnect {
				//deal with connection id, seqnum
			} else if newMessage.Type == MsgData {
				//add new message to messageQueue, deal with seqnum

			} else if newMessage.Type == MsgAck {
				continue
			}

		}
	}
	
}
func (c *client) Read() ([]byte, error) {
	// TODO: remove this line when you are ready to begin implementing this method.
	select {} // Blocks indefinitely.
	return nil, errors.New("not yet implemented")
}

func (c *client) Write(payload []byte) error {
	return errors.New("not yet implemented")
}

func (c *client) Close() error {
	return errors.New("not yet implemented")
}
