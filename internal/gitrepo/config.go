package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git 的配置分三层，名字照抄 git 自己的叫法 —— 命令行里怎么写，这里就怎么叫：
//
//   - system：机器上所有用户（/etc/gitconfig 之类）
//   - global：当前用户的每一个仓库（~/.gitconfig）
//   - local：只此一个仓库（.git/config）
//
// 三层同名时后面的覆盖前面的（local > global > system），这也是界面把它们
// 摆在一起的理由：单看一层说不清「这条到底生效没有」。
const (
	ScopeSystem = "system"
	ScopeGlobal = "global"
	ScopeLocal  = "local"
)

// ConfigEntry 是一条配置。
type ConfigEntry struct {
	Key   string
	Value string
	// Scope 是这一条所在的层级。除了上面三个，git 还会报 unknown ——
	// macOS 上 /Applications/Xcode.app/…/gitconfig 这类厂商塞进来的文件
	// 就走这一档。不认识的层级原样留着，由界面决定归到哪一栏。
	Scope string
	// File 是这条配置来自哪个文件（已经去掉 file: 前缀的绝对路径）。
	// 很老的 git 不报来源，可能是空的。
	File string
}

// ReadConfig 用一次 git 进程读回三层的全部配置。
//
// 不分三次问：这些条目在界面上是一张表，而分三次读就有三个时间点 ——
// 读的时候别人正在改配置，表里就会出现互相矛盾的条目。
//
// 需要 git ≥ 2.26（--show-scope）。缺了它这里的错误会原样浮到界面上，
// 不静默降级：一份读不全的配置比一句「你的 git 太老」危险得多。
func ReadConfig(ctx context.Context, dir string) ([]ConfigEntry, error) {
	out, _, err := runConfig(ctx, dir, "config", "--list", "--show-scope", "--show-origin", "--null")
	if err != nil {
		return nil, err
	}
	entries := parseConfigList(out)
	// 项目级报的是走到哪儿算哪儿的路子（cwd 在仓库里时是 ".git/config"），
	// 别的层级是绝对路径。统一接成绝对路径 —— 界面要把它显示给人看，
	// 而「.git/config」在设置面板里指不到任何东西。
	for i, e := range entries {
		if e.File == "" || filepath.IsAbs(e.File) {
			continue
		}
		base := dir
		if base == "" {
			base, _ = os.Getwd()
		}
		entries[i].File = filepath.Join(base, e.File)
	}
	return entries, nil
}

// parseConfigList 解析 `--list --show-scope --show-origin --null` 的输出。
//
// 一条记录是三个 NUL 分隔的字段：层级、来源文件、键 + 换行 + 值。键里不会
// 有换行，值里可以有（多行值就是这么存的），所以按第一个换行切是准的。
func parseConfigList(out []byte) []ConfigEntry {
	fields := bytes.Split(out, []byte{0})
	entries := make([]ConfigEntry, 0, len(fields)/3+1)
	for i := 0; i+2 < len(fields); i += 3 {
		kv := fields[i+2]
		if len(kv) == 0 {
			continue
		}
		key, value, _ := bytes.Cut(kv, []byte("\n"))
		if len(key) == 0 {
			continue
		}
		entries = append(entries, ConfigEntry{
			Key:   string(key),
			Value: string(value),
			Scope: string(fields[i]),
			File:  strings.TrimPrefix(string(fields[i+1]), "file:"),
		})
	}
	return entries
}

// WriteConfig 写入一条配置：同名的被替换成这一个值，本来没有就新增。
//
// 用 --replace-all 而不是默认的「只改头一条」：界面上一个键只有一行，存回去
// 也该只剩一条 —— 否则改完再看，行数凭空变多（url.*.insteadof 这类天然多值
// 的键就会这样）。
//
// 键值前面钉一个 `--`：git 的选项解析在它之后收手，值是以「-」开头的
// （`core.pager = -R` 这种）也不会被当成选项吃掉。
func WriteConfig(ctx context.Context, dir, scope, key, value string) error {
	if err := checkConfigKey(key); err != nil {
		return err
	}
	_, _, err := runConfig(ctx, dir, "config", "--"+scope, "--replace-all", "--", key, value)
	return err
}

// UnsetConfig 删掉一条配置（同名的多条一起删）。
//
// 键本来就不在不算错：界面上的「删除」记的是「这一行别留下」，而那条记录
// 可能已经被别处删掉了 —— 幂等才拦得住这种竞态。
func UnsetConfig(ctx context.Context, dir, scope, key string) error {
	if err := checkConfigKey(key); err != nil {
		return err
	}
	_, code, err := runConfig(ctx, dir, "config", "--"+scope, "--unset-all", "--", key)
	if err != nil && code != exitConfigKeyMissing {
		return err
	}
	return nil
}

// exitConfigKeyMissing 是 `git config --unset` 在键不存在时的退出码。
const exitConfigKeyMissing = 5

// checkConfigKey 挡掉会跑偏的键名。
//
// git 自己会校验键名的形状（section[.subsection].name），这里挡的是另一类：
// 空键名什么都没写，而以「-」开头的键名会被 git 的选项解析当成选项 ——
// `--replace-all` 前面那个 `--` 只护得住它后面的第一个位置，护不住
// 「键名 = --file」这种写法。
func checkConfigKey(key string) error {
	switch {
	case key == "":
		return errors.New("配置项名不能为空")
	case strings.HasPrefix(key, "-"):
		return fmt.Errorf("配置项名不能以 - 开头：%s", key)
	case strings.ContainsAny(key, " \t\n\v\f\r"):
		return fmt.Errorf("配置项名不能含空白：%s", key)
	case strings.Contains(key, "="):
		return fmt.Errorf("配置项名不能含 =：%s", key)
	}
	return nil
}

// runConfig 跑一条 git config，把退出码一并带回来。
//
// 为什么非要有退出码：`--unset` 在键不存在时退出码是 5、且**一句话都不打**，
// 光看错误信息分不出「键不在」（删干净了，算成功）和真出错（得让用户看见）。
func runConfig(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
	cmd := newGitCmd(ctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, -1, ctx.Err()
		}
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, code, errors.New(msg)
	}
	return stdout.Bytes(), 0, nil
}
