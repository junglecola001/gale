package fsutil

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// FreeSpace returns how many bytes are free on the volume that holds dir,
// for the user running the app.
func FreeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(LongPath(dir))
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, fmt.Errorf("fsutil: free space of %q: %w", dir, err)
	}
	return free, nil
}
