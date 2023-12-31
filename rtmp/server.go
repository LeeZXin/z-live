package rtmp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/LeeZXin/z-live/flv"
	"github.com/LeeZXin/z-live/hls"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/strutil"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"io"
	"net"
	"sync"
	"time"
)

const (
	netTimeout = 3 * time.Second
)

/*
TcpServer rtmp tcp服务端
接收rtmp推流数据
*/
type TcpServer struct {
	netTimeout time.Duration

	listener net.Listener
	ctx      context.Context
	cancelFn context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once
}

func NewTcpServer(addr string) *TcpServer {
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Logger.Panic(err)
	}
	return newServer(tcp)
}

func NewTlsServer(addr string) *TcpServer {
	cert, err := tls.LoadX509KeyPair(static.GetString("rtmp.tls.path.ca"), static.GetString("rtmp.tls.path.key"))
	if err != nil {
		logger.Logger.Panic(err)
	}
	tcp, err := tls.Listen("tcp", addr, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		logger.Logger.Panic(err)
	}
	return newServer(tcp)
}

func newServer(listener net.Listener) *TcpServer {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &TcpServer{
		netTimeout: 10 * time.Second,
		listener:   listener,
		ctx:        ctx,
		cancelFn:   cancelFunc,
		startOnce:  sync.Once{},
		stopOnce:   sync.Once{},
	}
}

func (r *TcpServer) ListenAndServe() {
	r.startOnce.Do(func() {
		logger.Logger.Info("listen rtmp tcp server: ", r.listener.Addr())
		quit.AddShutdownHook(func() {
			r.Shutdown()
			closeAllPublisher()
		})
		go func() {
			for {
				if r.ctx.Err() != nil {
					return
				}
				conn, err := r.listener.Accept()
				if err != nil {
					logger.Logger.Error(err)
					r.Shutdown()
					return
				}
				go func() {
					err2 := threadutil.RunSafe(func() {
						// 处理tcp连接
						r.handleConn(newNetConn(conn, 4*1024))
					}, func() {
						conn.Close()
					})
					if err2 != nil {
						logger.Logger.Error(err2)
						conn.Close()
					}
				}()
			}
		}()
	})
}

func (r *TcpServer) handleConn(conn *netConn) {
	defer func() {
		conn.Close()
	}()
	// 握手
	if err := handshake2Client(conn); err != nil {
		if !errors.Is(err, io.EOF) {
			logger.Logger.Error(err)
		}
		return
	}
	for {
		if r.ctx.Err() != nil {
			return
		}
		if err := readAndHandleUserCtrlMsg(conn); err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Logger.Error(err)
			}
			return
		}
		switch conn.cs.typeId {
		case 17, 20:
			if err := handleCmdMsg(conn); err != nil {
				if !errors.Is(err, io.EOF) {
					logger.Logger.Error(err)
				}
				return
			}
		}
		if conn.done {
			break
		}
	}
	if conn.cmdHandler.cntCmd == nil || conn.cmdHandler.pubCmd == nil {
		return
	}
	app, name := conn.cmdHandler.getKey()
	key := app + "/" + name
	// 推流
	if conn.isPublisher {
		publisher := newStreamPublisher(conn)
		defer publisher.close()
		registerPublisher(key, publisher)
		defer deregisterPublisher(key)
		// 将整个视频文件保存到本地
		flvFileWriter, err := flv.NewFileWriter(fmt.Sprintf("./%s_%s_%d.flv", name, strutil.RandomStr(5), time.Now().UnixMilli()))
		if err == nil {
			publisher.Register(flvFileWriter)
		}
		// 可以用hls播放
		hlsWriter := hls.NewStreamWriter(app, name)
		publisher.Register(hlsWriter)
		publisher.start()
	} else {
		publisher, ok := FindPublisher(key)
		if ok {
			// 拉流
			writer := newStreamWriter(conn)
			publisher.Register(writer)
			writer.Start()
		} else {
			conn.Close()
		}
	}
}

// readAndHandleUserCtrlMsg 读chunkStream并处理控制命令
func readAndHandleUserCtrlMsg(conn *netConn) error {
	err := readChunkStream(conn)
	if err != nil {
		return err
	}
	conn.handleUserControlMsg()
	// 回ack
	err = conn.ack()
	if err != nil {
		return err
	}
	return err
}

func readChunkStream(conn *netConn) error {
	// 读取chunkStream
	for {
		// 读取fmt和csid
		tfmt, csid, err := conn.readFmtAndCsid()
		logger.Logger.Debugf("fmt: %v csid: %v", tfmt, csid)
		if err != nil {
			return err
		}
		logger.Logger.Debugf("read csid: %v tfmt: %v", csid, tfmt)
		cs, b := conn.chunks[csid]
		if !b {
			cs = &chunkStream{
				csid: csid,
				tfmt: tfmt,
			}
		} else {
			cs.tfmt = tfmt
			cs.csid = csid
		}
		err = cs.readChunk(conn)
		if err != nil {
			return err
		}
		conn.chunks[csid] = cs
		if cs.isReadDone() {
			conn.cs = cs
			break
		}
	}
	return nil
}

func (r *TcpServer) Shutdown() {
	r.stopOnce.Do(func() {
		logger.Logger.Info("shutdown rtmp tcp server")
		r.cancelFn()
	})
}
