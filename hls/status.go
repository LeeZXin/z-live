package hls

// status 转换过程中 数据保存
type status struct {
	hasSetFirstTs  bool
	firstTimestamp int64
	lastTimestamp  int64
}

func newStatus() *status {
	return &status{}
}

func (t *status) update(timestamp uint32) {
	if !t.hasSetFirstTs {
		t.hasSetFirstTs = true
		t.firstTimestamp = int64(timestamp)
	}
	t.lastTimestamp = int64(timestamp)
}

func (t *status) resetAndNew() {
	t.firstTimestamp = 0
	t.lastTimestamp = 0
	t.hasSetFirstTs = false
}

func (t *status) durationMs() int64 {
	return t.lastTimestamp - t.firstTimestamp
}
