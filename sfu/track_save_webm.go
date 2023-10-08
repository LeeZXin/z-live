package sfu

import (
	"fmt"
	"github.com/LeeZXin/z-live/webm"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"golang.org/x/net/context"
	"net/http"
	"sync"
	"time"
)

// SaveToWebmTrackService 保存音视频数据到webm
type SaveToWebmTrackService struct {
	saver    *webm.Saver
	conn     *webrtc.PeerConnection
	ctx      context.Context
	cancelFn context.CancelFunc
	rtcpOnce sync.Once
}

func NewSaveToWebmTrackService() RTPService {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &SaveToWebmTrackService{
		ctx:      ctx,
		cancelFn: cancelFunc,
		rtcpOnce: sync.Once{},
	}
}

func (s *SaveToWebmTrackService) IsMediaRecvService() bool {
	return true
}

func (s *SaveToWebmTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *SaveToWebmTrackService) AuthenticateAndInit(*http.Request) error {
	fileName := fmt.Sprintf("%d.webm", time.Now().UnixMicro())
	s.saver = webm.NewSaver(fileName)
	return nil
}

func (s *SaveToWebmTrackService) OnDataChannel(*webrtc.DataChannel) {}

func (s *SaveToWebmTrackService) OnTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	s.rtcpOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.conn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				case <-s.ctx.Done():
					return
				}
			}
		}()
	})
	for {
		rtp, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		switch track.Kind() {
		case webrtc.RTPCodecTypeAudio:
			s.saver.PushOpus(rtp)
		case webrtc.RTPCodecTypeVideo:
			s.saver.PushVP8(rtp)
		}
	}
}

func (s *SaveToWebmTrackService) OnClose() {
	if s.saver != nil {
		s.saver.Close()
		s.cancelFn()
	}
}
