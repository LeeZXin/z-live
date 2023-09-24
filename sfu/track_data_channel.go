package sfu

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/pion/webrtc/v4"
	"net/http"
	"time"
)

type DataChannelService struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	conn     *webrtc.PeerConnection
}

func NewDataChannelService() RTPService {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &DataChannelService{
		ctx:      ctx,
		cancelFn: cancelFunc,
	}
}

func (s *DataChannelService) IsMediaRecvService() bool {
	return false
}

func (s *DataChannelService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *DataChannelService) AuthenticateAndInit(request *http.Request) error {
	logger.Logger.Info("header:", request.Header)
	return nil
}

func (s *DataChannelService) OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver) {}

func (s *DataChannelService) OnDataChannel(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		go func() {
			i := 0
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					i += 1
					if err := dc.SendText(time.Now().String()); err != nil {
						logger.Logger.Error(err.Error())
						return
					}
				}
				if i == 5 {
					dc.Close()
				}
			}
		}()
	})
}

func (s *DataChannelService) OnClose() {
	s.cancelFn()
}
