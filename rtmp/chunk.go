package rtmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/z-live/util/bytesutil"
)

// chunkStream rtmp单个chunk流
type chunkStream struct {
	tfmt      uint32
	fmt       uint32
	csid      uint32
	timestamp uint32
	length    uint32
	typeId    uint32
	streamId  uint32
	timeDelta uint32
	hasExtTs  bool
	index     uint32
	remain    uint32
	readDone  bool
	data      []byte
}

func (c *chunkStream) toString() string {
	return fmt.Sprintf("fmt: %v, csid: %v, timestamp: %v, length: %v, typeId: %v, streamId: %v, timeDelta: %v, hasExtTs: %v, index: %v, remain: %v, readDone: %v msglen: %v",
		c.fmt, c.csid, c.timestamp, c.length, c.typeId, c.streamId, c.timeDelta, c.hasExtTs, c.index, c.remain, c.readDone, len(c.data))
}

// isReadDone 判断单个流是否读取完毕
func (c *chunkStream) isReadDone() bool {
	return c.readDone
}

// new 初始化
func (c *chunkStream) new() {
	c.readDone = false
	c.index = 0
	c.remain = c.length
	c.data = make([]byte, c.length)
}

// writeBasicHeader 写基础头部
func (c *chunkStream) writeBasicHeader(conn *netConn) error {
	h := c.fmt << 6
	if c.csid < 64 {
		h |= c.csid
		return conn.WriteUintBE(h, 1)
	}
	if c.csid-64 < 256 {
		h |= 0
		err := conn.WriteUintBE(h, 1)
		if err == nil {
			return conn.WriteUintLE(c.csid-64, 1)
		} else {
			return err
		}
	}
	if c.csid-64 < 65536 {
		h |= 1
		err := conn.WriteUintBE(h, 1)
		if err == nil {
			return conn.WriteUintLE(c.csid-64, 2)
		} else {
			return err
		}
	}
	return errors.New("wrong csid")
}

/*
fmt = 0
| -- timestamp 3b -- | -- message length 3b -- | -- typeid 1b -- | -- stream id 4b -- | -- extended timestamp 4b -- |
fmt = 1
| -- timedelta 3b -- | -- message length 3b -- | -- typeid 1b -- | -- extended timestamp 4b -- |
fmt = 2
| -- timedelta 3b -- | -- extended timestamp 4b -- |
fmt = 3
| -- 0b -- |
*/
func (c *chunkStream) writeHeader(conn *netConn) error {
	if err := c.writeBasicHeader(conn); err != nil {
		return err
	}
	ts := c.timestamp
	if c.fmt != 3 {
		if c.timestamp > 0xffffff {
			ts = 0xffffff
		}
		if err := conn.WriteUintBE(ts, 3); err != nil {
			return err
		}
		if c.fmt != 2 {
			if c.length > 0xffffff {
				return fmt.Errorf("length=%d", c.length)
			}
			if err := conn.WriteUintBE(c.length, 3); err != nil {
				return err
			}
			if err := conn.WriteUintBE(c.typeId, 1); err != nil {
				return err
			}
			if c.fmt != 1 {
				if err := conn.WriteUintLE(c.streamId, 4); err != nil {
					return err
				}
			}
		}
	}
	if ts >= 0xffffff {
		return conn.WriteUintBE(c.timestamp, 4)
	}
	return nil
}

// writeChunk 将chunk流写到conn中
func (c *chunkStream) writeChunk(conn *netConn) error {
	if c.typeId == idSetChunkSize {
		conn.chunkSize = binary.BigEndian.Uint32(c.data)
	}
	chunkSize := conn.chunkSize
	if c.typeId == av.TAG_AUDIO {
		c.csid = 4
	} else if c.typeId == av.TAG_VIDEO ||
		c.typeId == av.TAG_SCRIPTDATAAMF0 ||
		c.typeId == av.TAG_SCRIPTDATAAMF3 {
		c.csid = 6
	}
	totalLen := uint32(0)
	numChunks := c.length / chunkSize
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == c.length {
			break
		}
		if i == 0 {
			c.fmt = uint32(0)
		} else {
			c.fmt = uint32(3)
		}
		if err := c.writeHeader(conn); err != nil {
			return err
		}
		inc := chunkSize
		start := i * chunkSize
		if uint32(len(c.data))-start <= inc {
			inc = uint32(len(c.data)) - start
		}
		totalLen += inc
		end := start + inc
		buf := c.data[start:end]
		if _, err := conn.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// readFmt0
// | -- timestamp 3b -- | -- message length 3b -- | -- typeid 1b -- | -- stream id 4b -- | -- extended timestamp 4b -- |
func (c *chunkStream) readFmt0(conn *netConn) error {
	ts, err := conn.ReadUintBE(3)
	if err != nil {
		return err
	}
	c.length, err = conn.ReadUintBE(3)
	if err != nil {
		return err
	}
	c.typeId, err = conn.ReadUintBE(1)
	if err != nil {
		return err
	}
	c.streamId, err = conn.ReadUintLE(4)
	if err != nil {
		return err
	}
	if ts == 0xffffff {
		ts, err = conn.ReadUintBE(4)
		if err != nil {
			return err
		}
		c.hasExtTs = true
	} else {
		c.hasExtTs = false
	}
	c.timestamp = ts
	c.new()
	return nil
}

// fmt = 1
// | -- timedelta 3b -- | -- message length 3b -- | -- typeid 1b -- | -- extended timestamp 4b -- |
func (c *chunkStream) readFmt1(conn *netConn) error {
	ts, err := conn.ReadUintBE(3)
	if err != nil {
		return err
	}
	c.length, err = conn.ReadUintBE(3)
	if err != nil {
		return err
	}
	c.typeId, err = conn.ReadUintBE(1)
	if err != nil {
		return err
	}
	if ts == 0xffffff {
		ts, err = conn.ReadUintBE(4)
		c.hasExtTs = true
	} else {
		c.hasExtTs = false
	}
	c.timeDelta = ts
	c.timestamp += ts
	c.new()
	return nil
}

func (c *chunkStream) readFmt2(conn *netConn) error {
	ts, err := conn.ReadUintBE(3)
	if err != nil {
		return err
	}
	if ts == 0xffffff {
		ts, err = conn.ReadUintBE(4)
		if err != nil {
			return err
		}
		c.hasExtTs = true
	} else {
		c.hasExtTs = false
	}
	c.timeDelta = ts
	c.timestamp += ts
	c.new()
	return nil
}

func (c *chunkStream) readFmt3(conn *netConn) error {
	if c.remain == 0 {
		if c.hasExtTs {
			ts, err := conn.ReadUintBE(4)
			if err != nil {
				return err
			}
			switch c.fmt {
			case 0:
				c.timestamp = ts
			case 1, 2:
				c.timestamp += ts
			}
		} else {
			switch c.fmt {
			case 1, 2:
				c.timestamp += c.timeDelta
			}
		}
		c.new()
	}
	return nil
}

func (c *chunkStream) readChunk(conn *netConn) error {
	if c.remain != 0 && c.tfmt != 3 {
		return fmt.Errorf("invalid remain = %d", c.remain)
	}
	chunkSize := conn.remoteChunkSize
	switch c.tfmt {
	case 0:
		c.fmt = 0
		if err := c.readFmt0(conn); err != nil {
			return err
		}
	case 1:
		c.fmt = 1
		if err := c.readFmt1(conn); err != nil {
			return err
		}
	case 2:
		c.fmt = 2
		if err := c.readFmt2(conn); err != nil {
			return err
		}
	case 3:
		if err := c.readFmt3(conn); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid format=%d", c.fmt)
	}
	size := int(c.remain)
	if size > int(chunkSize) {
		size = int(chunkSize)
	}
	buf := c.data[c.index : c.index+uint32(size)]
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	c.index += uint32(size)
	c.remain -= uint32(size)
	if c.remain == 0 {
		c.readDone = true
	}
	//logger.Logger.Debugf("cs: %s", c.toString())
	return nil
}

func initControlMsg(id, size, value uint32) chunkStream {
	ret := chunkStream{
		fmt:      0,
		csid:     2,
		typeId:   id,
		streamId: 0,
		length:   size,
		data:     make([]byte, size),
	}
	bytesutil.PutU32BE(ret.data[:size], value)
	return ret
}

func newAckCs(size uint32) chunkStream {
	return initControlMsg(idAck, 4, size)
}

func newSetChunkSizeCs(size uint32) chunkStream {
	return initControlMsg(idSetChunkSize, 4, size)
}

func newWindowAckSizeCs(size uint32) chunkStream {
	return initControlMsg(idWindowAckSize, 4, size)
}

func newSetPeerBandwidthCs(size uint32) chunkStream {
	ret := initControlMsg(idSetPeerBandwidth, 5, size)
	ret.data[4] = 2
	return ret
}

func userControlMsg(eventType, bufLen uint32) chunkStream {
	bufLen += 2
	ret := chunkStream{
		fmt:      0,
		csid:     2,
		typeId:   4,
		streamId: 1,
		length:   bufLen,
		data:     make([]byte, bufLen),
	}
	ret.data[0] = byte(eventType >> 8 & 0xff)
	ret.data[1] = byte(eventType & 0xff)
	return ret
}
