//go:build !windows

package filesystem

import (
	"assethub/internal/domain"
	"golang.org/x/sys/unix"
	"os"
)

func checkSpace(path string, needed int64, percent *uint64) error {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return err
	}
	available := uint64(s.Bavail) * uint64(s.Bsize)
	reserve := uint64(s.Blocks) * uint64(s.Bsize) * reservePercent(percent) / 100
	if uint64(needed) > available || available-uint64(needed) < reserve {
		return domain.ErrCapacity
	}
	return nil
}
func sameVolume(a, b string) (bool, error) {
	var x, y unix.Stat_t
	if err := unix.Stat(a, &x); err != nil {
		return false, err
	}
	if err := unix.Stat(b, &y); err != nil {
		return false, err
	}
	return x.Dev == y.Dev, nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func diskStats(path string) (uint64, uint64, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return 0, 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), uint64(s.Blocks) * uint64(s.Bsize), nil
}
