package filesystem

import (
	"assethub/internal/domain"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Cache struct {
	Root           string
	Capacity       int64
	ReservePercent *uint64
	mu             sync.Mutex
}

func (c *Cache) path(id string, index int) (string, error) {
	if !validID(id) || index < 0 {
		return "", domain.ErrInvalid
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", id, index)))
	return filepath.Join(c.Root, fmt.Sprintf("%x.cache", sum[:])), nil
}
func (c *Cache) Get(id string, chunk domain.Chunk) ([]byte, error) {
	p, err := c.path(id, chunk.Index)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, chunk.CipherSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != chunk.CipherSize || digest(data) != chunk.SHA256 {
		_ = os.Remove(p)
		return nil, nil
	}
	return data, nil
}
func (c *Cache) Put(id string, chunk domain.Chunk, data []byte) error {
	if int64(len(data)) != chunk.CipherSize || digest(data) != chunk.SHA256 {
		return domain.ErrIntegrity
	}
	if int64(len(data)) > c.Capacity {
		return nil
	}
	p, err := c.path(id, chunk.Index)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err = os.MkdirAll(c.Root, 0700); err != nil {
		return err
	}
	existing, err := os.ReadFile(p)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		return err
	}
	type item struct {
		path string
		size int64
		time int64
	}
	list := []item{}
	var size int64
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".cache" || !entry.Type().IsRegular() {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			continue
		}
		size += info.Size()
		list = append(list, item{filepath.Join(c.Root, entry.Name()), info.Size(), info.ModTime().UnixNano()})
	}
	sort.Slice(list, func(a, b int) bool { return list[a].time < list[b].time })
	for _, entry := range list {
		if size+int64(len(data)) <= c.Capacity {
			break
		}
		if os.Remove(entry.path) == nil {
			size -= entry.size
		}
	}
	if err = checkSpace(c.Root, int64(len(data)), c.ReservePercent); err != nil {
		return err
	}
	return atomicWrite(p, data)
}
