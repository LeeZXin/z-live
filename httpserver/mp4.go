package httpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/util/threadutil"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	mp4Suffix = ".mp4"
)

type Mp4Server struct {
	addr   string
	engine *gin.Engine

	startOnce sync.Once
}

func NewMp4Server(addr string) *Mp4Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		err := threadutil.RunSafe(func() {
			handleMp4Request(c)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, "")
		}
	})
	ret := &Mp4Server{
		addr:   addr,
		engine: engine,

		startOnce: sync.Once{},
	}
	return ret
}

func (s *Mp4Server) ListenAndServe() {
	s.startOnce.Do(func() {
		logger.Logger.Info("listen mp4 http server: ", s.addr)
		server := &http.Server{
			Addr:        s.addr,
			ReadTimeout: 30 * time.Second,
			IdleTimeout: 30 * time.Second,
			Handler:     s.engine,
		}
		go func() {
			quit.AddShutdownHook(func() {
				logger.Logger.Info("shutdown mp4 http server")
				server.Shutdown(context.Background())
			})
			err := server.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Logger.Panic(err.Error())
			}
		}()
	})
}

func handleMp4Request(c *gin.Context) {
	u := c.Request.URL.Path
	if path.Ext(u) != mp4Suffix {
		c.String(http.StatusBadRequest, "invalid path")
		return
	}
	// Read the MP4 file from disk
	videoFilePath := strings.TrimLeft(u, "/")
	videoFile, err := os.Open(videoFilePath)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to open the video file")
		return
	}
	defer videoFile.Close()
	fmt.Println(c.Request.Header)
	// Set the Content-Type header
	c.Header("Content-Type", "video/mp4")
	// Set the Accept-Ranges header
	c.Header("Accept-Ranges", "bytes")
	// Get the range header value from the request
	c.Status(http.StatusOK)
	http.ServeContent(c.Writer, c.Request, "", time.Now(), videoFile)
}
