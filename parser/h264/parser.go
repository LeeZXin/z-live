package h264

import (
	"bytes"
	"fmt"
	"io"
)

const (
	nalu_type_slice    byte = 1  //slice_layer_without_partioning_rbsp() sliceheader
	nalu_type_dpa      byte = 2  // slice_data_partition_a_layer_rbsp( ), slice_header
	nalu_type_dpb      byte = 3  // slice_data_partition_b_layer_rbsp( )
	nalu_type_dpc      byte = 4  // slice_data_partition_c_layer_rbsp( )
	nalu_type_idr      byte = 5  // slice_layer_without_partitioning_rbsp( ),sliceheader
	nalu_type_sei      byte = 6  //sei_rbsp( )
	nalu_type_sps      byte = 7  //seq_parameter_set_rbsp( )
	nalu_type_pps      byte = 8  //pic_parameter_set_rbsp( )
	nalu_type_aud      byte = 9  // access_unit_delimiter_rbsp( )
	nalu_type_eoesq    byte = 10 //end_of_seq_rbsp( )
	nalu_type_eostream byte = 11 //end_of_stream_rbsp( )
	nalu_type_filler   byte = 12 //filler_data_rbsp( )
)

const (
	naluBytesLen int = 4
	maxSpsPpsLen int = 2 * 1024
)

var (
	decDataNil        = fmt.Errorf("dec buf is nil")
	spsDataError      = fmt.Errorf("sps data error")
	ppsHeaderError    = fmt.Errorf("pps header error")
	ppsDataError      = fmt.Errorf("pps data error")
	naluHeaderInvalid = fmt.Errorf("nalu header invalid")
	videoDataInvalid  = fmt.Errorf("video data not match")
	dataSizeNotMatch  = fmt.Errorf("data size not match")
	naluBodyLenError  = fmt.Errorf("nalu body len error")
)

var startCode = []byte{0x00, 0x00, 0x00, 0x01}
var naluAud = []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0xf0}

type Parser struct {
	specificInfo []byte
	pps          *bytes.Buffer
	w            io.Writer
	spsInfo      *SPS
}

type sequenceHeader struct {
	version              byte //8bits
	profileIndication    byte //8bits
	profileCompatibility byte //8bits
	levelIndication      byte //8bits
	reserved1            byte //6bits
	naluLen              byte //2bits
	reserved2            byte //3bits
	spsNum               byte //5bits
	ppsNum               byte //8bits
	spsLen               int
	ppsLen               int
}

func NewParser(writer io.Writer) *Parser {
	return &Parser{
		pps: bytes.NewBuffer(make([]byte, maxSpsPpsLen)),
		w:   writer,
	}
}

// return value 1:sps, value2 :pps
func (p *Parser) parseSpecificInfo(src []byte) error {
	if len(src) < 9 {
		return decDataNil
	}
	var sps []byte
	var pps []byte
	var seq sequenceHeader
	seq.version = src[0]
	seq.profileIndication = src[1]
	seq.profileCompatibility = src[2]
	seq.levelIndication = src[3]
	seq.reserved1 = src[4] & 0xfc
	seq.naluLen = src[4]&0x03 + 1
	seq.reserved2 = src[5] >> 5
	//get sps
	seq.spsNum = src[5] & 0x1f
	seq.spsLen = int(src[6])<<8 | int(src[7])
	if len(src[8:]) < seq.spsLen || seq.spsLen <= 0 {
		return spsDataError
	}
	sps = append(sps, startCode...)
	spsBits := src[8:(8 + seq.spsLen)]
	sps = append(sps, spsBits...)
	// 可以用来获取分辨率等信息
	spsRet, err := ParseSPSNALUnit(spsBits, true)
	if err == nil {
		p.spsInfo = spsRet
	}
	//get pps
	tmpBuf := src[(8 + seq.spsLen):]
	if len(tmpBuf) < 4 {
		return ppsHeaderError
	}
	seq.ppsNum = tmpBuf[0]
	seq.ppsLen = int(0)<<16 | int(tmpBuf[1])<<8 | int(tmpBuf[2])
	if len(tmpBuf[3:]) < seq.ppsLen || seq.ppsLen <= 0 {
		return ppsDataError
	}
	pps = append(pps, startCode...)
	pps = append(pps, tmpBuf[3:]...)
	p.specificInfo = append(p.specificInfo, sps...)
	p.specificInfo = append(p.specificInfo, pps...)
	return nil
}

func (p *Parser) isNaluHeader(src []byte) bool {
	if len(src) < naluBytesLen {
		return false
	}
	return src[0] == 0x00 &&
		src[1] == 0x00 &&
		src[2] == 0x00 &&
		src[3] == 0x01
}

func (p *Parser) naluSize(src []byte) (int, error) {
	if len(src) < naluBytesLen {
		return 0, naluHeaderInvalid
	}
	buf := src[:naluBytesLen]
	size := 0
	for i := 0; i < len(buf); i++ {
		size = size<<8 + int(buf[i])
	}
	return size, nil
}

func (p *Parser) getAnnexbH264(src []byte) error {
	dataSize := len(src)
	if dataSize < naluBytesLen {
		return videoDataInvalid
	}
	p.pps.Reset()
	_, err := p.w.Write(naluAud)
	if err != nil {
		return err
	}
	index := 0
	nalLen := 0
	hasSpsPps := false
	hasWriteSpsPps := false
	for dataSize > 0 {
		nalLen, err = p.naluSize(src[index:])
		if err != nil {
			return dataSizeNotMatch
		}
		index += naluBytesLen
		dataSize -= naluBytesLen
		if dataSize >= nalLen && len(src[index:]) >= nalLen && nalLen > 0 {
			nalType := src[index] & 0x1f
			switch nalType {
			case nalu_type_aud:
			case nalu_type_idr:
				if !hasWriteSpsPps {
					hasWriteSpsPps = true
					if !hasSpsPps {
						if _, err = p.w.Write(p.specificInfo); err != nil {
							return err
						}
					} else {
						if _, err = p.w.Write(p.pps.Bytes()); err != nil {
							return err
						}
					}
				}
				fallthrough
			case nalu_type_slice:
				fallthrough
			case nalu_type_sei:
				_, err = p.w.Write(startCode)
				if err != nil {
					return err
				}
				_, err = p.w.Write(src[index : index+nalLen])
				if err != nil {
					return err
				}
			case nalu_type_sps:
				fallthrough
			case nalu_type_pps:
				hasSpsPps = true
				_, err = p.pps.Write(startCode)
				if err != nil {
					return err
				}
				_, err = p.pps.Write(src[index : index+nalLen])
				if err != nil {
					return err
				}
			}
			index += nalLen
			dataSize -= nalLen
		} else {
			return naluBodyLenError
		}
	}
	return nil
}

func (p *Parser) Parse(b []byte, isSeq bool) error {
	if isSeq {
		return p.parseSpecificInfo(b)
	}
	if p.isNaluHeader(b) {
		_, err := p.w.Write(b)
		return err
	}
	return p.getAnnexbH264(b)
}
