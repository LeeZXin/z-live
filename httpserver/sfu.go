package httpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/z-live/sfu"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/ws"
	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v4"
	"net/http"
	"os"
	"sync"
	"time"
)

type SfuServer struct {
	addr      string
	engine    *gin.Engine
	startOnce sync.Once
}

type DcService struct {
	ctx      context.Context
	cancelFn context.CancelFunc
}

func NewDcService() sfu.RTPService {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &DcService{
		ctx:      ctx,
		cancelFn: cancelFunc,
	}
}

func (s *DcService) AuthenticateAndInit(request *http.Request) error {
	logger.Logger.Info("header:", request.Header)
	return nil
}

func (s *DcService) OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver) {}

func (s *DcService) OnDataChannel(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					if err := dc.SendText(time.Now().String()); err != nil {
						return
					}
				}
			}
		}()
	})
}

func (s *DcService) OnClose() {
	logger.Logger.Info("closeeeee")
	s.cancelFn()
}

func NewSfuServer(addr string) *SfuServer {
	gin.SetMode(gin.ReleaseMode)
	ret := &SfuServer{
		addr:      addr,
		startOnce: sync.Once{},
	}
	engine := gin.New()
	engine.Any("/signal-dataChannel", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(NewDcService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	engine.Any("/signal-video", ws.RegisterWebsocketService(func() ws.Service {
		nano := time.Now().UnixNano()
		return sfu.NewSignalService(
			sfu.NewSaveToDiskTrackService(
				fmt.Sprintf("./%d.ogg", nano),
				fmt.Sprintf("./%d.ivf", nano),
				nil,
			),
		)
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	engine.GET("/video", func(c *gin.Context) {
		html, err := openHtml("./resources/sfu-video.html")
		if err != nil {
			c.String(http.StatusNotFound, "error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
	})
	engine.GET("/dataChannel", func(c *gin.Context) {
		html, err := openHtml("./resources/sfu-dataChannel-index.html")
		if err != nil {
			c.String(http.StatusNotFound, "error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", html)
	})
	ret.engine = engine
	return ret
}

func (s *SfuServer) ListenAndServe() {
	s.startOnce.Do(func() {
		logger.Logger.Info("listen sfu http server: ", s.addr)
		server := &http.Server{
			Addr:        s.addr,
			ReadTimeout: 30 * time.Second,
			IdleTimeout: 30 * time.Second,
			Handler:     s.engine,
		}
		go func() {
			quit.AddShutdownHook(func() {
				logger.Logger.Info("shutdown sfu http server")
				server.Shutdown(context.Background())
			})
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Logger.Panic(err.Error())
			}
		}()
	})
}

func openHtml(path string) ([]byte, error) {
	file, err := os.ReadFile(path)
	return file, err
}
