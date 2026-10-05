package main

import (
	"io/fs"
)

// webDistSub 返回嵌入的 web/dist 子文件系统。
func webDistSub() (fs.FS, error) {
	return fs.Sub(webDist, "web/dist")
}
