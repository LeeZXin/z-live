package rtmp

import (
	"bytes"
	"errors"
	"github.com/LeeZXin/z-live/amf"
	"github.com/LeeZXin/zsf/logger"
)

var (
	ErrReq = errors.New("wrong req")
)

type connectCmd struct {
	App            string `json:"app"`
	FlashVer       string `json:"flashVer"`
	SwfUrl         string `json:"swfUrl"`
	TcUrl          string `json:"tcUrl"`
	Fpad           bool   `json:"fpad"`
	AudioCodecs    int    `json:"audioCodecs"`
	VideoCodecs    int    `json:"videoCodecs"`
	VideoFunction  int    `json:"videoFunction"`
	PageUrl        string `json:"pageUrl"`
	ObjectEncoding int    `json:"objectEncoding"`
}

type publishCmd struct {
	PubName string `json:"pubName"`
	PubType string `json:"pubType"`
}

type cmdHandler struct {
	conn *netConn

	transactionId int
	streamId      int

	cntCmd *connectCmd
	pubCmd *publishCmd

	buf *bytes.Buffer
}

func (c *cmdHandler) connect(vs []any) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
		case float64:
			id := int(v.(float64))
			if id != 1 {
				return ErrReq
			}
			c.transactionId = id
		case amf.Object:
			objMap := v.(amf.Object)
			logger.Logger.Debugf("amf obj: %v", objMap)
			cmd := &connectCmd{}
			if app, ok := objMap["app"]; ok {
				cmd.App = app.(string)
			}
			if flashVer, ok := objMap["flashVer"]; ok {
				cmd.FlashVer = flashVer.(string)
			}
			if tcurl, ok := objMap["tcUrl"]; ok {
				cmd.TcUrl = tcurl.(string)
			}
			if encoding, ok := objMap["objectEncoding"]; ok {
				cmd.ObjectEncoding = int(encoding.(float64))
			}
			c.cntCmd = cmd
		}
	}
	return nil
}

func (c *cmdHandler) releaseStream(vs []any) error {
	return nil
}

func (c *cmdHandler) fcPublish(vs []any) error {
	return nil
}

func (c *cmdHandler) connectResp(cur *chunkStream) error {
	cs := newWindowAckSizeCs(2500000)
	if err := cs.writeChunk(c.conn); err != nil {
		return err
	}
	cs = newSetPeerBandwidthCs(2500000)
	if err := cs.writeChunk(c.conn); err != nil {
		return err
	}
	cs = newSetChunkSizeCs(uint32(1024))
	if err := cs.writeChunk(c.conn); err != nil {
		return err
	}
	resp := make(amf.Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["ObjectEncoding"] = c.cntCmd.ObjectEncoding
	return c.writeMsg(cur.csid, cur.streamId, "_result", c.transactionId, resp, event)
}

func (c *cmdHandler) writeMsg(csid, streamId uint32, args ...any) error {
	c.buf.Reset()
	for _, v := range args {
		if _, err := c.conn.amfCodec.Encode(c.buf, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := c.buf.Bytes()
	cs := chunkStream{
		fmt:       0,
		csid:      csid,
		timestamp: 0,
		typeId:    20,
		streamId:  streamId,
		length:    uint32(len(msg)),
		data:      msg,
	}
	if err := cs.writeChunk(c.conn); err != nil {
		return err
	}
	return c.conn.Flush()
}

func (c *cmdHandler) createStream(vs []any) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
		case float64:
			c.transactionId = int(v.(float64))
		case amf.Object:
		}
	}
	return nil
}

func (c *cmdHandler) getKey() (string, string) {
	if c.cntCmd != nil && c.pubCmd != nil {
		return c.cntCmd.App, c.pubCmd.PubName
	}
	return "", ""
}

func (c *cmdHandler) createStreamResp(cur *chunkStream) error {
	return c.writeMsg(cur.csid, cur.streamId, "_result", c.transactionId, nil, c.streamId)
}

func (c *cmdHandler) publishOrPlay(vs []any) error {
	cmd := &publishCmd{}
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 2 {
				// 流名称
				cmd.PubName = v.(string)
			} else if k == 3 {
				// 流类型 live、record、append
				cmd.PubType = v.(string)
			}
		case float64:
			id := int(v.(float64))
			c.transactionId = id
		case amf.Object:
		}
	}
	c.pubCmd = cmd
	return nil
}

func (c *cmdHandler) publishResp(cur *chunkStream) error {
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publishing."
	return c.writeMsg(cur.csid, cur.streamId, "onStatus", 0, nil, event)
}

func (c *cmdHandler) setBegin() error {
	ret := userControlMsg(streamBegin, 4)
	for i := 0; i < 4; i++ {
		ret.data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	return ret.writeChunk(c.conn)
}

func (c *cmdHandler) setRecorded() error {
	ret := userControlMsg(streamIsRecorded, 4)
	for i := 0; i < 4; i++ {
		ret.data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	return ret.writeChunk(c.conn)
}

func (c *cmdHandler) playResp(cur *chunkStream) error {
	if err := c.setRecorded(); err != nil {
		return err
	}
	if err := c.setBegin(); err != nil {
		return err
	}
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Play.Reset"
	event["description"] = "Playing and resetting stream."
	if err := c.writeMsg(cur.csid, cur.streamId, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Play.Start"
	event["description"] = "Started playing stream."
	if err := c.writeMsg(cur.csid, cur.streamId, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Data.Start"
	event["description"] = "Started playing stream."
	if err := c.writeMsg(cur.csid, cur.streamId, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Play.PublishNotify"
	event["description"] = "Started playing notify."
	if err := c.writeMsg(cur.csid, cur.streamId, "onStatus", 0, nil, event); err != nil {
		return err
	}
	return c.conn.Flush()
}
