package sfu

import (
	"errors"
	"fmt"
	"github.com/LeeZXin/z-live/webm"
	"github.com/LeeZXin/zsf/util/strutil"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"net/http"
	"strings"
	"time"
)

type SaveToDiskTrackService struct {
	oggFile *oggwriter.OggWriter
	ivfFile *ivfwriter.IVFWriter
	conn    *webrtc.PeerConnection
}

func NewSaveToDiskTrackService() RTPService {
	return &SaveToDiskTrackService{}
}

func (s *SaveToDiskTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *SaveToDiskTrackService) AuthenticateAndInit(request *http.Request) error {
	now := time.Now().UnixNano()
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
	defer i.Close()
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
	conn   *webrtc.PeerConnection
	roomId string
	member *Member
}

func (s *RoomForwardTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
	member := NewMember(conn)
	room := GetOrNewRoom(s.roomId)
	member.SetRoom(room)
	s.member = member
}

func (s *RoomForwardTrackService) AuthenticateAndInit(request *http.Request) error {
	roomId := request.URL.Query().Get("room")
	if roomId == "" {
		return errors.New("roomId empty")
	}
	s.roomId = roomId
	return nil
}

func (s *RoomForwardTrackService) OnDataChannel(*webrtc.DataChannel) {}

func (s *RoomForwardTrackService) OnTrack(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	trackUid := fmt.Sprintf("%s_%s%d", remote.Kind().String(), strutil.RandomStr(4), time.Now().UnixMicro())
	localTrack, err := webrtc.NewTrackLocalStaticRTP(remote.Codec().RTPCodecCapability, trackUid, trackUid+"_stream")
	if err != nil {
		s.conn.Close()
		return
	}
	switch remote.Kind() {
	case webrtc.RTPCodecTypeAudio:
		s.member.SetAudioTrack(localTrack)
	case webrtc.RTPCodecTypeVideo:
		s.member.SetVideoTrack(localTrack)
	}
	if s.member.AudioTrack() != nil && s.member.VideoTrack() != nil {
		s.member.Room().AddMember(s.member)
	}
	sendToTrack(remote, localTrack)
}

func sendToTrack(remote *webrtc.TrackRemote, track *webrtc.TrackLocalStaticRTP) {
	for {
		rtpPacket, _, err := remote.ReadRTP()
		if err != nil {
			return
		}
		if err = track.WriteRTP(rtpPacket); err != nil {
			return
		}
	}
}

func (s *RoomForwardTrackService) OnClose() {
	if s.member != nil {
		room := s.member.Room()
		if room != nil {
			room.DelMember(s.member)
		}
	}
}

type SaveToWebmTrackService struct {
	saver *webm.Saver
	conn  *webrtc.PeerConnection
}

func NewSaveToWebmTrackService() RTPService {
	return &SaveToWebmTrackService{}
}

func (s *SaveToWebmTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *SaveToWebmTrackService) AuthenticateAndInit(*http.Request) error {
	fileName := fmt.Sprintf("%d.webm", time.Now().UnixNano())
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
			_ = s.saver.PushOpus(rtp)
		case webrtc.RTPCodecTypeVideo:
			_ = s.saver.PushVP8(rtp)
		}
	}
}

func (s *SaveToWebmTrackService) OnClose() {
	if s.saver != nil {
		s.saver.Close()
	}
}
