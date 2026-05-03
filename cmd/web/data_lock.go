package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const dataLockFileName = "agenthub.lock"

// dataDirLock 表示当前进程持有的数据目录锁。
type dataDirLock struct {
	file *os.File // file 表示持有文件锁的文件句柄。
}

// acquireDataDirLock 使用 ctx 和 dataDir 参数创建并持有数据目录锁。
func acquireDataDirLock(ctx context.Context, dataDir string) (*dataDirLock, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	lockPath := filepath.Join(dataDir, dataLockFileName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开数据目录锁失败: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("AGENTHUB_DATA 已被其他进程使用: %s", dataDir)
		}
		return nil, fmt.Errorf("持有数据目录锁失败: %w", err)
	}
	if err := writeDataDirLockInfo(file, dataDir); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	log.Ctx(ctx).Info().Str("dataDir", dataDir).Str("lockPath", lockPath).Msg("已持有数据目录锁")
	return &dataDirLock{file: file}, nil
}

// Release 释放当前进程持有的数据目录锁。
func (l *dataDirLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		log.Warn().Err(err).Msg("释放数据目录锁失败")
	}
	if err := l.file.Close(); err != nil {
		log.Warn().Err(err).Msg("关闭数据目录锁文件失败")
	}
	l.file = nil
}

// writeDataDirLockInfo 使用 file 和 dataDir 参数写入当前进程锁信息。
func writeDataDirLockInfo(file *os.File, dataDir string) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("清空数据目录锁文件失败: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("定位数据目录锁文件失败: %w", err)
	}
	content := fmt.Sprintf("pid=%d\nstartedAt=%s\ndataDir=%s\n", os.Getpid(), time.Now().Format(time.RFC3339), dataDir)
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("写入数据目录锁文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步数据目录锁文件失败: %w", err)
	}
	return nil
}
