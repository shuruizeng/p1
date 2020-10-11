// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"fmt"
	"encoding/json"
	"log"
	"time"
)

type newsend struct {
	message *Message
	acked bool
	nextBackoff int
	currentBackoff int
}

type clientInfo struct {
	connId int
	closeSucceed chan bool
	udpAddr *lspnet.UDPAddr
	maxSeqNum int
	sendSeqNum int
	sendPendingMessageQueue []*Message
	sendMessageQueue []*Message  //message sending to client
	unackedMessages []*newsend  //message did not recieve ack from client
	messageWaitMap map[int]*Message //out of order 
	close bool
	Lost bool
	epoch int
	epochNorespond int
	unacked_count int
	flag bool
}

type serverReadRes struct {
	connId int
	payLoad []byte
	err error
}

type serverWriteReq struct {
	connId int
	payLoad []byte
}
type server struct {
	// TODO: Implement this!
	clients map[int]*clientInfo
	maxId int
	newMessage chan newMessage
	sendMessage chan newMessage
	newConn *lspnet.UDPConn
	closeReadRoutine chan bool
	closeReadFunc chan bool
	closeServer chan bool
	closed bool
	closeMain chan bool
	read bool
	messagesRead []*Message
	messagePending []*Message
	udpaddrConnIdMap map[*lspnet.UDPAddr]*clientInfo
	closeWriteRoutine chan bool
	readReq chan bool
	readRes chan serverReadRes
	closeConnReq chan *clientInfo
	closeConnRes chan error
	writeReq chan serverWriteReq
	messageToSend []newMessage
	writeRes chan error
	params *Params
	trigger *time.Ticker
}
type newMessage struct {
	message *Message
	udpaddr *lspnet.UDPAddr
}



// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	addr, err := lspnet.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	conn, err := lspnet.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	server := server{
		clients: make(map[int]*clientInfo),
		maxId: 0,
		newMessage: make(chan newMessage),
		sendMessage: make(chan newMessage),
		udpaddrConnIdMap: make(map[*lspnet.UDPAddr]*clientInfo),
		newConn: conn,
		closeReadRoutine: make(chan bool),
		closeServer: make(chan bool),
		messagesRead:make([]*Message,0),
		messagePending:make([]*Message,0),
		readReq: make(chan bool),
		closeMain: make(chan bool),
		read: false,
		readRes: make(chan serverReadRes),
		closeConnReq: make(chan *clientInfo),
		closeConnRes: make(chan error),
		writeReq: make(chan serverWriteReq),
		messageToSend: make([]newMessage, 0),
		writeRes: make(chan error),
		params: params,
		trigger: time.NewTicker(time.Duration(params.EpochMillis) * time.Millisecond),
	}
	go server.Main()
	go server.ReadRoutine()
	go server.WriteRoutine()
	return &server, nil
}
func (s *server) WriteRoutine() {
	log.Printf("In Server WriteRoutine")
	for {
		select {
		case <- s.closeWriteRoutine:
			return
		case sendMessage:= <- s.sendMessage:
			message, udpaddr := sendMessage.message, sendMessage.udpaddr
			res, err := json.Marshal(message)
			if err != nil {
				errors.New("Error during marshaling")
			}
			_, error := s.newConn.WriteToUDP(res, udpaddr)
			if error != nil {
				errors.New("Error during writing to UDP")
			}
			// fmt.Println("\n")
			// fmt.Println("Write To Client" + message.String())
		}
	}
}

func (s *server) Main() {
	log.Printf("In Server Main")
	for {
			// fmt.Println("\n")
		select {
		//check both readReq and len(messagesRead > 0)
		case <- s.closeMain:
			fmt.Println("close Main")
			s.closeReadRoutine <- true
			s.closeReadFunc <- true
			return
		case <- s.trigger.C:
			
			for _, cli := range s.clients {
				cli.epoch = cli.epoch + 1
				//drop client if no respond epoch num exceed
				cli.epochNorespond = cli.epochNorespond+ 1
				if cli.epochNorespond == s.params.EpochLimit && len(cli.messageWaitMap)== 0 {
					cli.Lost = true
					s.tryRead(cli,nil)
					s.drop(cli)
				} else {
					for i := 0; i < len(cli.unackedMessages); i++ {
						msg := cli.unackedMessages[i]
						if !msg.acked && msg.nextBackoff == msg.currentBackoff {
							id := cli.connId
							cli.flag = false
							addr := s.clients[id].udpAddr
							msg.updateNextBackoff(s.params.MaxBackOffInterval)
							s.sendMessage <- newMessage{message: msg.message, udpaddr:addr}
							// log.Printf("Server Resend Unacked Message: " + msg.message.String() + "At BackOff: %d", msg.nextBackoff)
						} else {
							msg.currentBackoff = msg.currentBackoff + 1
						}
					}
					if cli.flag {
						s.sendMessage <- newMessage{message: NewAck(cli.connId, 0), udpaddr: cli.udpAddr}
					}
					cli.flag = true
				}
			}

		case <- s.readReq:
			// fmt.Println("\n")
			// log.Printf("Server Read Request")
			// s.readRes = readRes
			if s.closed == true {
				s.readRes <- serverReadRes{connId: 0, payLoad: nil, err: errors.New("Server Closed")}
			} else {
				s.read = true
				s.tryRead(nil, nil)
			}
		case recievedMessage := <- s.newMessage:
			// fmt.Println("\n")
			// log.Printf("Server Recieved Message: "+ recievedMessage.message.String())
			message, addr := recievedMessage.message, recievedMessage.udpaddr
			// log.Printf(message.String())
			id := message.ConnID
			// log.Printf("ConnID: %d", id)
			if message.Type == MsgConnect {
				// fmt.Println("\n")
				// log.Printf("Server Connect")
				_, ok := s.udpaddrConnIdMap[addr]
				if ok {
					continue
				} else {
					client := s.connectClient(addr, id, s.maxId)
					s.clients[client.connId] = client
					s.maxId = s.maxId + 1
					client.epochNorespond = 0
					ackmessage := NewAck(client.connId, client.maxSeqNum)
					client.maxSeqNum = client.maxSeqNum + 1
					s.sendMessage <- newMessage{message: ackmessage, udpaddr: addr}
					s.udpaddrConnIdMap[addr] = client
				}
				
				// log.Printf("Server Connect Send Ack Success")
			}  else if message.Type == MsgAck {
				// fmt.Println("\n")
				// fmt.Println("Server Ack")
				client, ok  := s.clients[id]
				// if (len(s.messageToSend) > 0) {
				// 	s.messageToSend = s.messageToSend[1:]
				// }
				if ok {
					client.epochNorespond = 0
					s.trySend(client, message)
				}
				//also need checksum?
			} else if message.Type == MsgData {
				// fmt.Println("\n")
				// log.Printf("Server Data")
				client, ok := s.clients[id]
				// log.Printf("Client exists: %t ", ok)
				// fmt.Println("Server Main Data Processing:  ", message.String())
				// message.Payload = message.Payload[0:message.Size]
				if ok {
					log.Printf("Client MaxSeqNum: %d", client.maxSeqNum)
					log.Printf("Message SeqNum: %d", message.SeqNum)
					client.epochNorespond = 0
					ackmessage := NewAck(client.connId, message.SeqNum)
					s.sendMessage <- newMessage{message: ackmessage, udpaddr: addr}
					s.tryRead(client, message)
				} else {
					errors.New("Message Sent by client not in server Connection")
				}
			}
		case <- s.closeServer:
			s.closed = true
		case client := <- s.closeConnReq:
			fmt.Println("@@@@@@@@Server Close Connect@@@@@@@")
			if client.close == false {
				client.close = true
				if  client.unacked_count == 0 && len(client.unackedMessages) == 0 && len(client.sendMessageQueue) == 0 {
					fmt.Println("In Upper Case")
					client.closeSucceed <- true
				} else {
					s.trySend(client, nil)
				}
			} else {
				client.closeSucceed <- true
			}
		case writeReq := <- s.writeReq:
			connId, payload := writeReq.connId, writeReq.payLoad
			client, ok := s.clients[connId]
			var message *Message
			if ok {
				SeqNum := client.sendSeqNum
				message = NewData(connId, SeqNum, len(payload), payload,(uint16)(ByteArray2Checksum(payload)))
				// newmessage := newMessage{message:message, udpaddr: client.udpAddr}
				// s.messageToSend = append(s.messageToSend, newmesage)
				s.trySend(client, message)
				client.sendSeqNum = client.sendSeqNum + 1
				s.writeRes <- nil
			} else if client.close || client.Lost {
				s.writeRes <- errors.New("Client Closed/Lost")
			} else {
				s.writeRes <- errors.New("Client not found")
			}

		}
	}
}


func (msg *newsend) updateNextBackoff(MaxBackoffInterval int) {
	if msg.currentBackoff == 0 {
		msg.nextBackoff = msg.nextBackoff + 1
	} 
	doubleBackOff := msg.nextBackoff * 2
	if doubleBackOff < MaxBackoffInterval {
		msg.nextBackoff = doubleBackOff
	} else {
		msg.nextBackoff = MaxBackoffInterval
	}
	msg.currentBackoff = 0
	// fmt.Println("MaxBackOffInterval is: ", MaxBackoffInterval)
	// fmt.Println("Next BackOff: ", msg.nextBackoff, "Current BackOff: ", msg.currentBackoff)
}

func (s *server) drop(c *clientInfo) {
	connId := c.connId
	s.CloseConn(connId)
	delete(c.messageWaitMap, connId)
}

func (s *server) trySend(c *clientInfo, message *Message) {
	//Put it into pending message list
	if message != nil && message.Type == MsgData {
		// fmt.Println("Try Send Data Message")
		// fmt.Println("MessageQueue length: ", len(c.sendMessageQueue), "Windowsize: ", s.params.WindowSize, "unacked_Count: ", c.unacked_count)
		// fmt.Println("PendingMessage length: ", len(c.sendPendingMessageQueue), "Message: ", message.String())
		if c.unacked_count >= s.params.MaxUnackedMessages {
			c.sendPendingMessageQueue = append(c.sendPendingMessageQueue,message)
			return
		} else if c.unacked_count > 0 && message.SeqNum > c.unackedMessages[0].message.SeqNum + s.params.WindowSize - 1 {
			fmt.Println()
			fmt.Println("Message Append into Pending: ", message)
			fmt.Println()
			c.sendPendingMessageQueue = append(c.sendPendingMessageQueue,message)
			return 
		} else {
			c.sendMessageQueue = append(c.sendMessageQueue, message)
		}
	}
	if message != nil && message.Type == MsgAck && len(c.unackedMessages) > 0{
		//check ack == first left ack and delete it from c.unackedmessages
		// for i:= 0; i < len(c.unackedMessages); i ++ {
		// 	unackedMessage := c.unackedMessages[i]
		// 	if message.SeqNum == unackedMessage.message.SeqNum {
		// 		fmt.Println()
		// 		fmt.Println("Client Message: ", message)
		// 		fmt.Println("Client UnackedMessage", unackedMessage.message)
		// 		fmt.Println("Client UnackedMessage Length: ", len(c.unackedMessages), "Client sendMessageQueue Length: ", len(c.sendMessageQueue))
		// 		fmt.Println("Client SendMessage Queue: ", c.sendMessageQueue, "Length: ", len(c.sendMessageQueue))
		// 		fmt.Println()
		// 		c.unackedMessages = append(c.unackedMessages[:i], c.unackedMessages[i+1:]...)
		// 		// c.unackedMessages = c.unackedMessages[1:]
		// 		c.unacked_count = c.unacked_count - 1
		// 		if len(c.sendMessageQueue) <= s.params.WindowSize && len(c.sendPendingMessageQueue) > 0 {
		// 			c.sendMessageQueue = append(c.sendMessageQueue, c.sendPendingMessageQueue[0])
		// 			c.sendPendingMessageQueue = c.sendPendingMessageQueue[1:]
		// 		}
		// 	}
		// }
		if message.SeqNum == c.unackedMessages[0].message.SeqNum {
			// fmt.Println("Message: ", message)
			// fmt.Println("UnackedMessage", c.unackedMessages[0].message)
			c.unackedMessages = c.unackedMessages[1:]
			c.unacked_count = c.unacked_count - 1
			// fmt.Println("MessageQueue length: ", len(c.sendMessageQueue), "Windowsize: ", s.params.WindowSize)
			// fmt.Println("PendingMessage length: ", len(c.sendPendingMessageQueue))
			if len(c.sendMessageQueue) <= s.params.WindowSize && len(c.sendPendingMessageQueue) > 0{
				// fmt.Println("In Pending to Sending Condition")
				c.sendMessageQueue = append(c.sendMessageQueue, c.sendPendingMessageQueue[0])
				c.sendPendingMessageQueue = c.sendPendingMessageQueue[1:]
			}
		} else {
			// log.Printf("CheckSum error when removing unackedMessages")
			// errors.New("Incorrect CheckSum")
			return
		}
	}
	for {
		if len(c.sendMessageQueue) > 0 {
			msg := c.sendMessageQueue[0]
			s.sendMessage <- newMessage{message: msg, udpaddr: c.udpAddr}
			c.sendMessageQueue = c.sendMessageQueue[1:]
			unackedMsg := newsend{message:msg, acked: false, nextBackoff: 0, currentBackoff: 0}
			c.unackedMessages = append(c.unackedMessages, &unackedMsg)
			c.unacked_count = c.unacked_count + 1
			if (len(c.sendPendingMessageQueue) > 0) {
				c.sendMessageQueue = append(c.sendMessageQueue, c.sendPendingMessageQueue[0])
				c.sendPendingMessageQueue = c.sendPendingMessageQueue[1:]
			}
		} else {
			if  c.unacked_count == 0 && len(c.unackedMessages) == 0 && len(c.sendMessageQueue) == 0 && c.close {
				fmt.Println("In this Case")
				c.closeSucceed <- true
				return
			}
			return
		}
	}
}


func (s *server) tryRead(client *clientInfo, message *Message) {
	if client != nil && message != nil {
		if client.maxSeqNum == message.SeqNum {
			// client.messageQueue = append(client.messageQueue, message)
			s.messagesRead = append(s.messagesRead,message)
			client.maxSeqNum = client.maxSeqNum + 1
		} else if client.maxSeqNum < message.SeqNum {
			waitedmessage, ok := client.messageWaitMap[client.maxSeqNum]
			for ok {
				s.messagesRead = append(s.messagesRead,waitedmessage)
				delete(client.messageWaitMap, client.maxSeqNum)
				client.maxSeqNum = client.maxSeqNum + 1
				waitedmessage, ok = client.messageWaitMap[client.maxSeqNum]
			} 
			client.messageWaitMap[message.SeqNum] = message
		} else {
			errors.New("Incorrect Seq Number")
		}
	}
	
	if client != nil && client.Lost && s.read == true {
		s.readRes <- serverReadRes{payLoad: nil, err: errors.New("Client Lost")}
	}
	
	if client != nil && client.close && s.read == true {
		s.readRes <- serverReadRes{payLoad: nil, err: errors.New("Client Closed")}
	}
	// fmt.Println("Server TryRead()")
	if s.read == true && len(s.messagesRead) > 0 {
		message := s.messagesRead[0]
		s.messagesRead = s.messagesRead[1:]
		if  s.clients[message.ConnID].close {
			s.readRes <- serverReadRes{connId: 0, payLoad: nil, err: errors.New("Client Closed")}
		} else {
			s.readRes <- serverReadRes{connId: message.ConnID, payLoad: message.Payload, err: nil}
		}
		// close(s.readRes)
		// s.readRes = nil
		s.read = false
		fmt.Println("Read a message: " + message.String())
	} else {
		// log.Printf("MessagesRead length: %d", len(s.messagesRead))
	}
}

func (s *server) connectClient(addr *lspnet.UDPAddr, id int, maxId int) *clientInfo {
	// addstr = addr.String()
	// _, ok := s.clients[addstr]
	// if ok {
	// 	return clientInfo
	// }

	client := clientInfo{
		connId: maxId + 1,
		udpAddr: addr,
		maxSeqNum: 0,
		sendSeqNum:1,
		Lost: false,
		closeSucceed: make(chan bool),
		sendPendingMessageQueue: make([]*Message, 0),
		sendMessageQueue: make([]*Message,0),
		unackedMessages: make([]*newsend, 0),
		epoch: 0,
		epochNorespond: 0,
		messageWaitMap: make(map[int]*Message),
		close: false,
		flag: false,
		unacked_count: 0,
	}
	
	log.Printf("Connect New Client: %d ", id)
	return &client

}

func (s *server) ReadRoutine() {
	log.Printf("In Server ReadRoutine")
	for {
		select {
		case <- s.closeReadRoutine:
			return
		default:
			var buffer [2000]byte
			n, addr, err := s.newConn.ReadFromUDP(buffer[0:])
			if err != nil {
				errors.New("Error When ReadFrom UDP")
			}
			var msg Message
			json.Unmarshal(buffer[:n],&msg)
			if err != nil {
				errors.New("Error during unmarshaling")
			}
			//addr
			// s.newMessage <- newMessage{message: &msg, udpaddr: addr}
			if msg.Size <= len(msg.Payload) {
				msg.Payload = msg.Payload[0:msg.Size]
				s.newMessage <- newMessage{message: &msg, udpaddr: addr}
			}
			// log.Printf("ReadRoutine: Message Recieved by Server: " + msg.String() + ". Sent from Client: %d", msg.ConnID)
		}
	}
}

func (s *server) Read() (int, []byte, error) {
	// TODO: remove this line when you are ready to begin implementing this method.
	// fmt.Println("\n")
	fmt.Println("Called Server Read")
	// localch := make(chan serverReadRes)
	s.readReq <- true
	// select {
	// case <- s.closeReadFunc:
	// 	return 0, nil, errors.New("Server Closed")
	res := <- s.readRes
	return res.connId, res.payLoad, res.err
	// default:
	// 	//live lock?
	// 	s.readReq <- true

}


func (s *server) Write(connId int, payload []byte) error {
	// fmt.Println("\n")
	// fmt.Println("payload: ", payload)
	s.writeReq <- serverWriteReq{connId: connId, payLoad: payload}
	res := <- s.writeRes
	// fmt.Println("Return Value:", res)
	return res
}

func (s *server) CloseConn(connId int) error {
	fmt.Println("#######Close Conn: ", connId)
	client, ok := s.clients[connId]
	if ok {
		s.closeConnReq <- client
		fmt.Println("In this condition")
		<- client.closeSucceed
		fmt.Println("Clean up Succeed")
		return errors.New("Client Closed")
	} else {
		return errors.New("Client not exist")
	}
	
}

func (s *server) Close() error {

	fmt.Println("#########Close Server")
	s.closeServer <- true
	for _, client := range s.clients {
		s.CloseConn(client.connId)
	} 
	s.newConn.Close()
	fmt.Println("##################Close Server Succeed")
	s.closeMain <- true
	fmt.Println("##################Close Main Succeed")
	return nil
}
   