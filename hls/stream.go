package hls

import (
	"bytes"
	"context"
	"fmt"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/z-live/flv"
	"github.com/LeeZXin/z-live/hls/ts"
	"github.com/LeeZXin/z-live/parser"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/property/static"
	"sync"
)

const (
	videoHZ              = 90000
	aacSampleLen         = 1024
	maxQueueNum          = 512
	h264DefaultHz uint64 = 90
	duration             = 3000
)

var (
	SaveFileFlag bool
)

func init() {
	SaveFileFlag = static.GetBool("hls.saveFile")
}

var (
	ErrNoPublisher         = fmt.Errorf("no publisher")
	ErrInvalidReq          = fmt.Errorf("invalid req url path")
	ErrNoSupportVideoCodec = fmt.Errorf("no support video codec")
	ErrNoSupportAudioCodec = fmt.Errorf("no support audio codec")
)

const (
	fileMode = iota + 1
	liveMode
)

// StreamWriter 实现hls m3u8 ts转换
type StreamWriter struct {
	name        string
	seq         int
	bwriter     *bytes.Buffer
	btswriter   *bytes.Buffer
	muxer       *ts.Muxer
	pts, dts    uint64
	stat        *status
	align       *align
	audioCache  *audioCache
	tsCache     *TsCache
	tsParser    *parser.CodecParser
	packetQueue chan *av.Packet
	ctx         context.Context
	cancelFn    context.CancelFunc
	closeOnce   sync.Once

	firstCut bool
}

func NewStreamWriter(app, name string) *StreamWriter {
	ctx, cancelFunc := context.WithCancel(context.Background())
	bwriter := bytes.NewBuffer(make([]byte, 100*1024))
	btswriter := bytes.NewBuffer(nil)
	w := &StreamWriter{
		name:        app + "/" + name,
		align:       &align{},
		stat:        newStatus(),
		audioCache:  newAudioCache(),
		muxer:       ts.NewMuxer(btswriter),
		tsCache:     NewTsCache(app, name),
		tsParser:    parser.NewCodecParser(bwriter),
		btswriter:   btswriter,
		bwriter:     bwriter,
		packetQueue: make(chan *av.Packet, maxQueueNum),
		ctx:         ctx,
		cancelFn:    cancelFunc,
		closeOnce:   sync.Once{},
	}
	registerStreamWriter(w)
	quit.AddShutdownHook(func() {
		w.Close()
	})
	go w.muxPacket()
	return w
}

func (w *StreamWriter) GetM3u8Body() []byte {
	return w.tsCache.GenM3U8PlayList()
}

func (w *StreamWriter) GetTsBody(ts string) []byte {
	item, err := w.tsCache.GetItem(ts)
	if err != nil {
		return []byte{}
	}
	return item
}

func (w *StreamWriter) WritePacket(p *av.Packet) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	return threadutil.RunSafe(func() {
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

func (w *StreamWriter) muxPacket() {
	defer func() {
		if w.firstCut {
			w.flush2Cache()
		}
		w.Close()
	}()
	for {
		select {
		case p, ok := <-w.packetQueue:
			if !ok {
				return
			}
			if p.IsMetadata {
				continue
			}
			err := flv.Demux(p)
			if err == flv.ErrAvcEndSEQ {
				continue
			}
			if err != nil {
				return
			}
			compositionTime, isSeq, err := w.parse(p)
			if err != nil || isSeq {
				continue
			}
			w.stat.update(p.Timestamp)
			w.calcPtsDts(p.IsVideo, p.Timestamp, uint32(compositionTime))
			w.tsMux(p)
		}
	}
}

func (w *StreamWriter) Close() {
	w.closeOnce.Do(func() {
		w.cancelFn()
		deregisterStreamWriter(w.name)
		close(w.packetQueue)
	})
}

func (w *StreamWriter) cut() {
	if !w.firstCut {
		w.firstCut = true
		w.muxer.WritePAT()
		w.muxer.WritePMT(av.SOUND_AAC, true)
	} else if w.stat.durationMs() >= duration {
		w.flush2Cache()
	}
}

func (w *StreamWriter) flush2Cache() {
	w.flushAudio()
	w.seq++
	w.tsCache.SetItem(int(w.stat.durationMs()), w.seq, w.btswriter.Bytes())
	w.btswriter.Reset()
	w.stat.resetAndNew()
	w.muxer.WritePAT()
	w.muxer.WritePMT(av.SOUND_AAC, true)
}

func (w *StreamWriter) parse(p *av.Packet) (int32, bool, error) {
	var compositionTime int32
	var ah av.AudioPacketHeader
	var vh av.VideoPacketHeader
	if p.IsVideo {
		vh = p.Header.(av.VideoPacketHeader)
		if vh.CodecID() != av.VIDEO_H264 {
			return compositionTime, false, ErrNoSupportVideoCodec
		}
		compositionTime = vh.CompositionTime()
		if vh.IsKeyFrame() && vh.IsSeq() {
			return compositionTime, true, w.tsParser.Parse(p)
		}
	} else {
		ah = p.Header.(av.AudioPacketHeader)
		if ah.SoundFormat() != av.SOUND_AAC {
			return compositionTime, false, ErrNoSupportAudioCodec
		}
		if ah.AACPacketType() == av.AAC_SEQHDR {
			return compositionTime, true, w.tsParser.Parse(p)
		}
	}
	w.bwriter.Reset()
	if err := w.tsParser.Parse(p); err != nil {
		return compositionTime, false, err
	}
	p.Data = w.bwriter.Bytes()
	if p.IsVideo && vh.IsKeyFrame() {
		w.cut()
	}
	return compositionTime, false, nil
}

func (w *StreamWriter) calcPtsDts(isVideo bool, ts, compositionTs uint32) {
	w.dts = uint64(ts) * h264DefaultHz
	if isVideo {
		w.pts = w.dts + uint64(compositionTs)*h264DefaultHz
	} else {
		sampleRate, _ := w.tsParser.SampleRate()
		w.align.align(&w.dts, uint32(videoHZ*aacSampleLen/sampleRate))
		w.pts = w.dts
	}
}

func (w *StreamWriter) flushAudio() error {
	return w.muxAudio(1)
}

func (w *StreamWriter) muxAudio(limit byte) error {
	if w.audioCache.cacheNum() < limit {
		return nil
	}
	var p av.Packet
	_, pts, buf := w.audioCache.getFrame()
	p.Data = buf
	p.Timestamp = uint32(pts / h264DefaultHz)
	return w.muxer.WritePacket(&p)
}

func (w *StreamWriter) tsMux(p *av.Packet) error {
	if p.IsVideo {
		return w.muxer.WritePacket(p)
	}
	w.audioCache.cache(p.Data, w.pts)
	return w.muxAudio(cacheMaxFrames)
}
