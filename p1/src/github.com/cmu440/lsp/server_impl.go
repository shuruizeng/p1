// Contains the implementation of a LSP server.

package lsp

import (
	"fmt"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"errors"
	// "container/list"
)
type read_res struct {
	id_to int
	pl []byte
	err error
}

type write_req struct {
	id_from int
	pl []byte
	signal chan bool
}

type close_req struct {
	id_to int
	signal chan bool
}

type newMessage struct {
	message *Message
	addr *lspnet.UDPAddr
}

type clientInfo struct {
	udpAddr *lspnet.UDPAddr
	connId int
	seqNum int
	signal_stop bool
	messageQueue []*Message
	messageWaitMap map[int]*Message
}

type server struct {
	// TODO: Implement this!
	// packetMap map[int]*lspnet.UDPAddr
	clients map[int]*clientInfo
	messagesRead []*Message
	maxId chan int
	cur_read chan read_res
	newConn *lspnet.UDPConn
	newMessage chan newMessage
	sendMessage chan newMessage
	write_req chan write_req
	read chan(chan read_res)
	to_close chan close_req
	dropped []int
	curId int
	err_drop chan bool
	signal_close chan bool //Use it in the same manner in Close() as P0
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
	server := &server{
		// packetMap: make(map[int]*UDPAddr),
		newConn: conn,
		clients: make(map[int]*clientInfo),
		// buffer: make(chan []byte),
		maxId: make(chan int),
		// dropped: [make([]int)],
		write_req: make(chan write_req),
		read: make(chan (chan read_res)),
		cur_read: make(chan read_res),
		to_close: make(chan close_req),
		err_drop: make(chan bool),
		signal_close: make(chan bool),
		messagesRead: make([]*Message, 0),
	}
	go server.Main()
	go server.ReadMessage()
	return server, nil
}

//Main Routine that process all reads & write changes, handle locks related variable here only!!!
func (s *server) Main() {
	for {
		select {
		case <- s.signal_close:
			return
		case wr_req := <- s.write_req:
			id, payload := wr_req.id_from, wr_req.pl
			c, found := s.find(id)
			if found {
				data := c.create_data(payload)
				c.messageQueue = append(c.messageQueue, data)
				s.send(c)
				wr_req.signal <- true
			} else {
				wr_req.signal <- false
			}
		case close_request := <- s.to_close:
			id := close_request.id_to
			c, found := s.clients[id]
			if found {
				s.read_quit(id)
				c.signal_stop = true
				close_request.signal <- true
			} else {
				close_request.signal <- false
			}
		case recievedMessage := <- s.newMessage:
			message, addr := recievedMessage.message, recievedMessage.addr
			id := message.ConnID
			if message.Type == MsgConnect {
				client := s.connectClient(addr, id)
				ackmessage := NewAck(client.connId, client.seqNum)
				s.sendMessage <- newMessage{message: ackmessage, addr: addr}
			}  else if message.Type == MsgAck {
				// client, ok  := s.clients[id]
				continue
				//also need checksum?
			} else if message.Type == MsgData {
				client, ok := s.clients[id]
				if ok {
					ackmessage := NewAck(client.connId, client.seqNum)
					// s.sendMessage <- newMessage{message: message, addr: addr}
					if client.seqNum == message.SeqNum {
						client.messageQueue = append(client.messageQueue, ackmessage)
						s.messagesRead = append(s.messagesRead,ackmessage)
						client.seqNum = client.seqNum + 1
					} else {
						if client.seqNum > message.SeqNum {
							continue
						} 
						message, ok = client.messageWaitMap[client.seqNum]
						if ok {
							client.messageQueue = append(client.messageQueue, message)
							s.messagesRead = append(s.messagesRead,message)
							client.seqNum += 1
						}
						client.messageWaitMap[message.SeqNum] = message 
					}
				}
			}
		case sendReq := <- s.sendMessage:
			message, addr := sendReq.message, sendReq.addr
			res, err := json.Marshal(message)
			if err != nil {
				errors.New("Error during marshaling")
			} 
			_, error := s.newConn.WriteToUDP(res, addr)
			if error != nil {
				errors.New("Error during writing to UDP")
			}
		}
	}
}



func (s *server) connectClient(addr *lspnet.UDPAddr, id int) *clientInfo {
	// addstr = addr.String()
	// _, ok := s.clients[addstr]
	// if ok {
	// 	return clientInfo
	// } 
	id = <- s.maxId
	s.maxId <- id + 1

	client := clientInfo{
		connId: id,
		udpAddr: addr,
		seqNum: 0,
	}
	s.clients[id] = &client
	return &client

}

//Read Routine that loops to read message from client and store in packetMap
func (s *server) ReadMessage() {
	for {
		select {
		case <- s.to_close:
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
			s.newMessage <- newMessage{message: &msg, addr: addr}
		}
	}
}




func (s *server) Read() (int, []byte, error) {
	for {
		select {
		case <- s.signal_close:
			return 0, nil, errors.New("Server Closed")
		default:
			if len(s.messagesRead) > 0{
				message := s.messagesRead[0]
				s.messagesRead = s.messagesRead[1:]
				if  s.clients[message.ConnID].signal_stop {
					return 0, nil, errors.New("Client Closed")
				} else {
					return message.ConnID, message.Payload, nil
				}
				
			}
		} 
		
	}
}

func (s *server) Write(connId int, payload []byte) error {
	//Not sure what pkg to use for writing data and sending msg back to client
	//The structure should be similar to read
	ch := make(chan bool)
    cur_req := write_req{id_from: connId, pl: payload, signal: ch}
	s.write_req <- cur_req
	result := <- ch
	if result {
		return nil
	} else {
		return errors.New("ConnID not found")
	}
}

func (s *server) CloseConn(connId int) error {
	cur_ch := make(chan bool)
	s.to_close <- close_req{id_to: connId, signal: cur_ch}
	end := <- cur_ch
	if end {
		return nil
	}
	return errors.New("id not found")

}

func (s *server) Close() error {
	s.signal_close <- true
	return nil
}

func (s *server)find(connId int) (*clientInfo, bool) {
	cli, ok := s.clients[connId]
	if ok {
		return cli, ok
	} else {
		return nil, ok
	}
}
// func (s *server)read_quit(id int) {
// 	if s.cur_read != nil {
// 		s.cur_read <- read_res{id_to: id, pl: nil, err:errors.New("read routinestopped")}
// 	}
// }
func (c *clientInfo) create_data(payload []byte) *Message {
	cs := ByteArray2Checksum(payload)
	data := NewData(c.connId, c.seqNum, len(payload), payload, uint16(cs))
	c.seqNum = c.seqNum + 1
	return data
}
func (s *server) send(c *clientInfo) {
	if len(c.messageQueue) == 0 {
		return
	} else {
		cur := c.messageQueue[0]
		c.messageQueue = c.messageQueue[1:]
		udp_addr := c.udpAddr
		s.sendMessage <- newMessage{message: cur, addr: udp_addr}
	}
}
func (s *server)read_quit(id int) {
	if s.cur_read != nil {
		s.cur_read <- read_res{id_to: id, pl: nil, err:errors.New("read routinestopped")}
	}
}
