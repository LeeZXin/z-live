package sfu

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/google/uuid"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rtpPacket struct {
	mimeType string
	kind     webrtc.RTPCodecType
	packet   *rtp.Packet
}

const (
	saveToDiskFlag = true
)

// JoinRoomTrackService 多人音视频通话加入房间service
type JoinRoomTrackService struct {
	conn   *webrtc.PeerConnection
	roomId string
	userId string
	member *Member

	oggFile  *oggwriter.OggWriter
	ivfFile  *ivfwriter.IVFWriter
	rtcpOnce sync.Once

	ctx      context.Context
	cancelFn context.CancelFunc
	saveChan chan *rtpPacket
}

func NewJoinRoomTrackService() RTPService {
	ctx, cancelFunc := context.WithCancel(context.Background())
	ret := &JoinRoomTrackService{
		rtcpOnce: sync.Once{},
		ctx:      ctx,
		cancelFn: cancelFunc,
	}
	if saveToDiskFlag {
		ret.saveChan = make(chan *rtpPacket, 1024)
	}
	return ret
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
	if saveToDiskFlag {
		now := time.Now().UnixMicro()
		oggFileName := fmt.Sprintf("%s_%d.ogg", userId, now)
		oggFile, err := oggwriter.New(oggFileName, 48000, 2)
		if err != nil {
			return err
		}
		s.oggFile = oggFile
		ivfFileName := fmt.Sprintf("%s_%d.ivf", userId, now)
		ivfFile, err := ivfwriter.New(ivfFileName)
		if err != nil {
			return err
		}
		s.ivfFile = ivfFile
		go func() {
			for {
				select {
				case p, ok := <-s.saveChan:
					if !ok {
						return
					}
					if strings.EqualFold(p.mimeType, webrtc.MimeTypeOpus) {
						s.oggFile.WriteRTP(p.packet)
					} else if strings.EqualFold(p.mimeType, webrtc.MimeTypeVP8) {
						s.ivfFile.WriteRTP(p.packet)
					}
				}
			}
		}()
	}
	return nil
}

func (s *JoinRoomTrackService) OnDataChannel(*webrtc.DataChannel) {}

func (s *JoinRoomTrackService) OnTrack(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	if saveToDiskFlag {
		if webrtc.RTPCodecTypeVideo == remote.Kind() {
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						s.conn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())}})
					case <-s.ctx.Done():
						return
					}
				}
			}()
		}
	}
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
	s.sendToTrack(remote, localTrack)
}

func (s *JoinRoomTrackService) sendToTrack(remote *webrtc.TrackRemote, track *webrtc.TrackLocalStaticRTP) {
	for {
		p, _, err := remote.ReadRTP()
		if err != nil {
			return
		}
		if saveToDiskFlag {
			s.saveChan <- &rtpPacket{
				mimeType: remote.Codec().MimeType,
				kind:     remote.Kind(),
				packet:   p.Clone(),
			}
		}
		if err = track.WriteRTP(p); err != nil {
			logger.Logger.Error(err.Error())
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
	s.cancelFn()
	if s.saveChan != nil {
		close(s.saveChan)
	}
	if s.oggFile != nil {
		s.oggFile.Close()
	}
	if s.ivfFile != nil {
		s.ivfFile.Close()
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
	if !s.targetMember.AddListener(conn) {
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
