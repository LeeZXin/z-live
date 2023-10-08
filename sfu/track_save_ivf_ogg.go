package sfu

import (
	"fmt"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"net/http"
	"strings"
	"time"
)

// SaveIvfOggTrackService 保存音视频数据到ogg，ivf中
type SaveIvfOggTrackService struct {
	oggFile *oggwriter.OggWriter
	ivfFile *ivfwriter.IVFWriter
	conn    *webrtc.PeerConnection
}

func NewSaveIvfOggTrackService() RTPService {
	return &SaveIvfOggTrackService{}
}

func (s *SaveIvfOggTrackService) IsMediaRecvService() bool {
	return true
}

func (s *SaveIvfOggTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *SaveIvfOggTrackService) AuthenticateAndInit(request *http.Request) error {
	now := time.Now().UnixMicro()
	oggFileName := fmt.Sprintf("%d.ogg", now)
	oggFile, err := oggwriter.New(oggFileName, 48000, 2)
	if err == nil {
		s.oggFile = oggFile
	}
	ivfFileName := fmt.Sprintf("%d.ivf", now)
	ivfFile, err := ivfwriter.New(ivfFileName)
	if err == nil {
		s.ivfFile = ivfFile
	}
	return nil
}

func (s *SaveIvfOggTrackService) OnDataChannel(*webrtc.DataChannel) {

}

func (s *SaveIvfOggTrackService) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	codec := track.Codec()
	if strings.EqualFold(codec.MimeType, webrtc.MimeTypeOpus) {
		if s.oggFile != nil {
			saveToDisk(s.oggFile, track)
		}
	} else if strings.EqualFold(codec.MimeType, webrtc.MimeTypeVP8) {
		if s.ivfFile != nil {
			saveToDisk(s.ivfFile, track)
		}
	}
}

func saveToDisk(i media.Writer, track *webrtc.TrackRemote) {
	defer i.Close()
	for {
		p, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if err = i.WriteRTP(p); err != nil {
			return
		}
	}
}

func (s *SaveIvfOggTrackService) OnClose() {
	if s.oggFile != nil {
		s.oggFile.Close()
	}
	if s.ivfFile != nil {
		s.ivfFile.Close()
	}
}
