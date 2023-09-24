package sfu

import (
	"fmt"
	"github.com/LeeZXin/z-live/webm"
	"github.com/LeeZXin/zsf/logger"
	"github.com/pion/webrtc/v4"
	"net/http"
	"time"
)

// SaveToWebmTrackService 保存音视频数据到webm
type SaveToWebmTrackService struct {
	saver *webm.Saver
	conn  *webrtc.PeerConnection
}

func NewSaveToWebmTrackService() RTPService {
	return &SaveToWebmTrackService{}
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
	for {
		rtp, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		switch track.Kind() {
		case webrtc.RTPCodecTypeAudio:
			if err = s.saver.PushOpus(rtp); err != nil {
				logger.Logger.Error(err.Error())
			}
		case webrtc.RTPCodecTypeVideo:
			if err = s.saver.PushVP8(rtp); err != nil {
				logger.Logger.Error(err.Error())
			}
		}
	}
}

func (s *SaveToWebmTrackService) OnClose() {
	if s.saver != nil {
		s.saver.Close()
	}
}
