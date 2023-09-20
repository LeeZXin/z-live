package sfu

import (
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/hashset"
	"github.com/LeeZXin/zsf/util/taskutil"
	"github.com/pion/webrtc/v4"
	"sync"
	"sync/atomic"
	"time"
)

var (
	roomMap = make(map[string]*Room, 8)
	roomMu  = sync.RWMutex{}
)

func init() {
	task, _ := taskutil.NewPeriodicalTask(30*time.Second, func() {
		roomList := GetRoomList()
		for _, room := range roomList {
			CheckRoomMemberIsZero(room)
		}
	})
	task.Start()
	quit.AddShutdownHook(func() {
		task.Stop()
	})
}

type Room struct {
	id      string
	members hashset.HashSet[*Member]
	mu      sync.RWMutex
}

func (r *Room) Members() []*Member {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.members.ToSlice()
}

func (r *Room) MemberSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}

func (r *Room) addMember(member *Member) {
	if member == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members.Add(member)
}

func (r *Room) AddMember(member *Member) {
	r.addMember(member)
	r.shareTrackFromEachOthers(member)
}

func (r *Room) shareTrackFromEachOthers(member *Member) {
	if member == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for m := range r.members {
		if m == member {
			continue
		}
		member.AddOtherMemberTrack(m.AudioTrack())
		member.AddOtherMemberTrack(m.VideoTrack())
		m.AddOtherMemberTrack(member.AudioTrack())
		m.AddOtherMemberTrack(member.VideoTrack())
	}
}

func (r *Room) delMember(member *Member) {
	if member == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members.Delete(member)
}

func (r *Room) DelMember(member *Member) {
	r.delMember(member)
	r.removeTrackFromEachOthers(member)
}

func (r *Room) removeTrackFromEachOthers(member *Member) {
	if member == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for m := range r.members {
		if m == member {
			continue
		}
		m.RemoveOtherMemberTrack(m.AudioTrack())
		m.RemoveOtherMemberTrack(m.VideoTrack())
	}
}

type Member struct {
	mu         sync.RWMutex
	conn       *webrtc.PeerConnection
	audioTrack atomic.Value
	videoTrack atomic.Value
	room       atomic.Value

	trackMap map[*webrtc.TrackLocalStaticRTP]*webrtc.RTPSender
	trackMu  sync.RWMutex
}

func NewMember(conn *webrtc.PeerConnection) *Member {
	if conn == nil {
		return nil
	}
	return &Member{
		mu:       sync.RWMutex{},
		conn:     conn,
		trackMap: make(map[*webrtc.TrackLocalStaticRTP]*webrtc.RTPSender, 8),
		trackMu:  sync.RWMutex{},
	}
}

func (m *Member) SetRoom(room *Room) {
	if room == nil {
		return
	}
	m.room.Store(room)
}

func (m *Member) SetAudioTrack(track *webrtc.TrackLocalStaticRTP) {
	m.audioTrack.Store(track)
}

func (m *Member) SetVideoTrack(track *webrtc.TrackLocalStaticRTP) {
	m.videoTrack.Store(track)
}

func (m *Member) AudioTrack() *webrtc.TrackLocalStaticRTP {
	val := m.audioTrack.Load()
	if val != nil {
		return val.(*webrtc.TrackLocalStaticRTP)
	}
	return nil
}

func (m *Member) VideoTrack() *webrtc.TrackLocalStaticRTP {
	val := m.videoTrack.Load()
	if val != nil {
		return val.(*webrtc.TrackLocalStaticRTP)
	}
	return nil
}

func (m *Member) AddOtherMemberTrack(track *webrtc.TrackLocalStaticRTP) {
	if track == nil {
		return
	}
	rtpSender, err := m.conn.AddTrack(track)
	if err != nil {
		return
	}
	m.addOtherMemberTrack(track, rtpSender)
}

func (m *Member) addOtherMemberTrack(track *webrtc.TrackLocalStaticRTP, sender *webrtc.RTPSender) {
	m.trackMu.Lock()
	defer m.trackMu.Unlock()
	m.trackMap[track] = sender
}

func (m *Member) RemoveOtherMemberTrack(track *webrtc.TrackLocalStaticRTP) {
	m.trackMu.Lock()
	defer m.trackMu.Unlock()
	sender, ok := m.trackMap[track]
	if ok {
		_ = m.conn.RemoveTrack(sender)
	}
	delete(m.trackMap, track)
}

func (m *Member) Room() *Room {
	val := m.room.Load()
	if val != nil {
		return val.(*Room)
	}
	return nil
}

func GetOrNewRoom(id string) *Room {
	roomMu.Lock()
	defer roomMu.Unlock()
	room := roomMap[id]
	if room != nil {
		return room
	}
	room = &Room{
		id:      id,
		members: make(hashset.HashSet[*Member], 8),
		mu:      sync.RWMutex{},
	}
	roomMap[id] = room
	return room
}

func CheckRoomMemberIsZero(room *Room) {
	if room == nil {
		return
	}
	roomMu.Lock()
	defer roomMu.Unlock()
	if room.MemberSize() == 0 {
		delete(roomMap, room.id)
	}
}

func GetRoom(id string) *Room {
	roomMu.RLock()
	defer roomMu.RUnlock()
	return roomMap[id]
}

func GetRoomList() []*Room {
	roomMu.RLock()
	defer roomMu.RUnlock()
	ret := make([]*Room, 0, len(roomMap))
	for _, room := range roomMap {
		ret = append(ret, room)
	}
	return ret
}
