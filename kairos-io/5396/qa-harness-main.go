// Throwaway QA harness (not part of the PR): calls partitions.GetEfiPartition.
package main

import (
	"fmt"

	"github.com/kairos-io/kairos/v4/agent/pkg/utils/partitions"
	"github.com/kairos-io/kairos/v4/sdk/types/logger"
)

func main() {
	l := logger.NewKairosLogger("qa5397", "debug", false)
	p, err := partitions.GetEfiPartition(&l)
	if err != nil {
		fmt.Println("RESULT: error:", err)
		return
	}
	fmt.Printf("RESULT: chosen=%s mountpoint=%q label=%s\n", p.Path, p.MountPoint, p.FilesystemLabel)
}
