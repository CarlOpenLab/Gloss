// Package explain 把提交、分支、PR 和风险扫描要问模型的东西拼成提示词。
//
// 这里全是纯函数：不碰网络、不碰界面、不碰磁盘，所以没有 API key
// 也能完整测。
package explain

import "github.com/CarlOpenLab/Gloss/internal/diff"

// Path 是这个变更块所属的文件路径。
func Path(f diff.FileDiff) string { return f.Path() }

// DisplayPath 在重命名时把两个路径都写出来。
func DisplayPath(f diff.FileDiff) string {
	if f.Status == "renamed" && f.OldPath != "" && f.NewPath != "" {
		return f.OldPath + " → " + f.NewPath
	}
	return Path(f)
}

func statusText(s string) string {
	switch s {
	case "added":
		return "新增"
	case "deleted":
		return "删除"
	case "renamed":
		return "重命名"
	}
	return "修改"
}
