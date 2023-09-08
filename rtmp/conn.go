package rtmp

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/LeeZXin/z-live/amf"
	"github.com/LeeZXin/zsf/logger"
	"io"
	"net"
)

const (
	idSetChunkSize = iota + 1
	idAbortMessage
	idAck
	idUserControlMessages
	idWindowAckSize
	idSetPeerBandwidth
)

const (
	streamBegin      uint32 = 0
	streamEOF        uint32 = 1
	streamDry        uint32 = 2
	setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	pingRequest      uint32 = 6
	pingResponse     uint32 = 7
)

var (
	cmdConnect       = "connect"
	cmdFCPublish     = "FCPublish"
	cmdReleaseStream = "releaseStream"
	cmdCreateStream  = "createStream"
	cmdPublish       = "publish"
	cmdFCUnpublish   = "FCUnpublish"
	cmdDeleteStream  = "deleteStream"
	cmdPlay          = "play"
)

// netConn rtmp单个conn
type netConn struct {
	net.Conn
	amfCodec            *amfCodec
	chunkSize           uint32
	remoteChunkSize     uint32
	windowAckSize       uint32
	remoteWindowAckSize uint32
	received            uint32
	ackReceived         uint32
	done                bool
	isPublisher         bool
	buf                 *bufio.ReadWriter
	cs                  *chunkStream
	cmdHandler          *cmdHandler
	chunks              map[uint32]*chunkStream
}

func newNetConn(conn net.Conn, bufSize int) *netConn {
	ret := &netConn{
		chunks:              make(map[uint32]*chunkStream, 8),
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		amfCodec:            newAmfCodec(),
		Conn:                conn,
		buf:                 bufio.NewReadWriter(bufio.NewReaderSize(conn, bufSize), bufio.NewWriterSize(conn, bufSize)),
	}
	h := &cmdHandler{
		streamId: 1,
		conn:     ret,
		buf:      &bytes.Buffer{},
	}
	ret.cmdHandler = h
	return ret
}

func (c *netConn) Read(p []byte) (int, error) {
	return io.ReadAtLeast(c.buf, p, len(p))
}

func (c *netConn) Write(p []byte) (int, error) {
	return c.buf.Write(p)
}

func (c *netConn) Flush() error {
	return c.buf.Flush()
}

func (c *netConn) ReadUintBE(n int) (uint32, error) {
	ret := uint32(0)
	for i := 0; i < n; i++ {
		b, err := c.buf.ReadByte()
		if err != nil {
			return 0, err
		}
		ret = ret<<8 + uint32(b)
	}
	return ret, nil
}

// ReadUintLE 读小端字节
func (c *netConn) ReadUintLE(n int) (uint32, error) {
	ret := uint32(0)
	for i := 0; i < n; i++ {
		b, err := c.buf.ReadByte()
		if err != nil {
			return 0, err
		}
		ret += uint32(b) << uint32(i*8)
	}
	return ret, nil
}

// WriteUintBE 写大端字节
func (c *netConn) WriteUintBE(v uint32, n int) error {
	for i := 0; i < n; i++ {
		b := byte(v>>uint32((n-i-1)<<3)) & 0xff
		if err := c.buf.WriteByte(b); err != nil {
			return err
		}
	}
	return nil
}

func (c *netConn) WriteUintLE(v uint32, n int) error {
	for i := 0; i < n; i++ {
		b := byte(v) & 0xff
		if err := c.buf.WriteByte(b); err != nil {
			return err
		}
		v = v >> 8
	}
	return nil
}

func (c *netConn) readFmt() (uint32, uint32, error) {
	h, err := c.ReadUintBE(1)
	if err != nil {
		return 0, 0, err
	}
	return h >> 6, h & 0x3f, nil
}

// correctCsid 纠正csid
func (c *netConn) correctCsid(csid uint32) (uint32, error) {
	switch csid {
	case 0:
		id, err := c.ReadUintLE(1)
		if err != nil {
			return 0, err
		}
		return id + 64, nil
	case 1:
		id, err := c.ReadUintLE(2)
		if err != nil {
			return 0, err
		}
		return id + 64, nil
	}
	return csid, nil
}

// readFmtAndCsid 读fmt和csid
func (c *netConn) readFmtAndCsid() (uint32, uint32, error) {
	cfmt, csid, err := c.readFmt()
	if err != nil {
		return 0, 0, err
	}
	// 当低6位为0或1时 csid得特殊处理
	csid, err = c.correctCsid(csid)
	if err != nil {
		return 0, 0, err
	}
	return cfmt, csid, nil
}

// handleUserControlMsg 基本命令
func (c *netConn) handleUserControlMsg() {
	switch c.cs.typeId {
	case idSetChunkSize:
		c.remoteChunkSize = binary.BigEndian.Uint32(c.cs.data)
	case idWindowAckSize:
		c.remoteWindowAckSize = binary.BigEndian.Uint32(c.cs.data)
	case idAbortMessage:
	case idAck:
	case idUserControlMessages:
	case idSetPeerBandwidth:
	}
}

// handleCmdMsg 处理命令消息
func handleCmdMsg(c *netConn) error {
	amfType := amf.AMF0
	if c.cs.typeId == 17 {
		c.cs.data = c.cs.data[1:]
	}
	r := bytes.NewReader(c.cs.data)
	vs, err := c.amfCodec.DecodeBatch(r, amf.Version(amfType))
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	handler := c.cmdHandler
	cs := c.cs
	switch vs[0].(type) {
	case string:
		switch vs[0].(string) {
		case cmdConnect:
			if err = handler.connect(vs[1:]); err != nil {
				return err
			}
			if err = handler.connectResp(cs); err != nil {
				return err
			}
		case cmdCreateStream:
			if err = handler.createStream(vs[1:]); err != nil {
				return err
			}
			if err = handler.createStreamResp(cs); err != nil {
				return err
			}
		case cmdPublish:
			if err = handler.publishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = handler.publishResp(cs); err != nil {
				return err
			}
			c.done = true
			c.isPublisher = true
		case cmdPlay:
			if err = handler.publishOrPlay(vs[1:]); err != nil {
				return err
			}
			if err = handler.playResp(cs); err != nil {
				return err
			}
			c.done = true
			c.isPublisher = false
		case cmdFCPublish:
			if err = handler.fcPublish(vs); err != nil {
				return err
			}
		case cmdReleaseStream:
			if err = handler.releaseStream(vs); err != nil {
				return err
			}
		case cmdFCUnpublish:
		case cmdDeleteStream:
		default:
			logger.Logger.Info("no support command=", vs[0].(string))
		}
	}

	return nil
}

func (c *netConn) ack() error {
	size := c.cs.length
	c.received += size
	c.ackReceived += size
	if c.received >= 0xf0000000 {
		c.received = 0
	}
	if c.ackReceived >= c.remoteWindowAckSize {
		cs := newAckCs(c.ackReceived)
		err := cs.writeChunk(c)
		c.ackReceived = 0
		return err
	}
	return nil
}
