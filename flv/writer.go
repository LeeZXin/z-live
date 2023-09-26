package flv

import (
	"context"
	"github.com/LeeZXin/z-live/amf"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/z-live/util/bytesutil"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/threadutil"
	"io"
	"net/http"
	"os"
	"sync"
)

var (
	flvHeader = []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

const (
	fileMode = iota + 1
	httpMode
)

// Writer 实现rtmp保存到本地flv文件或者使用http-flv传输
type Writer struct {
	buf         []byte
	writer      io.WriteCloser
	packetQueue chan *av.Packet
	ctx         context.Context
	cancelFn    context.CancelFunc
	mode        int
	closeOnce   sync.Once
}

func NewFileWriter(fileName string) (*Writer, error) {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		return nil, err
	}
	return newWriter(file, fileMode)
}

type httpWriterWrapper struct {
	writer http.ResponseWriter
}

func (w *httpWriterWrapper) Write(content []byte) (int, error) {
	n, err := w.writer.Write(content)
	if err == nil {
		w.flush()
	}
	return n, err
}

func (w *httpWriterWrapper) flush() {
	w.writer.(http.Flusher).Flush()
}

func (w *httpWriterWrapper) Close() error {
	return nil
}

func NewHttpWriter(writer http.ResponseWriter) (*Writer, error) {
	return newWriter(&httpWriterWrapper{
		writer: writer,
	}, httpMode)
}

func newWriter(writer io.WriteCloser, mode int) (*Writer, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ret := &Writer{
		writer:      writer,
		packetQueue: make(chan *av.Packet, maxQueueNum),
		buf:         make([]byte, headerLen),
		ctx:         ctx,
		cancelFn:    cancelFunc,
		closeOnce:   sync.Once{},
		mode:        mode,
	}
	_, err := ret.writer.Write(flvHeader)
	if err != nil {
		return nil, err
	}
	bytesutil.PutI32BE(ret.buf[:4], 0)
	_, err = ret.writer.Write(ret.buf[:4])
	if err != nil {
		return nil, err
	}
	quit.AddShutdownHook(func() {
		ret.Close()
	})
	go ret.muxPacket()
	return ret, nil
}

func (w *Writer) WritePacket(p *av.Packet) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	return threadutil.RunSafe(func() {
		// 文件模式直接存储 不丢包
		if w.mode == fileMode {
			w.packetQueue <- p.Copy()
			return
		}
		if p.IsAudio {
			w.packetQueue <- p.Copy()
			return
		}
		if p.IsVideo {
			videoPkt, ok := p.Header.(av.VideoPacketHeader)
			if ok {
				if videoPkt.IsSeq() || videoPkt.IsKeyFrame() {
					w.packetQueue <- p.Copy()
					return
				}
			}
		}
		select {
		case w.packetQueue <- p.Copy():
		default:
		}
	})
}

func (w *Writer) muxPacket() {
	defer w.Close()
	for {
		select {
		case p, ok := <-w.packetQueue:
			if !ok {
				return
			}
			h := w.buf[:headerLen]
			typeID := av.TAG_VIDEO
			if !p.IsVideo {
				if p.IsMetadata {
					var err error
					typeID = av.TAG_SCRIPTDATAAMF0
					p.Data, err = amf.MetaDataReform(p.Data, amf.DEL)
					if err != nil {
						return
					}
				} else {
					typeID = av.TAG_AUDIO
				}
			}
			dataLen := len(p.Data)
			timestamp := p.Timestamp
			preDataLen := dataLen + headerLen
			timestampBase := timestamp & 0xffffff
			timestampExt := timestamp >> 24 & 0xff
			bytesutil.PutU8(h[0:1], uint8(typeID))
			bytesutil.PutI24BE(h[1:4], int32(dataLen))
			bytesutil.PutI24BE(h[4:7], int32(timestampBase))
			bytesutil.PutU8(h[7:8], uint8(timestampExt))
			if _, err := w.writer.Write(h); err != nil {
				return
			}
			if _, err := w.writer.Write(p.Data); err != nil {
				return
			}
			bytesutil.PutI32BE(h[:4], int32(preDataLen))
			if _, err := w.writer.Write(h[:4]); err != nil {
				return
			}
		}
	}
}

func (w *Writer) Close() {
	w.closeOnce.Do(func() {
		w.cancelFn()
		close(w.packetQueue)
		_ = w.writer.Close()
	})
}

func (w *Writer) Wait() {
	for {
		select {
		case <-w.ctx.Done():
			return
		}
	}
}
