package aac

import (
	"fmt"
	"github.com/LeeZXin/z-live/av"
	"io"
)

type mpegExtension struct {
	objectType byte
	sampleRate byte
}

type mpegCfgInfo struct {
	objectType     byte
	sampleRate     byte
	channel        byte
	sbr            byte
	ps             byte
	frameLen       byte
	exceptionLogTs int64
	extension      *mpegExtension
}

var aacRates = []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

var (
	specificBufInvalid = fmt.Errorf("audio mpegspecific error")
	audioBufInvalid    = fmt.Errorf("audiodata  invalid")
)

const (
	adtsHeaderLen = 7
)

type Parser struct {
	gettedSpecific bool
	adtsHeader     []byte
	cfgInfo        *mpegCfgInfo
	w              io.Writer
}

func NewParser(writer io.Writer) *Parser {
	return &Parser{
		w:              writer,
		gettedSpecific: false,
		cfgInfo:        &mpegCfgInfo{},
		adtsHeader:     make([]byte, adtsHeaderLen),
	}
}

func (p *Parser) specificInfo(src []byte) error {
	if len(src) < 2 {
		return specificBufInvalid
	}
	p.gettedSpecific = true
	p.cfgInfo.objectType = (src[0] >> 3) & 0xff
	p.cfgInfo.sampleRate = ((src[0] & 0x07) << 1) | src[1]>>7
	p.cfgInfo.channel = (src[1] >> 3) & 0x0f
	return nil
}

func (p *Parser) adts(src []byte) error {
	if len(src) <= 0 || !p.gettedSpecific {
		return audioBufInvalid
	}
	frameLen := uint16(len(src)) + 7
	p.adtsHeader[0] = 0xff
	p.adtsHeader[1] = 0xf1
	p.adtsHeader[2] &= 0x00
	p.adtsHeader[2] = p.adtsHeader[2] | (p.cfgInfo.objectType-1)<<6
	p.adtsHeader[2] = p.adtsHeader[2] | (p.cfgInfo.sampleRate)<<2
	p.adtsHeader[3] &= 0x00
	p.adtsHeader[3] = p.adtsHeader[3] | (p.cfgInfo.channel<<2)<<4
	p.adtsHeader[3] = p.adtsHeader[3] | byte((frameLen<<3)>>14)
	p.adtsHeader[4] &= 0x00
	p.adtsHeader[4] = p.adtsHeader[4] | byte((frameLen<<5)>>8)
	p.adtsHeader[5] &= 0x00
	p.adtsHeader[5] = p.adtsHeader[5] | byte(((frameLen<<13)>>13)<<5)
	p.adtsHeader[5] = p.adtsHeader[5] | (0x7C<<1)>>3
	p.adtsHeader[6] = 0xfc
	if _, err := p.w.Write(p.adtsHeader[0:]); err != nil {
		return err
	}
	if _, err := p.w.Write(src); err != nil {
		return err
	}
	return nil
}

func (p *Parser) SampleRate() int {
	rate := 44100
	if p.cfgInfo.sampleRate <= byte(len(aacRates)-1) {
		rate = aacRates[p.cfgInfo.sampleRate]
	}
	return rate
}

func (p *Parser) Parse(b []byte, packetType uint8) error {
	switch packetType {
	case av.AAC_SEQHDR:
		return p.specificInfo(b)
	case av.AAC_RAW:
		return p.adts(b)
	}
	return nil
}
