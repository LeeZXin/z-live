package main

import (
	"github.com/LeeZXin/z-live/httpserver"
	"github.com/LeeZXin/z-live/rtmp"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	startRtmp()
	startHls()
	startFlv()
	zsf.Run()
}

func startRtmp() {
	server := rtmp.NewTcpServer(":1935")
	server.ListenAndServe()
}

func startHls() {
	server := httpserver.NewHlsServer(":1936")
	server.ListenAndServe()
}

func startFlv() {
	server := httpserver.NewFlvServer(":1937")
	server.ListenAndServe()
}
