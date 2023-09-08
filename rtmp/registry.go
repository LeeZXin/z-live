package rtmp

import (
	"sync"
)

/*
注册推流reader
用于分发给拉流的writer
*/
var (
	pmu          = sync.RWMutex{}
	publisherMap = make(map[string]*streamPublisher, 8)
)

// registerPublisher 注册
func registerPublisher(key string, reader *streamPublisher) {
	pmu.Lock()
	defer pmu.Unlock()
	publisherMap[key] = reader
}

// deregisterPublisher 注销
func deregisterPublisher(key string) {
	pmu.Lock()
	defer pmu.Unlock()
	delete(publisherMap, key)
}

// FindPublisher 匹配
func FindPublisher(key string) (RegisterAction, bool) {
	pmu.RLock()
	defer pmu.RUnlock()
	reader, ok := publisherMap[key]
	return reader, ok
}

type packetWriterWrapper struct {
	PacketWriter
	sendCacheOnce sync.Once
}

func newPacketWriterWrapper(writer PacketWriter) *packetWriterWrapper {
	return &packetWriterWrapper{
		PacketWriter:  writer,
		sendCacheOnce: sync.Once{},
	}
}

func (w *packetWriterWrapper) writeCache(cache *streamCache) (err error) {
	w.sendCacheOnce.Do(func() {
		err = cache.send(w.PacketWriter)
	})
	return
}

type writerRegistryHolder struct {
	closed bool
	sync.RWMutex
	numGen  int
	members map[int]*packetWriterWrapper
}

func newWriterRegistryHolder() *writerRegistryHolder {
	ret := &writerRegistryHolder{
		RWMutex: sync.RWMutex{},
		members: make(map[int]*packetWriterWrapper, 8),
	}
	return ret
}

func (r *writerRegistryHolder) register(writer *packetWriterWrapper) {
	if writer == nil {
		return
	}
	r.Lock()
	defer r.Unlock()
	if r.closed {
		return
	}
	r.numGen += 1
	r.members[r.numGen] = writer
}

func (r *writerRegistryHolder) deregister(index int) {
	r.Lock()
	defer r.Unlock()
	delete(r.members, index)
}

func (r *writerRegistryHolder) getMembers() map[int]*packetWriterWrapper {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return map[int]*packetWriterWrapper{}
	}
	ret := make(map[int]*packetWriterWrapper, len(r.members))
	for k, v := range r.members {
		ret[k] = v
	}
	return ret
}

func (r *writerRegistryHolder) closeAll() {
	r.Lock()
	defer r.Unlock()
	r.closed = true
	for _, v := range r.members {
		v.Close()
	}
}
