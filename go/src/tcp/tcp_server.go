package tcp

import (
	"common"
	"fmt"
	"gamelog"
	"net"
	"sync"
	"time"
)

type TCPServer struct {
	Addr            string
	MaxConnNum      int
	PendingWriteNum int
	listener        net.Listener
	mutexConns      sync.Mutex
	connset         map[net.Conn]bool
	wgLn            sync.WaitGroup
	wgConns         sync.WaitGroup
}

func NewTcpServer(addr string, maxconn int) {
	svr := new(TCPServer)
	svr.Addr = addr //"ip:port"，ip可缺省
	svr.MaxConnNum = maxconn
	svr.PendingWriteNum = 32
	svr.init()
	svr.run()
	svr.Close()
}
func (server *TCPServer) init() bool {
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		gamelog.Error("TCPServer Init failed  error :%s", err.Error())
		return false
	}

	if server.MaxConnNum <= 0 {
		server.MaxConnNum = 5000
		gamelog.Info("Invalid MaxConnNum, reset to %d", server.MaxConnNum)
	}
	if server.PendingWriteNum <= 0 {
		server.PendingWriteNum = 32
		gamelog.Info("Invalid PendingWriteNum, reset to %d", server.PendingWriteNum)
	}

	server.listener = ln
	server.connset = make(map[net.Conn]bool)

	return true
}
func (server *TCPServer) run() {
	server.wgLn.Add(1)
	defer server.wgLn.Done()
	var tempDelay time.Duration
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				gamelog.Error("accept error: %s retrying in %d", err.Error(), tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			gamelog.Error("accept error: %s", err.Error())
			return
		}
		tempDelay = 0
		connNum := len(server.connset)
		server.mutexConns.Lock()
		if connNum >= server.MaxConnNum {
			server.mutexConns.Unlock()
			conn.Close()
			gamelog.Error("too many connections")
			continue
		}

		server.connset[conn] = true
		server.mutexConns.Unlock()
		server.wgConns.Add(1)
		gamelog.Info("Connect From: %s,  ConnNum: %d", conn.RemoteAddr().String(), connNum+1)
		tcpConn := newTCPConn(conn, server.PendingWriteNum)
		tcpConn.onNetClose = func() {
			// 清理tcp_server相关数据
			server.mutexConns.Lock()
			delete(server.connset, tcpConn.conn)
			gamelog.Info("Connect Endded:Data:%v, ConnNum is:%d", tcpConn.Data, len(server.connset))
			server.mutexConns.Unlock()
			server.wgConns.Done()
		}
		go tcpConn.readRoutine()
		go tcpConn.writeRoutine()
	}
}
func (server *TCPServer) Close() {
	server.listener.Close()
	server.wgLn.Wait()

	server.mutexConns.Lock()
	for conn := range server.connset {
		conn.Close()
	}

	server.connset = nil
	server.mutexConns.Unlock()
	server.wgConns.Wait()
	gamelog.Info("server been closed!!")
}

//////////////////////////////////////////////////////////////////////
//! 模块注册
type TcpConnKey struct {
	Name string
	ID   int
}

var g_reg_conn_list = make(map[TcpConnKey]*TCPConn)

func DoRegistToSvr(conn *TCPConn, data []byte) {
	var msg Msg_Regist_To_TcpSvr
	common.ToStruct(data, &msg)
	fmt.Println("TcpScr Reg:", msg)
	g_reg_conn_list[TcpConnKey{msg.Module, msg.ID}] = conn
}
func FindRegModuleConn(module string, id int) *TCPConn {
	fmt.Println(module, id)
	fmt.Println(g_reg_conn_list)
	if v, ok := g_reg_conn_list[TcpConnKey{module, id}]; ok {
		return v
	}
	return nil
}
