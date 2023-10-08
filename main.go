package main

import (
	"github.com/LeeZXin/z-live/httpserver"
	"github.com/LeeZXin/z-live/p2p"
	"github.com/LeeZXin/z-live/rtmp"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	startRtmp()
	startFlv()
	/*
		startHls()

		startMp4()

		startTurn()
		startP2pSignal()*/
	startSfu()
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

func startMp4() {
	server := httpserver.NewMp4Server(":1938")
	server.ListenAndServe()
}

func startSfu() {
	server := httpserver.NewSfuServer(":1939")
	server.ListenAndServe()
}

func startTurn() {
	p2p.StartTurnServer(":1940", ":1941", "z-live", "127.0.0.1")
}

func startP2pSignal() {
	server := httpserver.NewP2pSignalServer(":1942")
	server.ListenAndServe()
}
