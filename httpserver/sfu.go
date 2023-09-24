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
	"net/http"
	"os"
	"sync"
	"time"
)

/*
SfuServer 将展示webrtc协议下，sfu架构的通讯过程
利用websocket作为信令服务端交换sdp
可完成浏览器webrtc协议下
1、dataChannel传递数据，有http，感觉没什么用
2、将浏览器音视频数据实时保存在服务端本地
3、多人通过房间号音视频通讯

websocket使用zsf封装对websocket使用

多人音视频通讯采用多peerConnection架构
即有多少个成员，一个客户端就创建多少个peerConnection
可用于服务器调度和容灾考虑
具体查看readme
*/
type SfuServer struct {
	addr      string
	engine    *gin.Engine
	startOnce sync.Once
}

func NewSfuServer(addr string) *SfuServer {
	gin.SetMode(gin.ReleaseMode)
	ret := &SfuServer{
		addr:      addr,
		startOnce: sync.Once{},
	}
	engine := gin.New()
	// 创建data-channel的信令
	engine.Any("/signal-data-channel", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(sfu.NewDataChannelService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	// 将浏览器实时音视频保存在服务器
	engine.Any("/signal-video", ws.RegisterWebsocketService(func() ws.Service {
		// return sfu.NewSignalService(sfu.NewSaveIvfOggTrackService())
		return sfu.NewSignalService(sfu.NewSaveToWebmTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	// 多人通讯进入房间
	engine.Any("/signal-room", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(sfu.NewJoinTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	// 多人通讯下多PeerConnection 音视频转发
	engine.Any("/signal-forward", ws.RegisterWebsocketService(func() ws.Service {
		return sfu.NewSignalService(sfu.NewRoomForwardTrackService())
	}, ws.Config{
		MsgQueueSize: 8,
	}))
	// 将浏览器实时音视频保存在服务器 html页面
	engine.GET("/video.html", func(c *gin.Context) {
		openHtml("./resources/video.html", c)
	})
	// 多人通讯 html页面
	engine.GET("/room.html", func(c *gin.Context) {
		openHtml("./resources/room.html", c)
	})
	// dataChannel html页面
	engine.GET("/data-channel.html", func(c *gin.Context) {
		openHtml("./resources/data-channel.html", c)
	})
	// jq.js
	engine.GET("/jq.js", func(c *gin.Context) {
		openHtml("./resources/jq.js", c)
	})
	// 获取房间成员名单
	// 浏览器通过定时获取成员名单来创建多个<video></video>
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

// openHtml 打开本地文件
func openHtml(path string, c *gin.Context) {
	file, err := os.ReadFile(path)
	if err != nil {
		c.String(http.StatusNotFound, "error")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", file)
}
