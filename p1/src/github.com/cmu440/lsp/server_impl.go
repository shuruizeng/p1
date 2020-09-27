// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"net"
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

type client struct {
	addr *lspnet.UDPAddr
	id int
	signal_stop chan bool
}

type server struct {
	// TODO: Implement this!
	packetMap map[int]*lspnet.UDPAddr
	maxSeq chan int
	newConn *lspnet.UDPConn
	write chan write_req
	read chan read_res
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
	conn, err = lspnet.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	server := &server{
		packetMap: make(map[int]*UDPAddr),
		maxSeq: make(chan int),
		newConn: conn,
		clients: make([]*UDPConn),
		buffer: make(chan []byte),
		curId: 1,
		dropped: make([]int)
		write: make(chan write_req),
		read: make(chan read_req),
		to_close: make(chan close_req),
		err_drop: make(chan bool),
		signal_close: make(chan bool)
	}
	go server.Main()
	go server.ReadMessage()
	return &server, nil
}

//Main Routine that process all reads & write changes, handle locks related variable here only!!!
func (s *server) Main() {
	for {
		select {
		case <- s.signal_close:
			return
		case wr_req := <- s.write_req:

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
	ch := make(chan bool)
    cur_req := write_req{id_from: connId, pl: payload, signal: ch}
	s.write <- cur_req
	result := <- ch
	if result {
		return nil
	}

}

func (s *server) CloseConn(connId int) error {
	cur_ch := make(chan bool)
	s.to_close

}

func (s *server) Close() error {
	s.signal_close <- true
}
