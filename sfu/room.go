package sfu

import (
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/taskutil"
	"github.com/pion/webrtc/v4"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// roomMap 存储房间
	roomMap = make(map[string]*Room, 8)
	// roomMu 房间锁
	roomMu = sync.RWMutex{}
)

func init() {
	// 定时清除房间人数为0的房间
	task, _ := taskutil.NewPeriodicalTask(30*time.Second, func() {
		roomList := GetRoomList()
		for _, room := range roomList {
			CheckRoomMemberIsZero(room)
		}
	})
	task.Start()
	// 程序退出时关闭定时任务
	quit.AddShutdownHook(func() {
		task.Stop()
	})
}

// Room 多人通讯房间
type Room struct {
	id      string
	members map[string]*Member
	mu      sync.RWMutex
}

// Members 获取房间成员列表
func (r *Room) Members() []*Member {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*Member, 0, len(r.members))
	for _, member := range r.members {
		ret = append(ret, member)
	}
	return ret
}

// GetMember 获取单个成员
func (r *Room) GetMember(userId string) *Member {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.members[userId]
}

// MemberSize 成员数量
func (r *Room) MemberSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}

// AddMember 添加成员
func (r *Room) AddMember(member *Member) {
	if member == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[member.UserId()] = member
}

// DelMember 删除成员
func (r *Room) DelMember(member *Member) {
	if member == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members, member.UserId())
}

// Member 成员
type Member struct {
	mu         sync.RWMutex
	conn       *webrtc.PeerConnection
	audioTrack atomic.Value
	videoTrack atomic.Value
	room       atomic.Value
	userId     string
}

// NewMember 创建一个成员
func NewMember(conn *webrtc.PeerConnection, userId string) *Member {
	if conn == nil {
		return nil
	}
	return &Member{
		userId: userId,
		mu:     sync.RWMutex{},
		conn:   conn,
	}
}

// UserId 获取成员用户id
func (m *Member) UserId() string {
	return m.userId
}

// SetRoom 记录成员所在房间
func (m *Member) SetRoom(room *Room) {
	if room == nil {
		return
	}
	m.room.Store(room)
}

// SetAudioTrack 保存音频track
func (m *Member) SetAudioTrack(track *webrtc.TrackLocalStaticRTP) {
	m.audioTrack.Store(track)
}

// SetVideoTrack 保存视频track
func (m *Member) SetVideoTrack(track *webrtc.TrackLocalStaticRTP) {
	m.videoTrack.Store(track)
}

// AudioTrack 获取音频track
func (m *Member) AudioTrack() *webrtc.TrackLocalStaticRTP {
	val := m.audioTrack.Load()
	if val != nil {
		return val.(*webrtc.TrackLocalStaticRTP)
	}
	return nil
}

// VideoTrack 获取视频track
func (m *Member) VideoTrack() *webrtc.TrackLocalStaticRTP {
	val := m.videoTrack.Load()
	if val != nil {
		return val.(*webrtc.TrackLocalStaticRTP)
	}
	return nil
}

// Room 获取成员所在房间
func (m *Member) Room() *Room {
	val := m.room.Load()
	if val != nil {
		return val.(*Room)
	}
	return nil
}

// GetOrNewRoom 获取或者创建房间
func GetOrNewRoom(id string) *Room {
	roomMu.Lock()
	defer roomMu.Unlock()
	room := roomMap[id]
	if room != nil {
		return room
	}
	room = &Room{
		id:      id,
		members: make(map[string]*Member, 8),
		mu:      sync.RWMutex{},
	}
	roomMap[id] = room
	return room
}

// CheckRoomMemberIsZero 检查房间成员数量是否为0
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

// GetRoom 获取单个房间
func GetRoom(roomId string) *Room {
	roomMu.RLock()
	defer roomMu.RUnlock()
	return roomMap[roomId]
}

// GetRoomList 获取房间列表
func GetRoomList() []*Room {
	roomMu.RLock()
	defer roomMu.RUnlock()
	ret := make([]*Room, 0, len(roomMap))
	for _, room := range roomMap {
		ret = append(ret, room)
	}
	return ret
}
