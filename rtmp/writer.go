package rtmp

import "github.com/LeeZXin/z-live/av"

// PacketWriter 多种writer 实现接口
type PacketWriter interface {
	WritePacket(*av.Packet) error
	Close()
}
