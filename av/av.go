package av

const (
	TAG_AUDIO          = 8
	TAG_VIDEO          = 9
	TAG_SCRIPTDATAAMF0 = 18
	TAG_SCRIPTDATAAMF3 = 0xf
)

const (
	MetadatAMF0  = 0x12
	MetadataAMF3 = 0xf
)

const (
	SOUND_MP3                   = 2
	SOUND_NELLYMOSER_16KHZ_MONO = 4
	SOUND_NELLYMOSER_8KHZ_MONO  = 5
	SOUND_NELLYMOSER            = 6
	SOUND_ALAW                  = 7
	SOUND_MULAW                 = 8
	SOUND_AAC                   = 10
	SOUND_SPEEX                 = 11

	SOUND_5_5Khz = 0
	SOUND_11Khz  = 1
	SOUND_22Khz  = 2
	SOUND_44Khz  = 3

	SOUND_8BIT  = 0
	SOUND_16BIT = 1

	SOUND_MONO   = 0
	SOUND_STEREO = 1

	AAC_SEQHDR = 0
	AAC_RAW    = 1
)

const (
	AVC_SEQHDR = 0
	AVC_NALU   = 1
	AVC_EOS    = 2

	FRAME_KEY   = 1
	FRAME_INTER = 2

	VIDEO_H264 = 7
)

var (
	PUBLISH = "publish"
	PLAY    = "play"
)

type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	Timestamp  uint32 // dts
	StreamId   uint32
	Header     PacketHeader
	Data       []byte
}

func (p *Packet) Copy() *Packet {
	ret := new(Packet)
	ret.IsAudio = p.IsAudio
	ret.IsVideo = p.IsVideo
	ret.IsMetadata = p.IsMetadata
	ret.Timestamp = p.Timestamp
	ret.StreamId = p.StreamId
	ret.Header = p.Header
	ret.Data = make([]byte, len(p.Data))
	copy(ret.Data, p.Data)
	return ret
}

type PacketHeader interface {
}

type AudioPacketHeader interface {
	PacketHeader
	SoundFormat() uint8
	AACPacketType() uint8
}

type VideoPacketHeader interface {
	PacketHeader
	IsKeyFrame() bool
	IsSeq() bool
	CodecID() uint8
	CompositionTime() int32
}
