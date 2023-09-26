package p2p

import (
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/ws"
	"nhooyr.io/websocket"
)

/*
websocket service来做信令交换
*/
const (
	CandidateType = "candidate"
	OfferType     = "offer"
	AnswerType    = "answer"
	ActionType    = "action"
)

type WsMsg struct {
	MsgType string `json:"msgType"`
	Content string `json:"content"`
}

func (m *WsMsg) IsCandidateMsg() bool {
	return m.MsgType == CandidateType
}

func (m *WsMsg) IsOfferType() bool {
	return m.MsgType == OfferType
}

func (m *WsMsg) IsAnswerType() bool {
	return m.MsgType == AnswerType
}

func (m *WsMsg) String() string {
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

type service struct {
	member *Member
}

func NewSignalService() ws.Service {
	return &service{}
}

func (s *service) OnOpen(session *ws.Session) {
	member, err := authenticateAndInit(session)
	if err != nil {
		session.Close(websocket.StatusBadGateway, "authentication failed")
		return
	}
	s.member = member
	if member.ShouldOffer() {
		member.AskToSendOffer()
	} else {
		member.NotifyToRecvAnswer()
	}
}

func (s *service) OnTextMessage(_ *ws.Session, text string) {
	var msg WsMsg
	err := json.Unmarshal([]byte(text), &msg)
	if err != nil {
		return
	}
	member := s.member
	if msg.IsOfferType() {
		otherMember := member.GetOtherMember()
		if otherMember != nil {
			otherMember.SendOffer(msg.Content)
		}
	} else if msg.IsCandidateMsg() {
		otherMember := member.GetOtherMember()
		if otherMember != nil {
			otherMember.SendCandidate(msg.Content)
		}
	} else if msg.IsAnswerType() {
		otherMember := member.GetOtherMember()
		if otherMember != nil {
			otherMember.SendAnswer(msg.Content)
		}
	}
}

func (*service) OnBinaryMessage(*ws.Session, []byte) {}

func (s *service) OnClose(*ws.Session) {
	if s.member != nil {
		s.member.Room().DelMember(s.member)
	}
}

func authenticateAndInit(session *ws.Session) (*Member, error) {
	query := session.Request().URL.Query()
	roomId := query.Get("room")
	if roomId == "" {
		return nil, errors.New("invalid arguments")
	}
	userId := query.Get("user")
	if userId == "" {
		return nil, errors.New("invalid arguments")
	}
	room := GetOrNewRoom(roomId)
	if room.MemberSize() >= 2 {
		return nil, errors.New("room member size eq or greater than 2")
	}
	member := NewMember(userId, session)
	member.SetRoom(room)
	if err := room.AddMember(member); err != nil {
		return nil, err
	}
	return member, nil
}
