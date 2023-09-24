package sfu

import (
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/ws"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

/*
websocket service来做信令交换
*/
const (
	CandidateType = "candidate"
	OfferType     = "offer"
)

type WsMsg struct {
	MsgType string `json:"msgType"`
	Content string `json:"content"`
}

func (m *WsMsg) IsCandidateMsg() bool {
	return m.MsgType == CandidateType
}

func (m *WsMsg) GetCandidate() (webrtc.ICECandidateInit, error) {
	var ret webrtc.ICECandidateInit
	err := json.Unmarshal([]byte(m.Content), &ret)
	if err != nil {
		return ret, err
	}
	if ret.Candidate == "" {
		return ret, errors.New("invalid msg")
	}
	return ret, nil
}

func (m *WsMsg) IsOfferType() bool {
	return m.MsgType == OfferType
}

func (m *WsMsg) GetOffer() (webrtc.SessionDescription, error) {
	var ret webrtc.SessionDescription
	err := json.Unmarshal([]byte(m.Content), &ret)
	if err != nil {
		return ret, err
	}
	if ret.SDP == "" {
		return ret, errors.New("invalid msg")
	}
	return ret, nil
}

type service struct {
	conn       *webrtc.PeerConnection
	rtpService RTPService
}

func NewSignalService(rtpService RTPService) ws.Service {
	return &service{
		rtpService: rtpService,
	}
}

func (s *service) OnOpen(session *ws.Session) {
	if s.rtpService == nil {
		session.Close(websocket.StatusAbnormalClosure, "sys error")
		return
	}
	if err := s.rtpService.AuthenticateAndInit(session.Request()); err != nil {
		session.Close(websocket.StatusBadGateway, "authentication failed")
		return
	}
	peerConnection, err := newPeerConnection(s.rtpService.IsMediaRecvService())
	if err != nil {
		logger.Logger.Error(err.Error())
		session.Close(websocket.StatusAbnormalClosure, "sys err")
		return
	}
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		outbound, oerr := json.Marshal(c.ToJSON())
		if oerr != nil {
			logger.Logger.Error(oerr.Error())
			return
		}
		if err = session.WriteTextMessage(string(outbound)); err != nil {
			session.Close(websocket.StatusAbnormalClosure, "write err")
		}
	})
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			session.Close(websocket.StatusNormalClosure, "")
		case webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateFailed:
			session.Close(websocket.StatusNormalClosure, "")
			if s.rtpService != nil {
				s.rtpService.OnClose()
			}
		}
	})
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		switch state {
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateClosed:
			peerConnection.Close()
		}
	})
	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		s.rtpService.OnDataChannel(dc)
	})
	peerConnection.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		s.rtpService.OnTrack(remote, receiver)
	})
	s.rtpService.OnNewPeerConnection(peerConnection)
	s.conn = peerConnection
}
func (s *service) OnTextMessage(session *ws.Session, text string) {
	var msg WsMsg
	err := json.Unmarshal([]byte(text), &msg)
	if err != nil {
		return
	}
	if msg.IsOfferType() {
		offer, err := msg.GetOffer()
		if err != nil {
			return
		}
		if err = s.conn.SetRemoteDescription(offer); err != nil {
			return
		}
		answer, err := s.conn.CreateAnswer(nil)
		if err != nil {
			return
		}
		if err = s.conn.SetLocalDescription(answer); err != nil {
			return
		}
		outbound, err := json.Marshal(answer)
		if err != nil {
			return
		}
		if err = session.WriteTextMessage(string(outbound)); err != nil {
			session.Close(websocket.StatusAbnormalClosure, "write err")
		}
	} else if msg.IsCandidateMsg() {
		candidate, err := msg.GetCandidate()
		if err != nil {
			return
		}
		if err = s.conn.AddICECandidate(candidate); err != nil {
			return
		}
	}
}

func (*service) OnBinaryMessage(*ws.Session, []byte) {}

func (s *service) OnClose(*ws.Session) {
	if s.conn != nil {
		state := s.conn.ConnectionState()
		if state != webrtc.PeerConnectionStateConnected {
			s.conn.Close()
		}
	}
}
