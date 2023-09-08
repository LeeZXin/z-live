package hls

import (
	"os"
	"sync"
)

var (
	rmu      = sync.RWMutex{}
	registry = make(map[string]*StreamWriter, 8)
)

func registerStreamWriter(writer *StreamWriter) {
	if writer == nil {
		return
	}
	rmu.Lock()
	defer rmu.Unlock()
	registry[writer.name] = writer
}

func deregisterStreamWriter(name string) {
	rmu.Lock()
	defer rmu.Unlock()
	delete(registry, name)
}

func FindStreamWriter(name string) (*StreamWriter, bool) {
	rmu.RLock()
	defer rmu.RUnlock()
	ret, ok := registry[name]
	return ret, ok
}

func GetFileContent(fileName string) []byte {
	ret, err := os.ReadFile(dirPrefix + fileName)
	if err != nil {
		return []byte{}
	}
	return ret
}
