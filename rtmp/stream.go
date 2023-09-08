package rtmp

import (
	"context"
	"errors"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/z-live/flv"
	"github.com/LeeZXin/zsf/executor"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/threadutil"
	"io"
	"sync"
)

const (
	maxQueueNum = 1024
)

type streamWriter struct {
	registerIndex int64
	conn          *netConn
	packetQueue   chan *av.Packet
	lastTimestamp uint32

	ctx      context.Context
	cancelFn context.CancelFunc

	closeOnce sync.Once
}

func newStreamWriter(conn *netConn) *streamWriter {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &streamWriter{
		conn:        conn,
		packetQueue: make(chan *av.Packet, maxQueueNum),
		ctx:         ctx,
		cancelFn:    cancelFunc,
		closeOnce:   sync.Once{},
	}
}

func (v *streamWriter) Start() {
	quit.AddShutdownHook(func() {
		v.Close()
	})
	go v.ReadPacket()
	v.SendPacket()
}

func (v *streamWriter) ReadPacket() {
	for {
		if v.ctx.Err() != nil {
			return
		}
		if err := readAndHandleUserCtrlMsg(v.conn); err != nil {
			return
		}
	}
}

func (v *streamWriter) WritePacket(p *av.Packet) error {
	if err := v.ctx.Err(); err != nil {
		return err
	}
	p = p.Copy()
	return threadutil.RunSafe(func() {
		if p.IsAudio {
			v.packetQueue <- p
		}
		if p.IsVideo {
			videoPkt, ok := p.Header.(av.VideoPacketHeader)
			if ok {
				if videoPkt.IsSeq() || videoPkt.IsKeyFrame() {
					v.packetQueue <- p
				}
			}
		}
		select {
		case v.packetQueue <- p:
		default:
		}
	})
}

func (v *streamWriter) SendPacket() {
	var cs chunkStream
	for {
		select {
		case p, ok := <-v.packetQueue:
			if !ok {
				return
			}
			cs.data = p.Data
			cs.length = uint32(len(p.Data))
			cs.streamId = p.StreamId
			cs.timestamp = p.Timestamp + v.lastTimestamp
			v.lastTimestamp = cs.timestamp
			if p.IsVideo {
				cs.typeId = av.TAG_VIDEO
			} else {
				if p.IsMetadata {
					cs.typeId = av.TAG_SCRIPTDATAAMF0
				} else {
					cs.typeId = av.TAG_AUDIO
				}
			}
			if err := cs.writeChunk(v.conn); err != nil {
				return
			}
			if err := v.conn.Flush(); err != nil {
				return
			}
		}
	}
}

func (v *streamWriter) Close() {
	v.closeOnce.Do(func() {
		v.cancelFn()
		close(v.packetQueue)
		v.conn.Close()
	})
}

type RegisterAction interface {
	Register(PacketWriter)
}

type streamPublisher struct {
	conn           *netConn
	cache          *streamCache
	registry       *writerRegistryHolder
	writeExecutors *executor.Executor

	sync.Mutex
	closed bool
}

func newStreamPublisher(conn *netConn) *streamPublisher {
	return &streamPublisher{
		registry: newWriterRegistryHolder(),
		conn:     conn,
		cache:    newStreamCache(),
		Mutex:    sync.Mutex{},
	}
}

func (v *streamPublisher) Register(writer PacketWriter) {
	if writer == nil {
		return
	}
	v.Lock()
	defer v.Unlock()
	if v.closed {
		return
	}
	v.registry.register(newPacketWriterWrapper(writer))
}

func (v *streamPublisher) start() {
	for {
		if v.isClosed() {
			return
		}
		var p av.Packet
		packet := &p
		err := v.read(packet)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			continue
		}
		v.cache.add(packet)
		writers := v.registry.getMembers()
		for i, writer := range writers {
			if err = writer.writeCache(v.cache); err != nil {
				writer.Close()
				v.registry.deregister(i)
				return
			}
			if err = writer.WritePacket(packet); err != nil {
				writer.Close()
				v.registry.deregister(i)
			}
		}
	}
}

func (v *streamPublisher) isClosed() bool {
	v.Lock()
	defer v.Unlock()
	return v.closed
}

func (v *streamPublisher) read(p *av.Packet) error {
	for {
		if err := readAndHandleUserCtrlMsg(v.conn); err != nil {
			return err
		}
		cs := v.conn.cs
		if cs.typeId == av.TAG_AUDIO ||
			cs.typeId == av.TAG_VIDEO ||
			cs.typeId == av.TAG_SCRIPTDATAAMF0 ||
			cs.typeId == av.TAG_SCRIPTDATAAMF3 {
			break
		}
	}
	cs := v.conn.cs
	p.IsAudio = cs.typeId == av.TAG_AUDIO
	p.IsVideo = cs.typeId == av.TAG_VIDEO
	p.IsMetadata = cs.typeId == av.TAG_SCRIPTDATAAMF0 || cs.typeId == av.TAG_SCRIPTDATAAMF3
	p.StreamId = cs.streamId
	p.Data = cs.data
	p.Timestamp = cs.timestamp
	if err := flv.DemuxH(p); err != nil {
		return err
	}
	return nil
}

func (v *streamPublisher) close() {
	v.Lock()
	defer v.Unlock()
	v.closed = true
	v.conn.Close()
	v.registry.closeAll()
}

type streamCache struct {
	gop      *gopRingQueue
	videoSeq *av.Packet
	audioSeq *av.Packet
	metadata *av.Packet
}

func newStreamCache() *streamCache {
	return &streamCache{
		gop: newGopRingQueue(10),
	}
}

func (c *streamCache) send(writer PacketWriter) error {
	if c.metadata != nil {
		if err := writer.WritePacket(c.metadata); err != nil {
			return err
		}
	}
	if c.videoSeq != nil {
		if err := writer.WritePacket(c.videoSeq); err != nil {
			return err
		}
	}
	if c.audioSeq != nil {
		if err := writer.WritePacket(c.audioSeq); err != nil {
			return err
		}
	}
	return c.gop.send(writer)
}

func (c *streamCache) add(p *av.Packet) {
	if p == nil {
		return
	}
	if p.IsMetadata {
		c.metadata = p
		return
	}
	if p.IsVideo {
		vh, ok := p.Header.(av.VideoPacketHeader)
		if !ok {
			return
		}
		if vh.IsSeq() {
			c.videoSeq = p
			return
		}
	} else {
		ah, ok := p.Header.(av.AudioPacketHeader)
		if ok {
			if ah.SoundFormat() == av.SOUND_AAC &&
				ah.AACPacketType() == av.AAC_SEQHDR {
				c.audioSeq = p
			}
			return
		}
	}
	c.gop.add(p)
}
