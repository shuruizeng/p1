// Contains the implementation of a LSP client.

package lsp

import (
	"fmt"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"errors"
	"log"
	"time"
	// "container/list"
)

type readRes struct {
	payLoad []byte
	err error
}

type clientnewsend struct {
	message *Message
	acked bool
	nextBackoff int
	currentBackoff int
}

type client struct {
	// TODO: implement this!
	flag bool
	epochNorespond int
	connId int
	maxSeqNum int
	connAddr *lspnet.UDPConn
	closeReadRoutine chan bool
	newMessage chan *Message
	closeReadFun chan bool
	sendMessage chan *Message
	sendMessageQueue []*Message
	unackedMessages []*clientnewsend
	sendPendingMessageQueue []*Message
	unacked_count int
	readMessageWaitMap map[int]*Message
	messagesRead []*Message
	readReq chan chan readRes
	readRes chan readRes
	connected chan bool
	stopconnecting bool
	closeWriteRoutine chan bool
	closeClient chan bool
	closed bool
	sendSeqNum int
	total_epoch int
	epoch_new int
	params *Params
	writeReq chan []byte
	callRead chan readRes
	trigger *time.Ticker
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
		flag: true,
		connId: 0,
		epochNorespond: 0,
		maxSeqNum: 1,
		unacked_count: 0,
		sendPendingMessageQueue: make([]*Message, 0),
		newMessage: make(chan *Message),
		closeReadRoutine: make(chan bool),
		sendMessage: make(chan *Message),
		sendMessageQueue: make([]*Message, 0),
		unackedMessages: make([]*clientnewsend,0),
		readMessageWaitMap: make(map[int]*Message),
		messagesRead: make([]*Message, 0),
		connected: make(chan bool),
		stopconnecting: false,
		closeWriteRoutine: make(chan bool),
		closed: false,
		closeClient: make(chan bool),
		sendSeqNum: 0,
		total_epoch: 0,
		epoch_new: 0,
		writeReq: make(chan []byte),
		params: params,
		callRead: make(chan readRes),
		readReq: make(chan chan readRes),
		trigger: time.NewTicker(time.Duration(params.EpochMillis) * time.Millisecond),
	}

	go client.ReadRoutine()
	go client.WriteRountine()
	go client.Main()

	msg := NewConnect()
	client.sendMessage <- msg

	for {
		select {
		case <- client.connected:
			log.Printf("Client Connected")
			client.stopconnecting = true
			return &client,nil
		}
	}
	return nil, errors.New("Failed Connecting to client")
}

func (c *client) ConnID() int {
	return c.connId
}

func (c *client) ReadRoutine() {
	log.Printf("In Client ReadRoutine")
	for {
		select {
		case <- c.closeReadRoutine:
			return
		default:
			// log.Printf("Client Reading")
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
			log.Printf("Client Recieved a new Message: "+ msg.String())
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
			log.Printf("Client Write To Server: " + sendMessage.String())
		}
	}
}

func (c *client) Main() {
	log.Printf("In Client Main Routine")
	for {
		select {
		case <- c.trigger.C:
			//drop client if no respond epoch num exceed epochlimit
			if c.epochNorespond == c.params.EpochLimit && len(c.readMessageWaitMap)== 0 {
				c.drop()
			} else {
				c.epochNorespond = c.epochNorespond+ 1
				for i := 0; i < len(c.unackedMessages); i++ {
					msg := c.unackedMessages[i]
					msg.currentBackoff = msg.currentBackoff + 1
					if !msg.acked && msg.nextBackoff == msg.currentBackoff {
						c.flag = false
						msg.updateNextBackoff(c.params.MaxBackOffInterval)
						c.sendMessage <- msg.message
					}
				}
				if c.flag {
					log.Printf("Client Not Dead Ack")
					c.sendMessage <-  NewAck(c.connId, 0)
				}
				c.flag = true
			}
		case newMessage := <- c.newMessage:
			log.Printf("Client Start Process Recieved new Message")
			//check type and
			/*if newMessage.Type == MsgConnect {
				fmt.Println("Client Connect")
				//deal with connection id, seqnum
				c.connId = newMessage.ConnID
				ackmessage := NewAck(c.connId + 1, c.maxSeqNum)
				c.connId = c.connId + 1
				c.maxSeqNum = c.maxSeqNum + 1
				c.sendMessage <- ackmessage
			} else*/

			if newMessage.Type == MsgData {
				//add new message to messageQueue, deal with seqnum
				log.Printf("Client Data Message: " + newMessage.String())
				c.epochNorespond = 0
				// log.Printf("MaxSeqNum: %d, MessageSeqNum: %d", c.maxSeqNum, newMessage.SeqNum)
				if c.maxSeqNum == newMessage.SeqNum {
					ackmessage := NewAck(c.connId, c.maxSeqNum)
					c.messagesRead = append(c.messagesRead, newMessage)
					c.tryRead()
					c.maxSeqNum = c.maxSeqNum + 1
					c.sendMessage <- ackmessage
				} else if c.maxSeqNum < newMessage.SeqNum {
					waitedmessage, ok := c.readMessageWaitMap[c.maxSeqNum]
					if ok {
						c.messagesRead = append(c.messagesRead,waitedmessage)
						c.maxSeqNum = c.maxSeqNum + 1
						c.tryRead()
						delete(c.readMessageWaitMap, c.maxSeqNum)
					} else {
						c.readMessageWaitMap[c.maxSeqNum] = newMessage
					}
				} else {
					errors.New("Incorrect Seq Number")
				}
				log.Printf("Done Processing Data")
			} else if newMessage.Type == MsgAck {
				log.Printf("Client Received Ack: " + newMessage.String())
				// log.Printf(newMessage.SeqNum)
				if newMessage.SeqNum == 0 && c.stopconnecting == false {
					c.connected <- true
					c.connId = newMessage.ConnID
					c.sendSeqNum = c.sendSeqNum + 1
				}
				// //remove Acked Message from Server
				// if (len(c.sendMessageQueue) > 0) {
				// 	c.sendMessageQueue = c.sendMessageQueue[1:]
				// }
				c.trySend(newMessage)
			}
		case res := <- c.readReq:
			log.Printf("Client received read request")
			if c.closed == true {
				res <- readRes{payLoad: nil, err: errors.New("timeout")}
			} else {
				c.readRes = res
				c.tryRead()
			}
		case <- c.closeClient:
			continue
		case payload := <- c.writeReq:
			msg := NewData(c.connId, c.sendSeqNum, len(payload), payload,(uint16)(ByteArray2Checksum(payload)))
			c.sendSeqNum = c.sendSeqNum + 1
			c.sendMessageQueue = append(c.sendMessageQueue, msg)
			c.trySend(msg)
		}
	}
}

func (msg *clientnewsend) updateNextBackoff(MaxBackoffInterval int) {
	if msg.currentBackoff == 0 {
		msg.nextBackoff = msg.nextBackoff + 1
	} else {
		doubleBackOff := msg.currentBackoff * 2
		if doubleBackOff < MaxBackoffInterval {
			msg.nextBackoff = doubleBackOff
		} else {
			msg.nextBackoff = MaxBackoffInterval
		}
	}
	msg.currentBackoff = 0
}
func (c *client) drop() {
	c.Close()
}

func (c *client) trySend(message *Message) {
	//check length unacked Message
	// if len(c.sendMessageQueue) > c.params.WindowSize {
	// 	return
	// } else if len(c.sendMessageQueue) > 0 {
	// 	msg := c.sendMessageQueue[0]
	// 	// log.Printf("Try Send")
	// 	c.sendMessage <- msg
	// 	c.sendMessageQueue = c.sendMessageQueue[1:]
	// 	log.Printf("Client Sent a message To Server: "+ msg.String())
	// } else {
	// 	return
	// }
	if message.Type == MsgData {
		if c.unacked_count >= c.params.MaxUnackedMessages || len(c.sendMessageQueue) >= c.params.WindowSize {
			c.sendPendingMessageQueue = append(c.sendPendingMessageQueue,message)
			return
		}
		c.sendMessageQueue = append(c.sendMessageQueue, message)
	}
	if message.Type == MsgAck && len(c.unackedMessages) > 0{
		//check ack == first left ack and delete it from c.unackedmessages
		
		if message.SeqNum == c.unackedMessages[0].message.SeqNum {
			fmt.Println("Client Message: ", message)
			fmt.Println("Client UnackedMessage", c.unackedMessages[0].message)
			c.unackedMessages = c.unackedMessages[1:]
			c.unacked_count = c.unacked_count - 1
			if len(c.sendMessageQueue) <= c.params.WindowSize && len(c.sendPendingMessageQueue) > 0{
				c.sendMessageQueue = append(c.sendMessageQueue, c.sendPendingMessageQueue[0])
				c.sendPendingMessageQueue = c.sendPendingMessageQueue[1:]
			}
		} else {
			log.Printf("Cliet CheckSum error when removing unackedMessages")
			errors.New("Client Incorrect CheckSum")
			return
		}
	}
	
	for {
		if len(c.sendMessageQueue) > 0 {
			msg := c.sendMessageQueue[0]
			c.sendMessage <- msg
			c.sendMessageQueue = c.sendMessageQueue[1:]
			unackedMsg := clientnewsend{message:msg, acked: false, nextBackoff: 1, currentBackoff: 0}
			c.unackedMessages = append(c.unackedMessages, &unackedMsg)
		} else {
			return
		}
	}
}

func (c *client) tryRead() {
	if c.readRes != nil && len(c.messagesRead) > 0 {
		msg := c.messagesRead[0]
		c.messagesRead = c.messagesRead[1:]
		c.readRes <- readRes{payLoad: msg.Payload, err: nil}
		close(c.readRes)
		c.readRes = nil
		fmt.Println("Client Read a message: "+ msg.String())
		fmt.Println("Remaining Client MessageRead ", c.messagesRead)
	}
}

func (c *client) Read() ([]byte, error) {
	fmt.Println("In Client Read Function")
	ch := make(chan readRes)
	c.readReq <- ch
	res := <- ch
	return res.payLoad, res.err
}

func (c *client) Write(payload []byte) error {
	if c.connAddr == nil {
		return errors.New("connection lost")
	}
	c.writeReq <- payload
	return nil
}

func (c *client) Close() error {
	c.closeClient <- true
	c.closeReadRoutine <- true
	c.closeWriteRoutine <- true
	c.connAddr.Close()
	return nil
}
