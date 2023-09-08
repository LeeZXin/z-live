package rtmp

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/LeeZXin/z-live/amf"
	"github.com/LeeZXin/z-live/av"
	"github.com/LeeZXin/zsf/property"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net"
	"net/url"
	"strings"
)

var (
	respResult     = "_result"
	respError      = "_error"
	onStatus       = "onStatus"
	publishStart   = "NetStream.Publish.Start"
	playStart      = "NetStream.Play.Start"
	connectSuccess = "NetConnection.Connect.Success"
	onBWDone       = "onBWDone"

	ErrFail = errors.New("respone err")
)

var (
	publishLive   = "live"
	publishRecord = "record"
	publishAppend = "append"
)

type cmdCli struct {
	conn *netConn

	streamId      uint32
	currentCmd    string
	transactionId int
	pubName       string
	url           string
	app           string
	tcUrl         string

	codec  *amfCodec
	bytesw *bytes.Buffer
}

func newCmdCli() *cmdCli {
	return &cmdCli{
		transactionId: 1,
		bytesw:        bytes.NewBuffer(nil),
		codec:         newAmfCodec(),
	}
}

func (c *cmdCli) writeConnectMsg() error {
	event := make(amf.Object)
	event["app"] = c.app
	event["type"] = "nonprivate"
	event["flashVer"] = "FMS.3.1"
	event["tcUrl"] = c.tcUrl
	c.currentCmd = cmdConnect
	if err := c.writeMsg(cmdConnect, c.transactionId, event); err != nil {
		return err
	}
	return c.readRespMsg()
}

func (c *cmdCli) writeMsg(args ...any) error {
	c.bytesw.Reset()
	for _, v := range args {
		if _, err := c.codec.Encode(c.bytesw, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := c.bytesw.Bytes()
	cs := chunkStream{
		fmt:       0,
		csid:      3,
		timestamp: 0,
		typeId:    20,
		streamId:  c.streamId,
		length:    uint32(len(msg)),
		data:      msg,
	}
	if err := cs.writeChunk(c.conn); err != nil {
		return err
	}
	return c.conn.Flush()
}

func (c *cmdCli) readRespMsg() error {
	if err := readChunkStream(c.conn); err != nil {
		return err
	}
	cs := c.conn.cs
	switch cs.typeId {
	case 20, 17:
		vs, _ := c.codec.DecodeBatch(bytes.NewReader(cs.data), amf.AMF0)
		for k, v := range vs {
			switch v.(type) {
			case string:
				switch c.currentCmd {
				case cmdConnect, cmdCreateStream:
					if v.(string) != respResult {
						return fmt.Errorf(v.(string))
					}
				case cmdPublish:
					if v.(string) != onStatus {
						return ErrFail
					}
				}
			case float64:
				switch c.currentCmd {
				case cmdConnect, cmdCreateStream:
					id := int(v.(float64))
					if k == 1 {
						if id != c.transactionId {
							return ErrFail
						}
					} else if k == 3 {
						c.streamId = uint32(id)
					}
				case cmdPublish:
					if int(v.(float64)) != 0 {
						return ErrFail
					}
				}
			case amf.Object:
				objmap := v.(amf.Object)
				switch c.currentCmd {
				case cmdConnect:
					code, ok := objmap["code"]
					if ok && code.(string) != connectSuccess {
						return ErrFail
					}
				case cmdPublish:
					code, ok := objmap["code"]
					if ok && code.(string) != publishStart {
						return ErrFail
					}
				}
			}
		}
	}
	return nil
}

func (c *cmdCli) writeCreateStreamMsg() error {
	c.transactionId++
	c.currentCmd = cmdCreateStream
	if err := c.writeMsg(cmdCreateStream, c.transactionId, nil); err != nil {
		return err
	}
	return c.readRespMsg()
}

func (c *cmdCli) writePublishMsg() error {
	c.transactionId++
	c.currentCmd = cmdPublish
	if err := c.writeMsg(cmdPublish, c.transactionId, nil, c.pubName, publishLive); err != nil {
		return err
	}
	return c.readRespMsg()
}

func (c *cmdCli) writePlayMsg() error {
	c.transactionId++
	c.currentCmd = cmdPlay
	if err := c.writeMsg(cmdPlay, 0, nil, c.pubName); err != nil {
		return err
	}
	return c.readRespMsg()
}

func (c *cmdCli) Start(req string, method string) error {
	u, err := url.Parse(req)
	if err != nil {
		return err
	}
	c.url = req
	ps := strings.SplitN(strings.TrimLeft(u.Path, "/"), "/", 2)
	if len(ps) != 2 {
		return fmt.Errorf("u path err: %s", req)
	}
	c.app = ps[0]
	c.pubName = ps[1]
	if u.RawQuery != "" {
		c.pubName += "?" + u.RawQuery
	}
	isRtmps := strings.HasPrefix(req, "rtmps://")
	var port string
	if isRtmps {
		c.tcUrl = "rtmps://" + u.Host + "/" + c.app
		port = ":443"
	} else {
		c.tcUrl = "rtmp://" + u.Host + "/" + c.app
		port = ":1935"
	}
	host := u.Host
	var remoteIP string
	if strings.Contains(host, ":") {
		host, port, err = net.SplitHostPort(host)
		if err != nil {
			return err
		}
		port = ":" + port
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	remoteIP = ips[rand.Intn(len(ips))].String()
	if strings.Index(remoteIP, ":") == -1 {
		remoteIP += port
	}
	var conn net.Conn
	if isRtmps {
		var config tls.Config
		if property.GetBool("enable_tls_verify") {
			roots, err := x509.SystemCertPool()
			if err != nil {
				log.Warning(err)
				return err
			}
			config.RootCAs = roots
		} else {
			config.InsecureSkipVerify = true
		}
		conn, err = tls.Dial("tcp", remoteIP, &config)
		if err != nil {
			return err
		}
	} else {
		conn, err = net.Dial("tcp", remoteIP)
		if err != nil {
			return err
		}
	}
	c.conn = newNetConn(conn, 4*1024)
	if err = handshake2Server(c.conn); err != nil {
		return err
	}
	if err = c.writeConnectMsg(); err != nil {
		return err
	}
	if err = c.writeCreateStreamMsg(); err != nil {
		return err
	}
	switch method {
	case av.PUBLISH:
		if err = c.writePublishMsg(); err != nil {
			return err
		}
	case av.PLAY:
		if err = c.writePlayMsg(); err != nil {
			return err
		}
	default:
		return errors.New("unsupported method")
	}
	return nil
}

func (c *cmdCli) Write(cs chunkStream) error {
	if cs.typeId == av.TAG_SCRIPTDATAAMF0 ||
		cs.typeId == av.TAG_SCRIPTDATAAMF3 {
		var err error
		if cs.data, err = amf.MetaDataReform(cs.data, amf.ADD); err != nil {
			return err
		}
		cs.length = uint32(len(cs.data))
	}
	return cs.writeChunk(c.conn)
}

func (c *cmdCli) Flush() error {
	return c.conn.Flush()
}
