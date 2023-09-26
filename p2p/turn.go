package p2p

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/pion/logging"
	"github.com/pion/turn/v3"
	"net"
)

func optimisticAuthHandler(username string, realm string, _ net.Addr) (key []byte, ok bool) {
	return turn.GenerateAuthKey(username, realm, "U_HAPPY_IS_OK"), true
}

type loggerFactoryImpl struct {
}

func (*loggerFactoryImpl) NewLogger(scope string) logging.LeveledLogger {
	return &loggerWrapper{}
}

type loggerWrapper struct {
}

func (*loggerWrapper) Trace(msg string) {
	logger.Logger.Trace(msg)
}

func (*loggerWrapper) Tracef(format string, args ...interface{}) {
	logger.Logger.Tracef(format, args)
}

func (*loggerWrapper) Debug(msg string) {
	logger.Logger.Debug(msg)
}

func (*loggerWrapper) Debugf(format string, args ...interface{}) {
	logger.Logger.Debugf(format, args)
}

func (*loggerWrapper) Info(msg string) {
	logger.Logger.Info(msg)
}

func (*loggerWrapper) Infof(format string, args ...interface{}) {
	logger.Logger.Debugf(format, args)
}

func (*loggerWrapper) Warn(msg string) {
	logger.Logger.Debug(msg)
}

func (*loggerWrapper) Warnf(format string, args ...interface{}) {
	logger.Logger.Warnf(format, args)
}

func (*loggerWrapper) Error(msg string) {
	logger.Logger.Error(msg)
}

func (*loggerWrapper) Errorf(format string, args ...interface{}) {
	logger.Logger.Errorf(format, args)
}

func StartTurnServer(udpAddr, tcpAddr, realm, turnIp string) {
	udpServer, err := net.ListenPacket("udp4", udpAddr)
	if err != nil {
		logger.Logger.Panic(err)
	}
	tcpServer, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		logger.Logger.Panic(err)
	}
	server, err := turn.NewServer(turn.ServerConfig{
		Realm:         realm,
		AuthHandler:   optimisticAuthHandler,
		LoggerFactory: &loggerFactoryImpl{},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpServer,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: net.ParseIP(turnIp),
					MinPort:      10000,
					MaxPort:      10300,
					Address:      "0.0.0.0",
				},
			},
		},
		ListenerConfigs: []turn.ListenerConfig{
			{
				Listener: tcpServer,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: net.ParseIP(turnIp),
					MinPort:      10301,
					MaxPort:      10600,
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		logger.Logger.Panic(err)
	}
	quit.AddShutdownHook(func() {
		server.Close()
	})
}
