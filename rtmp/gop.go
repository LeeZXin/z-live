package rtmp

import (
	"github.com/LeeZXin/z-live/av"
)

var (
	maxGOPCap = 1024
)

type gop struct {
	packets []*av.Packet
}

func newGop() *gop {
	return &gop{
		packets: make([]*av.Packet, 0, maxGOPCap),
	}
}

func (g *gop) add(p *av.Packet) {
	g.packets = append(g.packets, p)
}

func (g *gop) reset() {
	g.packets = g.packets[:0]
}

type gopRingQueue struct {
	start  bool
	gopNum int
	index  int
	queue  []*gop
}

func newGopRingQueue(gopNum int) *gopRingQueue {
	if gopNum <= 0 {
		gopNum = 1
	}
	queue := make([]*gop, gopNum)
	return &gopRingQueue{
		index:  -1,
		gopNum: gopNum,
		queue:  queue,
	}
}

func (q *gopRingQueue) send(writer PacketWriter) error {
	if q.queue[q.gopNum-1] != nil {
		for i := q.index + 1; i < q.gopNum; i++ {
			for _, p := range q.queue[i].packets {
				if err := writer.WritePacket(p); err != nil {
					return err
				}
			}
		}
	}
	for i := 0; i <= q.index; i++ {
		for _, p := range q.queue[i].packets {
			if err := writer.WritePacket(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q *gopRingQueue) add(p *av.Packet) {
	if p == nil {
		return
	}
	var isIFrame bool
	if p.IsVideo {
		vh := p.Header.(av.VideoPacketHeader)
		if vh.IsKeyFrame() && !vh.IsSeq() {
			isIFrame = true
		}
	}
	if isIFrame || q.start {
		if isIFrame {
			q.start = true
			q.index = (q.index + 1) % q.gopNum
			if q.queue[q.index] == nil {
				q.queue[q.index] = newGop()
			} else {
				q.queue[q.index].reset()
			}
		}
		q.queue[q.index].add(p)
	}
}
