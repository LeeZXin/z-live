package sfu

import (
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"net/http"
	"strings"
)

type SaveToDiskTrackService struct {
	oggFile        *oggwriter.OggWriter
	ivfFile        *ivfwriter.IVFWriter
	authenticateFn func(*http.Request) error
}

func NewSaveToDiskTrackService(oggFileName, ivfFileName string, authenticateFn func(*http.Request) error) RTPService {
	ret := &SaveToDiskTrackService{
		authenticateFn: authenticateFn,
	}
	oggFile, err := oggwriter.New(oggFileName, 48000, 2)
	if err == nil {
		ret.oggFile = oggFile
	}
	ivfFile, err := ivfwriter.New(ivfFileName)
	if err == nil {
		ret.ivfFile = ivfFile
	}
	return ret
}

func (s *SaveToDiskTrackService) AuthenticateAndInit(request *http.Request) error {
	if s.authenticateFn != nil {
		return s.authenticateFn(request)
	}
	return nil
}

func (s *SaveToDiskTrackService) OnDataChannel(*webrtc.DataChannel) {

}

func (s *SaveToDiskTrackService) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
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
	defer func() {
		if err := i.Close(); err != nil {
			panic(err)
		}
	}()

	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if err = i.WriteRTP(rtpPacket); err != nil {
			return
		}
	}
}

func (s *SaveToDiskTrackService) OnClose() {
	if s.oggFile != nil {
		s.oggFile.Close()
	}
	if s.ivfFile != nil {
		s.ivfFile.Close()
	}
}

type RoomForwardTrackService struct {
}

func (s *RoomForwardTrackService) AuthenticateAndInit(*http.Request) error {
	return nil
}

func (s *RoomForwardTrackService) OnDataChannel(*webrtc.DataChannel) {

}

func (s *RoomForwardTrackService) OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver) {

}

func (s *RoomForwardTrackService) OnClose() {

}
