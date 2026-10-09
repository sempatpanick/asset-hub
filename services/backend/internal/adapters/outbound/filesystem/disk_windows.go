package filesystem

import (
	"assethub/internal/domain"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
)

func checkSpace(path string, needed int64, percent *uint64) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var available, total, free uint64
	if err = windows.GetDiskFreeSpaceEx(p, &available, &total, &free); err != nil {
		return err
	}
	if uint64(needed) > available || available-uint64(needed) < total*reservePercent(percent)/100 {
		return domain.ErrCapacity
	}
	return nil
}
func sameVolume(a, b string) (bool, error) {
	x, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	y, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(filepath.VolumeName(x), filepath.VolumeName(y)), nil
}
func syncDirectory(string) error { return nil } // Directory fsync is verified on the Linux deployment target.

func diskStats(path string) (uint64, uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available, total, free uint64
	err = windows.GetDiskFreeSpaceEx(p, &available, &total, &free)
	return available, total, err
}
