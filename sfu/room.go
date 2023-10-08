package sfu

import (
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/taskutil"
	"github.com/pion/webrtc/v4"
	"hash/crc32"
	"sync"
	"sync/atomic"
	"time"
)

var (
	roomHolder = newShardingRoom(64)
)

type shardingRoom struct {
	shardCount int
	entryList  []*roomEntry
}

func newShardingRoom(shardCount int) *shardingRoom {
	if shardCount <= 0 {
		shardCount = 8
	}
	entryList := make([]*roomEntry, shardCount)
	for i := 0; i < shardCount; i++ {
		entryList[i] = newRoomEntry()
	}
	return &shardingRoom{
		shardCount: shardCount,
		entryList:  entryList,
	}
}

func (e *shardingRoom) hash(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

func (e *shardingRoom) getEntry(key string) *roomEntry {
	return e.entryList[e.hash(key)%uint32(e.shardCount)]
}

func (e *shardingRoom) checkRoomMemberIsZero() {
	for i := 0; i < e.shardCount; i++ {
		e.entryList[i].checkRoomMemberIsZero()
	}
}

type roomEntry struct {
	// roomMap 存储房间
	roomMap map[string]*Room
	// roomMu 房间锁
	sync.RWMutex
}

func newRoomEntry() *roomEntry {
	return &roomEntry{
		roomMap: make(map[string]*Room),
		RWMutex: sync.RWMutex{},
	}
}

// getOrNewRoom 获取或者创建房间
func (e *roomEntry) getOrNewRoom(id string) *Room {
	e.Lock()
	defer e.Unlock()
	room := e.roomMap[id]
	if room != nil {
		return room
	}
	room = &Room{
		id:         id,
		members:    make(map[string]*Member, 8),
		mu:         sync.RWMutex{},
		createTime: time.Now(),
	}
	e.roomMap[id] = room
	return room
}

// GetRoom 获取单个房间
func (e *roomEntry) getRoom(roomId string) *Room {
	e.RLock()
	defer e.RUnlock()
	return e.roomMap[roomId]
}

func (e *roomEntry) checkRoomMemberIsZero() {
	now := time.Now()
	e.Lock()
	defer e.Unlock()
	for id, room := range e.roomMap {
		if now.Sub(room.createTime) > 30*time.Second && room.MemberSize() == 0 {
			delete(e.roomMap, id)
		}
	}
}

func init() {
	// 定时清除房间人数为0的房间
	task, _ := taskutil.NewPeriodicalTask(30*time.Second, roomHolder.checkRoomMemberIsZero)
	task.Start()
	// 程序退出时关闭定时任务
	quit.AddShutdownHook(func() {
		task.Stop()
	})
}

// Room 多人通讯房间
type Room struct {
	id         string
	members    map[string]*Member
	mu         sync.RWMutex
	createTime time.Time
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
	member.LeaveRoom()
	delete(r.members, member.UserId())
}

// Member 成员
type Member struct {
	conn       *webrtc.PeerConnection
	audioTrack atomic.Value
	videoTrack atomic.Value
	room       atomic.Value
	userId     string

	listeners []*webrtc.PeerConnection
	mu        sync.Mutex
	isDel     bool
}

// NewMember 创建一个成员
func NewMember(conn *webrtc.PeerConnection, userId string) *Member {
	if conn == nil {
		return nil
	}
	return &Member{
		userId:    userId,
		conn:      conn,
		listeners: make([]*webrtc.PeerConnection, 0, 8),
		mu:        sync.Mutex{},
	}
}

func (m *Member) AddListener(c *webrtc.PeerConnection) bool {
	if c == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isDel {
		return false
	}
	m.listeners = append(m.listeners, c)
	return true
}

func (m *Member) LeaveRoom() {
	m.mu.Lock()
	m.isDel = true
	m.mu.Unlock()
	for _, listener := range m.listeners {
		listener.Close()
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
	return roomHolder.getEntry(id).getOrNewRoom(id)
}

// GetRoom 获取单个房间
func GetRoom(id string) *Room {
	return roomHolder.getEntry(id).getRoom(id)
}
