package httpserver

import (
	"context"
	"errors"
	"github.com/LeeZXin/z-live/flv"
	"github.com/LeeZXin/z-live/rtmp"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	flvSuffix = ".flv"
)

/*
FlvServer 用于http-flv，传输rtmp数据，直播使用
*/
type FlvServer struct {
	addr   string
	engine *gin.Engine

	startOnce sync.Once
}

func NewFlvServer(addr string) *FlvServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		err := threadutil.RunSafe(func() {
			handleFlvRequest(c)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, "")
		}
	})
	ret := &FlvServer{
		addr:   addr,
		engine: engine,

		startOnce: sync.Once{},
	}
	return ret
}

func (s *FlvServer) ListenAndServe() {
	s.startOnce.Do(func() {
		logger.Logger.Info("listen flv http server: ", s.addr)
		server := &http.Server{
			Addr:        s.addr,
			ReadTimeout: 30 * time.Second,
			IdleTimeout: 30 * time.Second,
			Handler:     s.engine,
		}
		go func() {
			quit.AddShutdownHook(func() {
				logger.Logger.Info("shutdown flv http server")
				server.Shutdown(context.Background())
			})
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Logger.Panic(err.Error())
			}
		}()
	})
}

func handleFlvRequest(c *gin.Context) {
	u := c.Request.URL.Path
	if u == "/httpFlv.html" {
		videoUrl, b := c.GetQuery("u")
		if !b {
			c.String(http.StatusBadRequest, "invalid arguments")
			return
		}
		flvHtml, err := openHttpFlvHtml(videoUrl)
		if err != nil {
			c.String(http.StatusNotFound, "error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", flvHtml)
		return
	} else if u == "/flv.js" {
		flvJs, err := openFlvJs()
		if err != nil {
			c.String(http.StatusNotFound, "error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", flvJs)
		return
	}
	if path.Ext(u) != flvSuffix {
		c.String(http.StatusBadRequest, "invalid path")
		return
	}
	key, err := parseFlv(u)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	pub, ok := rtmp.FindPublisher(key)
	if !ok {
		c.String(http.StatusNotFound, "invalid path")
		return
	}
	writer := c.Writer
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Content-Type", "video/x-flv")
	writer.Header().Set("Transfer-Encoding", "chunked")
	writer.WriteHeader(http.StatusOK)
	httpWriter, err := flv.NewHttpWriter(writer)
	if err != nil {
		c.String(http.StatusInternalServerError, "init failed")
	}
	pub.Register(httpWriter)
	httpWriter.Wait()
}

func openHttpFlvHtml(url string) ([]byte, error) {
	file, err := os.ReadFile("./resources/http-flv.html")
	if err != nil {
		return nil, err
	}
	fileStr := string(file)
	return []byte(strings.ReplaceAll(fileStr, "{{videoPath}}", url)), nil
}

func openFlvJs() ([]byte, error) {
	file, err := os.ReadFile("./resources/flv.js")
	if err != nil {
		return nil, err
	}
	return file, nil
}

func parseFlv(pathStr string) (string, error) {
	pathStr = strings.TrimSuffix(strings.TrimLeft(pathStr, "/"), flvSuffix)
	split := strings.Split(pathStr, "/")
	if len(split) != 2 {
		return "", errors.New("invalid path")
	}
	return split[0] + "/" + split[1], nil
}
