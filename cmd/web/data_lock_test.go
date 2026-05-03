package main

import (
	"context"
	"testing"
)

// TestAcquireDataDirLockExclusive 验证同一数据目录只能被一个进程持有。
func TestAcquireDataDirLockExclusive(t *testing.T) {
	dataDir := t.TempDir()
	first, err := acquireDataDirLock(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("首次持有数据目录锁失败: %v", err)
	}
	defer first.Release()

	if second, err := acquireDataDirLock(context.Background(), dataDir); err == nil {
		second.Release()
		t.Fatalf("重复持有同一数据目录锁应失败")
	}

	first.Release()
	if third, err := acquireDataDirLock(context.Background(), dataDir); err != nil {
		t.Fatalf("释放后应能重新持有数据目录锁: %v", err)
	} else {
		third.Release()
	}
}
