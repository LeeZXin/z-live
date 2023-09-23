package sfu

import (
	"errors"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"net/http"
)

type JoinRoomTrackService struct {
	conn   *webrtc.PeerConnection
	roomId string
	userId string
	member *Member
}

func NewJoinTrackService() RTPService {
	return &JoinRoomTrackService{}
}

func (s *JoinRoomTrackService) IsMediaRecvService() bool {
	return true
}

func (s *JoinRoomTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
	member := NewMember(conn, s.userId)
	room := GetOrNewRoom(s.roomId)
	member.SetRoom(room)
	s.member = member
}

func (s *JoinRoomTrackService) AuthenticateAndInit(request *http.Request) error {
	roomId := request.URL.Query().Get("room")
	if roomId == "" {
		return errors.New("roomId empty")
	}
	userId := request.URL.Query().Get("user")
	if userId == "" {
		return errors.New("userId empty")
	}
	s.roomId = roomId
	s.userId = userId
	return nil
}

func (s *JoinRoomTrackService) OnDataChannel(*webrtc.DataChannel) {}

func (s *JoinRoomTrackService) OnTrack(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	uid := uuid.NewString()
	localTrack, err := webrtc.NewTrackLocalStaticRTP(remote.Codec().RTPCodecCapability, uid, uid+"_stream")
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

func (s *JoinRoomTrackService) OnClose() {
	if s.member != nil {
		room := s.member.Room()
		if room != nil {
			room.DelMember(s.member)
		}
	}
}

type RoomForwardTrackService struct {
	conn         *webrtc.PeerConnection
	targetMember *Member
}

func NewRoomForwardTrackService() RTPService {
	return &RoomForwardTrackService{}
}

func (s *RoomForwardTrackService) IsMediaRecvService() bool {
	return false
}

func (s *RoomForwardTrackService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
	var err error
	if _, err = conn.AddTrack(s.targetMember.AudioTrack()); err != nil {
		conn.Close()
		return
	}
	if _, err = conn.AddTrack(s.targetMember.VideoTrack()); err != nil {
		conn.Close()
		return
	}
}

func (s *RoomForwardTrackService) AuthenticateAndInit(request *http.Request) error {
	roomId := request.URL.Query().Get("room")
	if roomId == "" {
		return errors.New("roomId empty")
	}
	userId := request.URL.Query().Get("user")
	if userId == "" {
		return errors.New("userId empty")
	}
	room := GetRoom(roomId)
	if room == nil {
		return errors.New("invalid room")
	}
	member := room.GetMember(userId)
	if member == nil {
		return errors.New("invalid member")
	}
	s.targetMember = member
	return nil
}

func (s *RoomForwardTrackService) OnDataChannel(*webrtc.DataChannel) {}

func (s *RoomForwardTrackService) OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver) {}

func (s *RoomForwardTrackService) OnClose() {}
