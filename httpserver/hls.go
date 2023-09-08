package httpserver

import (
	"context"
	"errors"
	"github.com/LeeZXin/z-live/hls"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/threadutil"
	"github.com/gin-gonic/gin"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	m3u8Suffix = ".m3u8"
	tsSuffix   = ".ts"
)

var crossDomainXml = []byte(
	`<?xml version="1.0" ?>
		<cross-domain-policy>
		<allow-access-from domain="*" />
		<allow-http-request-headers-from domain="*" headers="*"/>
	</cross-domain-policy>`,
)

type HlsServer struct {
	addr   string
	engine *gin.Engine

	startOnce sync.Once
}

func NewHlsServer(addr string) *HlsServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		err := threadutil.RunSafe(func() {
			handleHlsRequest(c)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, "")
		}
	})
	ret := &HlsServer{
		addr:   addr,
		engine: engine,

		startOnce: sync.Once{},
	}
	return ret
}

func (s *HlsServer) ListenAndServe() {
	s.startOnce.Do(func() {
		logger.Logger.Info("listen hls http server: ", s.addr)
		server := &http.Server{
			Addr:         s.addr,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  30 * time.Second,
			Handler:      s.engine,
		}
		go func() {
			quit.AddShutdownHook(func() {
				logger.Logger.Info("shutdown hls http server")
				server.Shutdown(context.Background())
			})
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Logger.Panic(err.Error())
			}
		}()
	})
}

func handleHlsRequest(c *gin.Context) {
	if path.Base(c.Request.URL.Path) == "crossdomain.xml" {
		c.Header("Content-Type", "application/xml")
		c.Writer.Write(crossDomainXml)
		return
	}
	switch path.Ext(c.Request.URL.Path) {
	case m3u8Suffix:
		filePath, key, err := parseM3u8(c.Request.URL.Path)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		if hls.SaveFileFlag {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Cache-Control", "no-audioCache")
			c.Data(http.StatusOK, "application/x-mpegURL", hls.GetFileContent(filePath))
		} else {
			writer, ok := hls.FindStreamWriter(key)
			if !ok {
				c.String(http.StatusNotFound, "not found")
				return
			}
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Cache-Control", "no-audioCache")
			c.Data(http.StatusOK, "application/x-mpegURL", writer.GetM3u8Body())
		}
	case tsSuffix:
		filePath, key, err := parseTs(c.Request.RequestURI)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		if hls.SaveFileFlag {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Cache-Control", "no-audioCache")
			c.Data(http.StatusOK, "application/x-mpegURL", hls.GetFileContent(filePath))
		} else {
			writer, ok := hls.FindStreamWriter(key)
			if !ok {
				c.String(http.StatusNotFound, "not found")
				return
			}
			c.Header("Access-Control-Allow-Origin", "*")
			c.Data(http.StatusOK, "video/mp2ts", writer.GetTsBody(c.Request.RequestURI))
		}
	default:
		c.String(http.StatusBadRequest, "invalid request")
	}
}

func parseM3u8(pathStr string) (string, string, error) {
	pathStr = strings.TrimLeft(pathStr, "/")
	paths := strings.Split(pathStr, "/")
	if len(paths) != 3 {
		return "", "", errors.New("invalid path")
	}
	return pathStr, paths[0] + "/" + paths[1], nil
}

func parseTs(pathStr string) (string, string, error) {
	pathStr = strings.TrimLeft(pathStr, "/")
	paths := strings.Split(pathStr, "/")
	if len(paths) != 3 {
		return "", "", errors.New("invalid path")
	}
	return pathStr, paths[0] + "/" + paths[1], nil
}
