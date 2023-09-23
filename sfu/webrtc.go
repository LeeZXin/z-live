package sfu

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
	"net/http"
)

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

type RTPService interface {
	IsMediaRecvService() bool
	AuthenticateAndInit(*http.Request) error
	OnNewPeerConnection(*webrtc.PeerConnection)
	OnDataChannel(*webrtc.DataChannel)
	OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver)
	OnClose()
}
