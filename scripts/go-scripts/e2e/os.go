package e2e

import "os"

// osStat 使用 path 参数读取文件状态，集中封装便于用例表达。
func osStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
