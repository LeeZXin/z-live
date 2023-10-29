package hls

import (
	"bytes"
	"fmt"
	"github.com/LeeZXin/zsf-utils/cryptoutil"
	"github.com/LeeZXin/zsf/logger"
	"os"
	"sync"
	"time"
)

const (
	maxTsCacheNum = 10
	dirPrefix     = "./hlstmp/"

	encryptFlag = true
)

var (
	EncryptAesKey = []byte("zxHBRIg81deD3VAJ")
)

/*
m3u8,ts 内存缓存
*/
var (
	ErrNoKey   = fmt.Errorf("no key for audioCache")
	tsItemPool sync.Pool
)

func init() {
	tsItemPool = sync.Pool{
		New: func() any {
			return newTsItem()
		},
	}
}

func NewTsItem() TsItem {
	return tsItemPool.Get().(TsItem)
}

func PutTsItem(item TsItem) {
	item.Reset()
	tsItemPool.Put(item)
}

type TsCache struct {
	lock         sync.RWMutex
	index        int
	key          string
	name         string
	m3u8Path     string
	tsPathPrefix string
	itemList     []string
	itemMap      map[string]TsItem
	util         cryptoutil.Crypto
}

func NewTsCache(app, name string) *TsCache {
	cbc, err := cryptoutil.NewAesCBC(EncryptAesKey)
	if err != nil {
		logger.Logger.Panic(err)
	}
	key := app + "/" + name
	ret := &TsCache{
		util:         cbc,
		index:        -1,
		key:          key,
		name:         name,
		m3u8Path:     dirPrefix + key + "/" + name + ".m3u8",
		tsPathPrefix: dirPrefix + key + "/",
		lock:         sync.RWMutex{},
		itemList:     make([]string, maxTsCacheNum),
		itemMap:      make(map[string]TsItem),
	}
	if SaveFileFlag {
		os.MkdirAll(dirPrefix+key, os.ModePerm)
	}
	return ret
}

func (t *TsCache) GenM3U8PlayList() []byte {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.genM3U8PlayList()
}

func (t *TsCache) genM3U8PlayList() []byte {
	var seq int
	var getSeq bool
	var maxDuration int
	ret := bytes.NewBuffer(nil)
	if t.itemList[maxTsCacheNum-1] != "" {
		for i := t.index + 1; i < maxTsCacheNum; i++ {
			v, ok := t.itemMap[t.itemList[i]]
			if ok {
				if v.Duration > maxDuration {
					maxDuration = v.Duration
				}
				if !getSeq {
					getSeq = true
					seq = v.SeqNum
				}
				fmt.Fprintf(ret, "#EXTINF:%.3f,\n%s\n", float64(v.Duration)/float64(1000), v.Name)
			}
		}
	}
	for i := 0; i <= t.index; i++ {
		v, ok := t.itemMap[t.itemList[i]]
		if ok {
			if v.Duration > maxDuration {
				maxDuration = v.Duration
			}
			if !getSeq {
				getSeq = true
				seq = v.SeqNum
			}
			fmt.Fprintf(ret, "#EXTINF:%.3f,\n%s\n", float64(v.Duration)/float64(1000), v.Name)
		}
	}
	w := bytes.NewBuffer(nil)
	fmt.Fprintf(w,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n\n",
		maxDuration/1000+1,
		seq)
	if encryptFlag {
		fmt.Fprintf(w,
			"#EXT-X-KEY:METHOD=AES-128,URI=\"%s\"\n",
			fmt.Sprintf("/key?name=%s", t.name),
		)
	}
	w.Write(ret.Bytes())
	return w.Bytes()
}

func (t *TsCache) SetItem(duration, seqNum int, b []byte) {
	// /live/movie/168923452.ts
	tsName := fmt.Sprintf("/%s/%d.ts", t.key, time.Now().UnixMilli())
	t.lock.Lock()
	defer t.lock.Unlock()
	t.index = (t.index + 1) % maxTsCacheNum
	k := t.itemList[t.index]
	if n, has := t.itemMap[k]; has {
		PutTsItem(n)
		delete(t.itemMap, k)
	}
	item := NewTsItem()
	if encryptFlag {
		item.Set(tsName, duration, seqNum, t.encryptTs(b))
	} else {
		item.Set(tsName, duration, seqNum, b)
	}
	t.itemMap[tsName] = item
	t.itemList[t.index] = tsName
	if SaveFileFlag {
		// save m3u8
		t.saveFileContent(t.m3u8Path, t.genM3U8PlayList())
		// save ts
		if encryptFlag {
			t.saveFileContent(dirPrefix+tsName, t.encryptTs(b))
		} else {
			t.saveFileContent(dirPrefix+tsName, b)
		}
	}
}

func (t *TsCache) encryptTs(b []byte) []byte {
	ret, _ := t.util.Encrypt(b)
	return ret
}

func (t *TsCache) saveFileContent(fileName string, content []byte) error {
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	if err != nil {
		fmt.Println(err.Error())
	}
	return err
}

func (t *TsCache) GetItem(key string) ([]byte, error) {
	t.lock.RLock()
	defer t.lock.RUnlock()
	item, ok := t.itemMap[key]
	if !ok {
		return nil, ErrNoKey
	}
	return item.Data.Bytes(), nil
}

type TsItem struct {
	Name     string
	SeqNum   int
	Duration int
	Data     *bytes.Buffer
}

func (t *TsItem) Set(name string, duration, seqNum int, b []byte) {
	t.Name = name
	t.SeqNum = seqNum
	t.Duration = duration
	t.Data.Write(b)
}

func (t *TsItem) Reset() {
	t.Data.Reset()
}

func newTsItem() TsItem {
	item := TsItem{}
	item.Data = bytes.NewBuffer(nil)
	return item
}
