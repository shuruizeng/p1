// Contains the implementation of a LSP client.

package lsp

import (
	"fmt"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"errors"
	// "container/list"
)

type readRes struct {
	payLoad []byte
	err error
}


type client struct {
	// TODO: implement this!
	connId int
	maxSeqNum int
	connAddr *lspnet.UDPConn
	closeReadRoutine chan bool
	newMessage chan *Message
	closeReadFun chan bool
	sendMessage chan *Message
	messagesRead []*Message
	readReq chan bool
	readRes chan readRes
	connected chan bool
	closeWriteRoutine chan bool
	closeClient chan bool
	closed bool
	messageWaitMap map[int]*Message
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
		maxSeqNum: 0,
		newMessage: make(chan *Message),
		closeReadRoutine: make(chan bool),
		sendMessage: make(chan *Message),
		messagesRead: make([]*Message, 0),
		connected: make(chan bool),
		closeWriteRoutine: make(chan bool),
		closed: false,
		closeClient: make(chan bool),
		messageWaitMap: make(map[int]*Message),
	}

	go client.ReadRoutine()
	go client.WriteRountine()
	go client.Main()

	msg := NewConnect()
	client.sendMessage <- msg

	if  connected:= <- client.connected; connected {
		return &client, nil
	}
	return nil, errors.New("Failed Connecting to client")


}

func (c *client) ConnID() int {
	return c.connId
}

func (c *client) ReadRoutine() {
	fmt.Println("In Client ReadRoutine")
	for {
		select {
		case <- c.closeReadRoutine:
			return
		default:
			fmt.Println("Client Reading")
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
			fmt.Println("Client Recieved a new Message")
			fmt.Println(msg)
		}
	}

}

func (c *client) WriteRountine() {
	fmt.Println("In Client WriteRoutine")
	for {
		select {
		case <- c.closeWriteRoutine:
			return
		case sendMessage := <- c.sendMessage:
			data, err := json.Marshal(sendMessage)
			if err != nil {
				errors.New("Error during Marshaling in ReadRoutine")
			}
			_, err = c.connAddr.Write(data)
			if err != nil {
				errors.New("Error during writing to Server")
			}
			fmt.Println("Client Write To Server")
			fmt.Println(sendMessage)
		}
	}
}

func (c *client) Main() {
	fmt.Println("In Client Main Routine")
	for {
		select {
		case newMessage := <- c.newMessage:
			fmt.Println("Client Start Process Recieved Message")
			//check type and
			/*if newMessage.Type == MsgConnect {
				fmt.Println("Client Connect")
				//deal with connection id, seqnum
				c.connId = newMessage.ConnID
				ackmessage := NewAck(c.connId + 1, c.maxSeqNum)
				c.connId = c.connId + 1
				c.maxSeqNum = c.maxSeqNum + 1
				c.sendMessage <- ackmessage
			} else*/ if newMessage.Type == MsgData {
				//add new message to messageQueue, deal with seqnum
				fmt.Println("Client Data")
				c.messagesRead = append(c.messagesRead, newMessage)
				if c.maxSeqNum == newMessage.SeqNum {
					ackmessage := NewAck(c.connId, c.maxSeqNum)
					c.messagesRead = append(c.messagesRead, newMessage)
					c.maxSeqNum = c.maxSeqNum + 1
					c.sendMessage <- ackmessage
				} else if c.maxSeqNum < newMessage.SeqNum {
					waitedmessage, ok := c.messageWaitMap[c.maxSeqNum]
					if ok {
						c.messagesRead = append(c.messagesRead,waitedmessage)
						c.maxSeqNum = c.maxSeqNum + 1
						delete(c.messageWaitMap, c.maxSeqNum)
					} else {
						c.messageWaitMap[c.maxSeqNum] = newMessage
					}
				} else {
					errors.New("Incorrect Seq Number")
				}
			} else if newMessage.Type == MsgAck {
				fmt.Println("Client Ack")
				fmt.Println(newMessage.SeqNum)
				if newMessage.SeqNum == 0 {
					c.connected <- true
					c.connId = newMessage.ConnID
				}
			}
		case <- c.readReq:
			fmt.Println("client received read request")
			if len(c.messagesRead) > 0 {
				message := c.messagesRead[0]
				c.messagesRead = c.messagesRead[1:]
				if  c.closed {
					c.readRes <- readRes{payLoad: nil, err: errors.New("Client Closed")}
				} else {
					c.readRes <- readRes{payLoad: message.Payload, err: nil}
				}
			}
		case <- c.closeClient:
			continue
		}
	}

}

func (c *client) Read() ([]byte, error) {
	fmt.Println("In Client Read Function")
	c.readReq <- true
	for {
		select {
		case res := <- c.readRes:
			return res.payLoad, res.err
		}

	}
}

func (c *client) Write(payload []byte) error {
	if c.connAddr == nil {
		return errors.New("connection lost")
	}
	msg := NewData(c.connId, c.maxSeqNum, len(payload), payload,
	(uint16)(ByteArray2Checksum(payload)))
	c.sendMessage <- msg
	return nil
}

func (c *client) Close() error {
	c.closeClient <- true
	c.closeReadRoutine <- true
	c.closeWriteRoutine <- true
	c.connAddr.Close()
	return nil
}
