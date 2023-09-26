package p2p

import (
	"errors"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/taskutil"
	"github.com/LeeZXin/zsf/ws"
	"hash/crc32"
	"sync"
	"sync/atomic"
	"time"
)

var (
	roomHolder = newShardingRoom(64)
)

func init() {
	// 定时清除房间人数为0的房间
	task, _ := taskutil.NewPeriodicalTask(30*time.Second, roomHolder.checkRoomMemberIsZero)
	task.Start()
	// 程序退出时关闭定时任务
	quit.AddShutdownHook(func() {
		task.Stop()
	})
}

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

// Room p2p通讯房间
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

// GetOtherMember 获取另外一个成员
func (r *Room) GetOtherMember(userId string) *Member {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for s := range r.members {
		if s != userId {
			return r.members[s]
		}
	}
	return nil
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
func (r *Room) AddMember(member *Member) error {
	if member == nil {
		return errors.New("nil member")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.members) >= 2 {
		return errors.New("member size greater than 2")
	}
	r.members[member.UserId()] = member
	member.SetShouldOffer(len(r.members) == 2)
	return nil
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
	room        atomic.Value
	shouldOffer atomic.Bool
	userId      string
	session     *ws.Session

	sendOfferOnce sync.Once

	offer, answer, candidate atomic.Value
}

// NewMember 创建一个成员
func NewMember(userId string, session *ws.Session) *Member {
	return &Member{
		userId:  userId,
		session: session,
	}
}

// UserId 获取成员用户id
func (m *Member) UserId() string {
	return m.userId
}

func (m *Member) SendOffer(offer string) error {
	msg := WsMsg{
		MsgType: OfferType,
		Content: offer,
	}
	return m.session.WriteTextMessage(msg.String())
}

func (m *Member) AskToSendOffer() error {
	msg := WsMsg{
		MsgType: ActionType,
		Content: "sendOffer",
	}
	return m.session.WriteTextMessage(msg.String())
}

func (m *Member) NotifyToRecvAnswer() error {
	msg := WsMsg{
		MsgType: ActionType,
		Content: "recvAnswer",
	}
	return m.session.WriteTextMessage(msg.String())
}

func (m *Member) SendAnswer(answer string) error {
	msg := WsMsg{
		MsgType: AnswerType,
		Content: answer,
	}
	return m.session.WriteTextMessage(msg.String())
}

func (m *Member) SendCandidate(candidate string) error {
	msg := WsMsg{
		MsgType: CandidateType,
		Content: candidate,
	}
	return m.session.WriteTextMessage(msg.String())
}

// SetRoom 记录成员所在房间
func (m *Member) SetRoom(room *Room) {
	if room == nil {
		return
	}
	m.room.Store(room)
}

func (m *Member) SetShouldOffer(shouldOffer bool) {
	m.shouldOffer.Store(shouldOffer)
}

// Room 获取成员所在房间
func (m *Member) Room() *Room {
	val := m.room.Load()
	if val != nil {
		return val.(*Room)
	}
	return nil
}

// GetOtherMember 获取另外一个成员
func (m *Member) GetOtherMember() *Member {
	return m.Room().GetOtherMember(m.userId)
}

func (m *Member) ShouldOffer() bool {
	return m.shouldOffer.Load()
}

func (m *Member) SetOffer(offer string) {
	m.offer.Store(offer)
}

func (m *Member) Offer() string {
	ret := m.offer.Load()
	if ret == nil {
		return ""
	}
	return ret.(string)
}

func (m *Member) SetAnswer(answer string) {
	m.answer.Store(answer)
}

func (m *Member) Answer() string {
	ret := m.answer.Load()
	if ret == nil {
		return ""
	}
	return ret.(string)
}

func (m *Member) SetCandidate(candidate string) {
	m.candidate.Store(candidate)
}

func (m *Member) Candidate() string {
	ret := m.candidate.Load()
	if ret == nil {
		return ""
	}
	return ret.(string)
}

// GetOrNewRoom 获取或者创建房间
func GetOrNewRoom(id string) *Room {
	return roomHolder.getEntry(id).getOrNewRoom(id)
}
