package httpserver

import (
	"context"
	"errors"
	"github.com/LeeZXin/z-live/sfu"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/listutil"
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
	conn     *webrtc.PeerConnection
}

func NewDcService() sfu.RTPService {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &DcService{
		ctx:      ctx,
		cancelFn: cancelFunc,
	}
}

func (s *DcService) IsMediaRecvService() bool {
	return false
}

func (s *DcService) OnNewPeerConnection(conn *webrtc.PeerConnection) {
	s.conn = conn
}

func (s *DcService) AuthenticateAndInit(request *http.Request) error {
	logger.Logger.Info("header:", request.Header)
	return nil
}

func (s *DcService) OnTrack(*webrtc.TrackRemote, *webrtc.RTPReceiver) {}

func (s *DcService) OnDataChannel(dc *webrtc.DataChannel) {
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
		return sfu.NewSignalService(sfu.NewSaveIvfOggTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	engine.Any("/room", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(sfu.NewJoinTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	engine.Any("/forward", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(sfu.NewRoomForwardTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	engine.GET("/video", func(c *gin.Context) {
		openHtml("./resources/sfu-video.html", c)
	})
	engine.GET("/media", func(c *gin.Context) {
		openHtml("./resources/media-device.html", c)
	})
	engine.GET("/dataChannel", func(c *gin.Context) {
		openHtml("./resources/sfu-dataChannel-index.html", c)
	})
	engine.GET("/jq.js", func(c *gin.Context) {
		openHtml("./resources/jq.js", c)
	})
	engine.GET("/getMemberList", func(c *gin.Context) {
		roomId, b := c.GetQuery("room")
		if !b {
			c.JSON(http.StatusOK, gin.H{
				"data": []string{},
			})
			return
		}
		room := sfu.GetRoom(roomId)
		if room == nil {
			c.JSON(http.StatusOK, gin.H{
				"data": []string{},
			})
			return
		}
		members := room.Members()
		userIdList, _ := listutil.Map(members, func(t *sfu.Member) (string, error) {
			return t.UserId(), nil
		})
		c.JSON(http.StatusOK, gin.H{
			"data": userIdList,
		})
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

func openHtml(path string, c *gin.Context) {
	file, err := os.ReadFile(path)
	if err != nil {
		c.String(http.StatusNotFound, "error")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", file)
}
