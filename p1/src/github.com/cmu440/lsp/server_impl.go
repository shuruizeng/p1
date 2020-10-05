// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"fmt"
	"encoding/json"
	"log"
)

type clientInfo struct {
	connId int
	udpAddr *lspnet.UDPAddr
	maxSeqNum int
	sendSeqNum int
	messageQueue []*Message
	messageWaitMap map[int]*Message
	close bool
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
	messagesRead []*Message
	messagePending []*Message
	closeWriteRoutine chan bool
	readReq chan chan serverReadRes
	readRes chan serverReadRes
	closeConnReq chan int
	closeConnRes chan error
	writeReq chan serverWriteReq
	unackedMessage []newMessage
	writeRes chan error
	params *Params
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
		newConn: conn,
		closeReadRoutine: make(chan bool),
		closeServer: make(chan bool),
		messagesRead:make([]*Message,0),
		messagePending:make([]*Message,0),
		readReq: make(chan chan serverReadRes),
		readRes: make(chan serverReadRes),
		closeConnReq: make(chan int),
		closeConnRes: make(chan error),
		writeReq: make(chan serverWriteReq),
		unackedMessage: make([]newMessage, 0),
		writeRes: make(chan error),
		params: params,
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
			fmt.Println("Write To Client" + message.String())
		}
	}
}

func (s *server) Main() {
	log.Printf("In Server Main")
	for {
			// fmt.Println("\n")
		select {
		//check both readReq and len(messagesRead > 0)
		case readRes := <- s.readReq :
			// fmt.Println("\n")
			log.Printf("Server Read Request")
			s.readRes = readRes
			s.tryRead()
		case recievedMessage := <- s.newMessage:
			// fmt.Println("\n")
			log.Printf("Server Recieved Message")
			message, addr := recievedMessage.message, recievedMessage.udpaddr
			log.Printf(message.String())
			id := message.ConnID
			log.Printf("ConnID: %d", id)
			fmt.Println(s.clients[id])
			if message.Type == MsgConnect {
				// fmt.Println("\n")
				log.Printf("Server Connect")
				client := s.connectClient(addr, id, s.maxId)
				s.clients[client.connId] = client
				s.maxId = s.maxId + 1
				ackmessage := NewAck(client.connId, client.maxSeqNum)
				client.maxSeqNum = client.maxSeqNum + 1
				s.sendMessage <- newMessage{message: ackmessage, udpaddr: addr}
				log.Printf("Server Send Ack Success")
			}  else if message.Type == MsgAck {
				// fmt.Println("\n")
				fmt.Println("Server Ack")
				// client, ok  := s.clients[id]
				if (len(s.unackedMessage) > 0) {
					s.unackedMessage = s.unackedMessage[1:]
				}
				s.trySend()
				//also need checksum?
			} else if message.Type == MsgData {
				// fmt.Println("\n")
				log.Printf("Server Data")
				client, ok := s.clients[id]
				log.Printf("Client exists: %t ", ok)
				
				if ok {
					log.Printf("Client MaxSeqNum: %d", client.maxSeqNum)
					log.Printf("Message SeqNum: %d", message.SeqNum)
					if client.maxSeqNum == message.SeqNum {
						// client.messageQueue = append(client.messageQueue, message)
						ackmessage := NewAck(client.connId, client.maxSeqNum)
						s.messagesRead = append(s.messagesRead,message)
						client.maxSeqNum = client.maxSeqNum + 1
						s.sendMessage <- newMessage{message: ackmessage, udpaddr: addr}
					} else if client.maxSeqNum < message.SeqNum {
						waitedmessage, ok := client.messageWaitMap[client.maxSeqNum]
						if ok {
							s.messagesRead = append(s.messagesRead,waitedmessage)
							client.maxSeqNum = client.maxSeqNum + 1
							delete(client.messageWaitMap, client.maxSeqNum)
						} else {
							client.messageWaitMap[client.maxSeqNum] = message
						}
					} else {
						errors.New("Incorrect Seq Number")
					}
					s.tryRead()
				} else {
					errors.New("Message Sent by client not in server Connection")
				}
			}
		case <- s.closeServer:
			for _, client := range s.clients {
				s.CloseConn(client.connId)
			}
		case connId := <- s.closeConnReq:
			fmt.Println("Server Close Connect")
			client, ok := s.clients[connId]
			if ok && client.close == false {
				client.close = true
				s.closeConnRes <- nil
			} else {
				s.closeConnRes <- errors.New("client not exist or closed")
			}
		case writeReq := <- s.writeReq:
			connId, payload := writeReq.connId, writeReq.payLoad
			client, ok := s.clients[connId]
			var message *Message
			if ok {
				SeqNum := client.sendSeqNum
				client.sendSeqNum = client.sendSeqNum + 1
				message = NewData(connId, SeqNum, len(payload), payload,(uint16)(ByteArray2Checksum(payload)))
				newmessage := newMessage{message:message, udpaddr: client.udpAddr}
				s.unackedMessage = append(s.unackedMessage, newmessage)
				s.trySend()
				s.writeRes <- nil
			} else {
				s.writeRes <- errors.New("Client not found")
			}
			
		}
	}
}

func (s *server) trySend() {
	if len(s.unackedMessage) > s.params.WindowSize {
		return 
	} else if len(s.unackedMessage) > 0 {
		msg := s.unackedMessage[0]
		s.sendMessage <- msg
		s.unackedMessage = s.unackedMessage[1:]
	} else {
		return
	}
}


func (s *server) tryRead() {
	if s.readRes != nil && len(s.messagesRead) > 0 {
		message := s.messagesRead[0]
		s.messagesRead = s.messagesRead[1:]
		if  s.clients[message.ConnID].close {
			s.readRes <- serverReadRes{connId: 0, payLoad: nil, err: errors.New("Client Closed")}
		} else {
			s.readRes <- serverReadRes{connId: message.ConnID, payLoad: message.Payload, err: nil}
		}
		close(s.readRes)
		s.readRes = nil
		fmt.Println("Read a message: " + message.String())
	} else {
		log.Printf("MessagesRead length: %d", len(s.messagesRead))
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
		sendSeqNum:0,
		messageQueue: make([]*Message,0),
		messageWaitMap: make(map[int]*Message),
		close: false,
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
			var buffer [1000]byte
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
			s.newMessage <- newMessage{message: &msg, udpaddr: addr}
			log.Printf("ReadRoutine: Message Recieved by Server")
			log.Printf(msg.String())
			log.Printf("Client ConnId: %d", msg.ConnID)
		}
	}
}

func (s *server) Read() (int, []byte, error) {
	// TODO: remove this line when you are ready to begin implementing this method.
	// fmt.Println("\n")
	fmt.Println("Call Server Read")
	localch := make(chan serverReadRes)
	s.readReq <- localch
	select {
	case <- s.closeReadFunc:
		return 0, nil, errors.New("Server Closed")
	case res := <- localch:
		return res.connId, res.payLoad, res.err
	// default:
	// 	//live lock?
	// 	s.readReq <- true
	}

}


func (s *server) Write(connId int, payload []byte) error {
	// fmt.Println("\n")
	fmt.Println("payload: ", payload)
	s.writeReq <- serverWriteReq{connId: connId, payLoad: payload}
	res := <- s.writeRes
	fmt.Println("Return Value:", res)
	return res
}

func (s *server) CloseConn(connId int) error {
	s.closeConnReq <- connId
	res := <- s.closeConnRes
	return res
}

func (s *server) Close() error {
	s.closeServer <- true
	s.closeReadRoutine <- true
	s.closeReadFunc <- true
	return nil
}
