// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"net"
)



type server struct {
	// TODO: Implement this!
	packetMap *map[int]*Message
	maxSeq chan int
	newConn chan *UDPConn
	clients []*UDPConn
	buffer chan []byte
	curId chan int
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
	conn, err = lspnet.ListenUDP("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	server := &server{
		packetMap: &make(map[int]*Message),
		maxSeq: make(chan int),
		newConn: make(chan *UDPConn),
		clients: make([]*UDPConn),
		buffer: make(chan []byte),
		curId: make(chan int),
		signal_close: make(chan bool)
	}
	server.newConn <- conn
	go server.accept()
	go server.Main()
	go server.ReadMessage()
	return server, nil
}

func (s *server) accept() {
	for {
		select {
			case c := <- s.newConn:
				s.clients = append(s.clients, c)
				val := len(s.clients)
				//Accept connection and send message. Not sure if it is
				//better to use client structure or just connector
			default:
				continue
		}
	}
}

//Main Routine that process all reads & write changes, handle locks related variable here only!!!
func (s *server) Main() {
	for {
		select {
		case <- s.signal_close:
			s.close_all() //Will implement close_all to close all available
			//connections
			return
		case c := <- s.newConn:
			go accept(s, c)
		//Not sure if we will need a separate routine for this or just do it in
		//read
		case buf := <- s.buffer:
			go s.Read()


		}
	}
}

//Read Routine that loops to read message from client and store in packetMap
func (s *server) ReadMessage() {
	for conn := range s.clients {
		//Not sure which function to use to get the data for this connection
		//Use data as the obtained message
		if data.Type == MsgData {
			s.packetMap[data.ConnId] = data
			s.buffer <- data.Payload
			s.curId <- data.ConnId
		}
	}
}


func (s *server) Read() (int, []byte, error) {
	for {
		select {
			case buf := <- server.buffer:
				//send ack message. Not sure what package to use
			default:
				continue
		}
	}
}

func (s *server) Write(connId int, payload []byte) error {
	//Not sure what pkg to use for writing data and sending msg back to client
	//The structure should be similar to read
}

func (s *server) CloseConn(connId int) error {

}

func (s *server) Close() error {
	s.signal_close <- true
}
