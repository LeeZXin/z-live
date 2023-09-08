package parser

import (
	"fmt"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/z-live/parser/aac"
	"github.com/LeeZXin/z-live/parser/h264"
	"github.com/LeeZXin/z-live/parser/mp3"
	"io"
)

var (
	errNoAudio = fmt.Errorf("demuxer no audio")
)

type CodecParser struct {
	aac  *aac.Parser
	mp3  *mp3.Parser
	h264 *h264.Parser
	w    io.Writer
}

func NewCodecParser(writer io.Writer) *CodecParser {
	return &CodecParser{
		w: writer,
	}
}

func (c *CodecParser) SampleRate() (int, error) {
	if c.aac == nil && c.mp3 == nil {
		return 0, errNoAudio
	}
	if c.aac != nil {
		return c.aac.SampleRate(), nil
	}
	return c.mp3.SampleRate(), nil
}

func (c *CodecParser) Parse(p *av.Packet) error {
	if p.IsVideo {
		f, ok := p.Header.(av.VideoPacketHeader)
		if ok {
			if f.CodecID() == av.VIDEO_H264 {
				if c.h264 == nil {
					c.h264 = h264.NewParser(c.w)
				}
				return c.h264.Parse(p.Data, f.IsSeq())
			}
		}
	} else {
		f, ok := p.Header.(av.AudioPacketHeader)
		if ok {
			switch f.SoundFormat() {
			case av.SOUND_AAC:
				if c.aac == nil {
					c.aac = aac.NewParser(c.w)
				}
				return c.aac.Parse(p.Data, f.AACPacketType())
			case av.SOUND_MP3:
				if c.mp3 == nil {
					c.mp3 = mp3.NewParser()
				}
				return c.mp3.Parse(p.Data)
			}
		}
	}
	return nil
}
