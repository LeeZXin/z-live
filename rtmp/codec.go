package rtmp

import "github.com/LeeZXin/z-live/amf"

// amfCodec amf编解码
type amfCodec struct {
	*amf.Decoder
	*amf.Encoder
}

func newAmfCodec() *amfCodec {
	return &amfCodec{
		Decoder: amf.NewDecoder(),
		Encoder: amf.NewEncoder(),
	}
}
