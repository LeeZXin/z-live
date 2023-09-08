package ts

import (
	"github.com/LeeZXin/z-live/av"
	"io"
)

const (
	tsDefaultDataLen = 184
	tsPacketLen      = 188
	h264DefaultHZ    = 90

	videoPID = 0x100
	audioPID = 0x101
	videoSID = 0xe0
	audioSID = 0xc0
)

type Muxer struct {
	videoCc  byte
	audioCc  byte
	patCc    byte
	pmtCc    byte
	pat      [tsPacketLen]byte
	pmt      [tsPacketLen]byte
	tsPacket [tsPacketLen]byte
	w        io.Writer
}

func NewMuxer(w io.Writer) *Muxer {
	return &Muxer{
		w: w,
	}
}

func (m *Muxer) WritePacket(p *av.Packet) error {
	first := true
	wBytes := 0
	pesIndex := 0
	tmpLen := byte(0)
	dataLen := byte(0)
	var pes pesHeader
	dts := int64(p.Timestamp) * int64(h264DefaultHZ)
	pts := dts
	pid := audioPID
	var videoH av.VideoPacketHeader
	if p.IsVideo {
		pid = videoPID
		videoH, _ = p.Header.(av.VideoPacketHeader)
		pts = dts + int64(videoH.CompositionTime())*int64(h264DefaultHZ)
	}
	err := pes.packet(p, pts, dts)
	if err != nil {
		return err
	}
	pesHeaderLen := pes.len
	packetBytesLen := len(p.Data) + int(pesHeaderLen)
	for {
		if packetBytesLen <= 0 {
			break
		}
		if p.IsVideo {
			m.videoCc++
			if m.videoCc > 0xf {
				m.videoCc = 0
			}
		} else {
			m.audioCc++
			if m.audioCc > 0xf {
				m.audioCc = 0
			}
		}
		var i byte = 0
		//sync byte
		m.tsPacket[i] = 0x47
		i++
		//error indicator, unit start indicator,ts priority,pid
		m.tsPacket[i] = byte(pid >> 8) //pid high 5 bits
		if first {
			m.tsPacket[i] = m.tsPacket[i] | 0x40 //unit start indicator
		}
		i++
		//pid low 8 bits
		m.tsPacket[i] = byte(pid)
		i++
		//scram control, adaptation control, counter
		if p.IsVideo {
			m.tsPacket[i] = 0x10 | byte(m.videoCc&0x0f)
		} else {
			m.tsPacket[i] = 0x10 | byte(m.audioCc&0x0f)
		}
		i++
		//关键帧需要加pcr
		if first && p.IsVideo && videoH.IsKeyFrame() {
			m.tsPacket[3] |= 0x20
			m.tsPacket[i] = 7
			i++
			m.tsPacket[i] = 0x50
			i++
			m.writePcr(m.tsPacket[0:], i, dts)
			i += 6
		}
		//frame data
		if packetBytesLen >= tsDefaultDataLen {
			dataLen = tsDefaultDataLen
			if first {
				dataLen -= i - 4
			}
		} else {
			m.tsPacket[3] |= 0x20 //have adaptation
			remainBytes := byte(0)
			dataLen = byte(packetBytesLen)
			if first {
				remainBytes = tsDefaultDataLen - dataLen - (i - 4)
			} else {
				remainBytes = tsDefaultDataLen - dataLen
			}
			m.adaptationBufInit(m.tsPacket[i:], remainBytes)
			i += remainBytes
		}
		if first && i < tsPacketLen && pesHeaderLen > 0 {
			tmpLen = tsPacketLen - i
			if pesHeaderLen <= tmpLen {
				tmpLen = pesHeaderLen
			}
			copy(m.tsPacket[i:], pes.data[pesIndex:pesIndex+int(tmpLen)])
			i += tmpLen
			packetBytesLen -= int(tmpLen)
			dataLen -= tmpLen
			pesHeaderLen -= tmpLen
			pesIndex += int(tmpLen)
		}
		if i < tsPacketLen {
			tmpLen = tsPacketLen - i
			if tmpLen <= dataLen {
				dataLen = tmpLen
			}
			copy(m.tsPacket[i:], p.Data[wBytes:wBytes+int(dataLen)])
			wBytes += int(dataLen)
			packetBytesLen -= int(dataLen)
		}
		if _, err = m.w.Write(m.tsPacket[0:]); err != nil {
			return err
		}
		first = false
	}
	return nil
}

// WritePAT return pat data
func (m *Muxer) WritePAT() error {
	i := 0
	remainByte := 0
	tsHeader := []byte{0x47, 0x40, 0x00, 0x10, 0x00}
	patHeader := []byte{0x00, 0xb0, 0x0d, 0x00, 0x01, 0xc1, 0x00, 0x00, 0x00, 0x01, 0xf0, 0x01}
	if m.patCc > 0xf {
		m.patCc = 0
	}
	tsHeader[3] |= m.patCc & 0x0f
	m.patCc++
	copy(m.pat[i:], tsHeader)
	i += len(tsHeader)
	copy(m.pat[i:], patHeader)
	i += len(patHeader)
	crc32Value := GenCrc32(patHeader)
	m.pat[i] = byte(crc32Value >> 24)
	i++
	m.pat[i] = byte(crc32Value >> 16)
	i++
	m.pat[i] = byte(crc32Value >> 8)
	i++
	m.pat[i] = byte(crc32Value)
	i++
	remainByte = tsPacketLen - i
	for j := 0; j < remainByte; j++ {
		m.pat[i+j] = 0xff
	}
	_, err := m.w.Write(m.pat[0:])
	return err
}

func (m *Muxer) WritePMT(soundFormat byte, hasVideo bool) error {
	i := 0
	j := 0
	var progInfo []byte
	remainBytes := 0
	tsHeader := []byte{0x47, 0x50, 0x01, 0x10, 0x00}
	pmtHeader := []byte{0x02, 0xb0, 0xff, 0x00, 0x01, 0xc1, 0x00, 0x00, 0xe1, 0x00, 0xf0, 0x00}
	if !hasVideo {
		pmtHeader[9] = 0x01
		progInfo = []byte{0x0f, 0xe1, 0x01, 0xf0, 0x00}
	} else {
		progInfo = []byte{0x1b, 0xe1, 0x00, 0xf0, 0x00, //h264 or h265*
			0x0f, 0xe1, 0x01, 0xf0, 0x00, //mp3 or aac
		}
	}
	pmtHeader[2] = byte(len(progInfo) + 9 + 4)
	if m.pmtCc > 0xf {
		m.pmtCc = 0
	}
	tsHeader[3] |= m.pmtCc & 0x0f
	m.pmtCc++
	if soundFormat == 2 ||
		soundFormat == 14 {
		if hasVideo {
			progInfo[5] = 0x4
		} else {
			progInfo[0] = 0x4
		}
	}
	copy(m.pmt[i:], tsHeader)
	i += len(tsHeader)
	copy(m.pmt[i:], pmtHeader)
	i += len(pmtHeader)
	copy(m.pmt[i:], progInfo[0:])
	i += len(progInfo)
	crc32Value := GenCrc32(m.pmt[5 : 5+len(pmtHeader)+len(progInfo)])
	m.pmt[i] = byte(crc32Value >> 24)
	i++
	m.pmt[i] = byte(crc32Value >> 16)
	i++
	m.pmt[i] = byte(crc32Value >> 8)
	i++
	m.pmt[i] = byte(crc32Value)
	i++
	remainBytes = tsPacketLen - i
	for j = 0; j < remainBytes; j++ {
		m.pmt[i+j] = 0xff
	}
	_, err := m.w.Write(m.pmt[0:])
	return err
}

func (m *Muxer) adaptationBufInit(src []byte, remainBytes byte) {
	src[0] = remainBytes - 1
	if remainBytes == 1 {
	} else {
		src[1] = 0x00
		for i := 2; i < len(src); i++ {
			src[i] = 0xff
		}
	}
	return
}

func (m *Muxer) writePcr(b []byte, i byte, pcr int64) {
	b[i] = byte(pcr >> 25)
	i++
	b[i] = byte((pcr >> 17) & 0xff)
	i++
	b[i] = byte((pcr >> 9) & 0xff)
	i++
	b[i] = byte((pcr >> 1) & 0xff)
	i++
	b[i] = byte(((pcr & 0x1) << 7) | 0x7e)
	i++
	b[i] = 0x00
}

type pesHeader struct {
	len  byte
	data [tsPacketLen]byte
}

// pesPacket return pes packet
func (header *pesHeader) packet(p *av.Packet, pts, dts int64) error {
	//PES header
	i := 0
	header.data[i] = 0x00
	i++
	header.data[i] = 0x00
	i++
	header.data[i] = 0x01
	i++
	sid := audioSID
	if p.IsVideo {
		sid = videoSID
	}
	header.data[i] = byte(sid)
	i++
	flag := 0x80
	ptslen := 5
	dtslen := ptslen
	headerSize := ptslen
	if p.IsVideo && pts != dts {
		flag |= 0x40
		headerSize += 5 //add dts
	}
	size := len(p.Data) + headerSize + 3
	if size > 0xffff {
		size = 0
	}
	header.data[i] = byte(size >> 8)
	i++
	header.data[i] = byte(size)
	i++
	header.data[i] = 0x80
	i++
	header.data[i] = byte(flag)
	i++
	header.data[i] = byte(headerSize)
	i++
	header.writeTs(header.data[0:], i, flag>>6, pts)
	i += ptslen
	if p.IsVideo && pts != dts {
		header.writeTs(header.data[0:], i, 1, dts)
		i += dtslen
	}
	header.len = byte(i)
	return nil
}

func (header *pesHeader) writeTs(src []byte, i int, fb int, ts int64) {
	val := uint32(0)
	if ts > 0x1ffffffff {
		ts -= 0x1ffffffff
	}
	val = uint32(fb<<4) | ((uint32(ts>>30) & 0x07) << 1) | 1
	src[i] = byte(val)
	i++
	val = ((uint32(ts>>15) & 0x7fff) << 1) | 1
	src[i] = byte(val >> 8)
	i++
	src[i] = byte(val)
	i++
	val = (uint32(ts&0x7fff) << 1) | 1
	src[i] = byte(val >> 8)
	i++
	src[i] = byte(val)
}
