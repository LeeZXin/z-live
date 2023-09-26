package httpserver

import (
	"context"
	"errors"
	"github.com/LeeZXin/z-live/p2p"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/ws"
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"time"
)

type P2pSignalServer struct {
	addr      string
	engine    *gin.Engine
	startOnce sync.Once
}

func NewP2pSignalServer(addr string) *P2pSignalServer {
	gin.SetMode(gin.ReleaseMode)
	ret := &P2pSignalServer{
		addr:      addr,
		startOnce: sync.Once{},
	}
	engine := gin.New()
	// 信令交换
	engine.Any("/signal", ws.RegisterWebsocketService(func() ws.Service {
		return p2p.NewSignalService()
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	// 将浏览器p2p音视频 html页面
	engine.GET("/p2p-video.html", func(c *gin.Context) {
		openHtml("./resources/p2p-video.html", c)
	})
	// dataChannel html页面
	engine.GET("/p2p-data-channel.html", func(c *gin.Context) {
		openHtml("./resources/p2p-data-channel.html", c)
	})
	ret.engine = engine
	return ret
}

func (s *P2pSignalServer) ListenAndServe() {
	s.startOnce.Do(func() {
		logger.Logger.Info("listen p2p signal http server: ", s.addr)
		server := &http.Server{
			Addr:        s.addr,
			ReadTimeout: 30 * time.Second,
			IdleTimeout: 30 * time.Second,
			Handler:     s.engine,
		}
		go func() {
			quit.AddShutdownHook(func() {
				logger.Logger.Info("shutdown p2p signal http server")
				server.Shutdown(context.Background())
			})
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Logger.Panic(err.Error())
			}
		}()
	})
}
