package gitrepo

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// RefKind 是引用的种类，侧栏按它分组。
type RefKind int

const (
	RefLocal  RefKind = iota // refs/heads/*
	RefRemote                // refs/remotes/*，不含 */HEAD
	RefTag                   // refs/tags/*
	RefStash                 // stash@{n}
)

// Ref 是侧栏 refs 树里的一项。
type Ref struct {
	Kind RefKind
	// Name 是短名，用来显示、以及和提交列表里 %D 装饰出来的名字对上：
	// main、origin/main、v1.0、stash@{0}。
	Name string
	// Full 是完整引用名（refs/heads/main、refs/tags/v1.0），**喂给 Log 的是它**。
	//
	// 不能拿短名去当 revision：仓库里同时有分支 v1 和标签 v1 时，git 的消歧
	// 顺序是 refs/v1 → refs/tags/v1 → refs/heads/v1，标签赢。用户点的是分支，
	// 看到的是标签的历史，而且不报任何错。
	Full     string
	Remote   string // 远程分支所属的 remote（"origin"），别的种类留空
	Upstream string // 本地分支的上游（"origin/main"），没有或非本地分支为空
	// Ahead / Behind 是本地分支相对上游的超前/落后提交数，UpstreamGone 是
	// 上游已经在远程上被删掉（%(upstream:track) 给的是 [gone]）。
	//
	// 三个数都从同一次 for-each-ref 里带出来，不按分支各跑一次 rev-list：
	// 侧栏一列分支几十条，一次一条进程会把读引用变成几十倍慢。track 为空
	// 表示「同步」或「没有上游」—— 两者都不值得在侧栏占一行字。
	Ahead        int
	Behind       int
	UpstreamGone bool
	Hash         string // 短哈希
	Head         bool   // 当前检出的本地分支
	// Subject 只有储藏用得上：储藏名字（"On main: ..."）是全部信息，
	// 分支和标签的提交信息在提交列表里看。
	Subject string
	When    time.Time
}

// Label 是侧栏每行显示的文字：远程分支去掉 remote 前缀，分组那一层已经在
// 树里了，叶子上再来一遍就是 "origin/origin/main"。
func (r Ref) Label() string {
	if r.Kind == RefRemote && r.Remote != "" {
		return strings.TrimPrefix(r.Name, r.Remote+"/")
	}
	return r.Name
}

// Refs 读本地分支、远程分支、标签和储藏。
//
// 一次 for-each-ref 拿三个命名空间：分三次调用要多起两个进程，而且三次之间
// 仓库状态可能变，读出来的东西对不上（比如分支刚被删掉）。
func (r *Repo) Refs(ctx context.Context) ([]Ref, error) {
	out, err := run(ctx, r.Root, "for-each-ref",
		"--sort=-committerdate",
		"--format=%(refname)%00%(objectname:short)%00%(HEAD)%00%(symref)%00%(committerdate:unix)%00%(upstream:short)%00%(upstream:track)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	refs := parseRefs(string(out))

	// 储藏另走一条命令：它不在 refs/ 下面，for-each-ref 看不见
	//（refs/stash 只有栈顶那一条，历史全躺在它的 reflog 里）。
	// 没有储藏时这条命令是空输出 + 退出码 0，不用特判。
	//
	// 读失败只当没有储藏，不往上抛：分支和标签才是侧栏的主功能，
	// 一条储藏读不出来不该让整棵树变成「读不到分支」。
	if stashes, err := run(ctx, r.Root, "stash", "list", "--format=%gd%x00%H%x00%s"); err == nil {
		refs = append(refs, parseStashes(string(stashes))...)
	}
	return refs, nil
}

func parseRefs(out string) []Ref {
	var refs []Ref
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) < 5 {
			continue
		}
		full := f[0]

		// 符号引用要跳过。克隆下来的仓库里 refs/remotes/origin/HEAD 指向
		// origin 的默认分支，它不是一条真的分支线 —— 列出来会多一条和
		// 别的分支重复的条目，画到拓扑图上还会和它指向的那条撞色。
		// %(symref) 非空就是符号引用；再按名字兜一道，各版本 git 对
		// %(symref) 的处理略有出入。
		if f[3] != "" || strings.HasSuffix(full, "/HEAD") {
			continue
		}

		var ref Ref
		switch {
		case strings.HasPrefix(full, "refs/heads/"):
			ref = Ref{Kind: RefLocal, Name: strings.TrimPrefix(full, "refs/heads/")}
		case strings.HasPrefix(full, "refs/remotes/"):
			name := strings.TrimPrefix(full, "refs/remotes/")
			remote, _, ok := strings.Cut(name, "/")
			if !ok {
				continue // 光秃秃一个 remote 名，不是分支
			}
			ref = Ref{Kind: RefRemote, Name: name, Remote: remote}
		case strings.HasPrefix(full, "refs/tags/"):
			ref = Ref{Kind: RefTag, Name: strings.TrimPrefix(full, "refs/tags/")}
		default:
			continue
		}

		ref.Full = full
		ref.Hash = f[1]
		ref.Head = strings.TrimSpace(f[2]) == "*"
		ref.When = unixTime(f[4])
		if len(f) > 5 {
			ref.Upstream = strings.TrimSpace(f[5])
		}
		if len(f) > 6 {
			ref.Ahead, ref.Behind, ref.UpstreamGone = parseTrack(f[6])
		}
		refs = append(refs, ref)
	}
	return refs
}

// parseTrack 把 %(upstream:track) 的输出拆成超前/落后计数。
//
// 形如 "[ahead 1, behind 2]"；同步和没有上游都是空串；上游被删掉是
// "[gone]"。认不出来的写法（git 换格式）一律当「没有可说」—— 侧栏那两
// 个小数字不显示就是了，不该让整份引用读失败。
func parseTrack(s string) (ahead, behind int, gone bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if strings.Contains(s, "gone") {
		return 0, 0, true
	}
	for _, part := range strings.Split(s, ",") {
		f := strings.Fields(strings.Trim(part, "[] "))
		if len(f) != 2 {
			continue
		}
		n, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		switch f[0] {
		case "ahead":
			ahead = n
		case "behind":
			behind = n
		}
	}
	return ahead, behind, false
}

func parseStashes(out string) []Ref {
	var refs []Ref
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) < 3 {
			continue
		}
		// 储藏的 Full 就是 stash@{n}：它本来就在 reflog 的命名空间里，
		// 不存在重名，也不会和分支/标签撞。
		refs = append(refs, Ref{
			Kind: RefStash, Name: parts[0], Full: parts[0],
			Hash: parts[1], Subject: parts[2],
		})
	}
	return refs
}

func unixTime(s string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
