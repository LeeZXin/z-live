package sfu

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
	"net/http"
)

// newPeerConnection 初始化peerConnection
func newPeerConnection(isRecv bool) (*webrtc.PeerConnection, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeVP8,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: nil,
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: nil,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	i := &interceptor.Registry{}
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		return nil, err
	}
	i.Add(intervalPliFactory)
	if err = webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		panic(err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	}
	ret, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	if isRecv {
		if _, err = ret.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
			return nil, err
		}
		if _, err = ret.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

// RTPService 业务服务interface
type RTPService interface {
	// IsMediaRecvService 是否接收媒体音视频流
	// 通常保存音视频数据的服务需要返回true
	// 在多人通讯下 roomForwardService返回false
	IsMediaRecvService() bool
	// AuthenticateAndInit 鉴权并初始化数据
	AuthenticateAndInit(*http.Request) error
	// OnNewPeerConnection 当peerConnection构建时触发
	OnNewPeerConnection(*webrtc.PeerConnection)
	// OnDataChannel 触发dataChannel
	OnDataChannel(*webrtc.DataChannel)
	// OnTrack 触发音视频数据
	OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver)
	// OnClose 连接关闭时触发
	OnClose()
}
