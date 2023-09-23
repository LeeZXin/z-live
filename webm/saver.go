package webm

import (
	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"os"
	"time"
)

type Saver struct {
	audioWriter, videoWriter       webm.BlockWriteCloser
	audioBuilder, videoBuilder     *samplebuilder.SampleBuilder
	audioTimestamp, videoTimestamp time.Duration
	fileName                       string
}

func NewSaver(fileName string) *Saver {
	return &Saver{
		fileName:     fileName,
		audioBuilder: samplebuilder.New(10, &codecs.OpusPacket{}, 48000),
		videoBuilder: samplebuilder.New(10, &codecs.VP8Packet{}, 90000),
	}
}

func (s *Saver) Close() {
	if s.audioWriter != nil {
		s.audioWriter.Close()
	}
	if s.videoWriter != nil {
		s.videoWriter.Close()
	}
}

func (s *Saver) PushOpus(rtpPacket *rtp.Packet) error {
	s.audioBuilder.Push(rtpPacket)
	for {
		sample := s.audioBuilder.Pop()
		if sample == nil {
			return nil
		}
		if s.audioWriter != nil {
			s.audioTimestamp += sample.Duration
			if _, err := s.audioWriter.Write(true, int64(s.audioTimestamp/time.Millisecond), sample.Data); err != nil {
				return err
			}
		}
	}
}

func (s *Saver) PushVP8(rtpPacket *rtp.Packet) error {
	s.videoBuilder.Push(rtpPacket)
	for {
		sample := s.videoBuilder.Pop()
		if sample == nil {
			return nil
		}
		videoKeyframe := sample.Data[0]&0x1 == 0
		if videoKeyframe {
			raw := uint(sample.Data[6]) | uint(sample.Data[7])<<8 | uint(sample.Data[8])<<16 | uint(sample.Data[9])<<24
			width := int(raw & 0x3FFF)
			height := int((raw >> 16) & 0x3FFF)
			if s.videoWriter == nil {
				if err := s.initWriter(width, height); err != nil {
					return err
				}
			}
		}
		if s.videoWriter != nil {
			s.videoTimestamp += sample.Duration
			if _, err := s.videoWriter.Write(videoKeyframe, int64(s.videoTimestamp/time.Millisecond), sample.Data); err != nil {
				return err
			}
		}
	}
}

func (s *Saver) initWriter(width, height int) error {
	w, err := os.OpenFile(s.fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	ws, err := webm.NewSimpleBlockWriter(w,
		[]webm.TrackEntry{
			{
				Name:            "Audio",
				TrackNumber:     1,
				TrackUID:        12345,
				CodecID:         "A_OPUS",
				TrackType:       2,
				DefaultDuration: 20000000,
				Audio: &webm.Audio{
					SamplingFrequency: 48000.0,
					Channels:          2,
				},
			}, {
				Name:            "Video",
				TrackNumber:     2,
				TrackUID:        67890,
				CodecID:         "V_VP8",
				TrackType:       1,
				DefaultDuration: 33333333,
				Video: &webm.Video{
					PixelWidth:  uint64(width),
					PixelHeight: uint64(height),
				},
			},
		})
	if err != nil {
		return err
	}
	s.audioWriter = ws[0]
	s.videoWriter = ws[1]
	return nil
}
