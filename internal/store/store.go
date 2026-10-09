// Package store 把用户设置、界面状态和 AI 解释缓存落到磁盘。
//
// 目录由调用方给（桌面包传 mygo 的用户数据目录），所以这个包不依赖 mygo，
// 测试里给个 t.TempDir() 就能完整跑。
//
// 三条约定：
//   - 读文件失败（第一次启动没有文件、JSON 损坏）一律当作空，绝不因此
//     让应用起不来 —— 配置文件坏了可以重建，应用打不开就全完了。
//   - 写入先落临时文件再 rename：中途崩掉也不会留下半个 JSON。
//   - 有 API key，config.json 用 0600。
package store

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// maxCacheEntries 是解释缓存的条数上限。每条几百字节，500 条足够覆盖
// 常见的 review 场景，又不至于让 cache.json 无限膨胀。
const maxCacheEntries = 500

// Config 是用户设置。
type Config struct {
	Providers []llm.Provider `json:"providers,omitempty"`
	// Active 是当前使用的供应商下标。
	Active int `json:"active,omitempty"`
	// CodeSize 是代码字号，0 表示用默认值。
	CodeSize float32 `json:"codeSize,omitempty"`
	// Split 为 true 时用左右对照视图。
	Split bool `json:"split,omitempty"`
	// Unified 为 true 时用单栏统一视图（默认左右对照）。
	Unified bool `json:"unified,omitempty"`
	// SidebarOff 而不是 SidebarOn：侧栏默认是开的，零值正好表示这个默认。
	SidebarOff bool `json:"sidebarOff,omitempty"`
	// SidebarWidth 是侧栏宽度，0 表示默认值。
	SidebarWidth float32 `json:"sidebarWidth,omitempty"`
	// FilePaneWidth 是主区文件列表栏宽度，0 表示默认值。
	FilePaneWidth float32 `json:"filePaneWidth,omitempty"`
	// ThemeMode 是外观模式偏好："" 或 "system"（跟随系统）、"light"（浅色模式）、"dark"（暗黑模式）。
	ThemeMode string `json:"themeMode,omitempty"`
	// Locale 是界面语言设定："" 或 "zh"（简体中文）、"en"（English）。
	Locale string `json:"locale,omitempty"`
	// AppIcon 是应用图标风格："black"（纯黑曜石）或 "white"（纯白极简）。
	AppIcon string `json:"appIcon,omitempty"`
}

// Provider 是当前选中的供应商配置。没有配置任何供应商时返回 nil，
// 此时 StreamExplanation 会走 Anthropic 的环境变量凭证链。
func (c Config) Provider() *llm.Provider {
	if len(c.Providers) == 0 {
		return nil
	}
	i := c.Active
	if i < 0 || i >= len(c.Providers) {
		i = 0
	}
	p := c.Providers[i]
	return &p
}

// State 是跟着仓库走的界面状态。
type State struct {
	LastRepo    string            `json:"lastRepo,omitempty"`
	RecentRepos []string          `json:"recentRepos,omitempty"`
	// Viewed 是「已 review」标记：文件路径 -> 当时的内容指纹。
	Viewed map[string]string `json:"viewed,omitempty"`
	// Drafts 是各仓库的提交信息草稿：仓库根路径 -> 还没提交出去的消息。
	// 消息写到一半切去看别的仓库、或者关掉窗口，回来还应该在。
	Drafts map[string]string `json:"drafts,omitempty"`
}

// cacheEntry 是一条解释缓存。At 用于超出上限时淘汰最旧的。
type cacheEntry struct {
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// Store 是三个文件的门面，可被多个 goroutine 同时使用。
type Store struct {
	dir string

	mu    sync.Mutex
	cfg   Config
	st    State
	cache map[string]cacheEntry
}

// Open 从 dir 读取已有的设置、状态和缓存。文件不存在或读不动都当作空，
// 返回值不会是 nil。dir 为空表示「没有数据目录」：读写都变成空操作，
// 应用照常能跑，只是这次的改动不会被保存。
func Open(dir string) *Store {
	s := &Store{dir: dir, cache: map[string]cacheEntry{}}
	if dir == "" {
		return s
	}
	readJSON(filepath.Join(dir, "config.json"), &s.cfg)
	readJSON(filepath.Join(dir, "state.json"), &s.st)
	// 缓存文件换过一次结构（早期是 map[string]string），读不动就当空。
	var c struct {
		Entries map[string]cacheEntry `json:"entries"`
	}
	readJSON(filepath.Join(dir, "cache.json"), &c)
	if c.Entries != nil {
		s.cache = c.Entries
	}
	return s
}

// Dir 是这个 store 的数据目录。
func (s *Store) Dir() string { return s.dir }

// Config 返回设置的副本。
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetConfig 替换设置。
func (s *Store) SetConfig(c Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = c
}

// Provider 是当前选中的供应商，没配置就是 nil（走环境变量凭证链）。
func (s *Store) Provider() *llm.Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Provider()
}

// State 返回状态，Viewed 和 Drafts 都是新的一份 map，改它们不会影响 store。
func (s *Store) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := s.st
	cp.Viewed = make(map[string]string, len(s.st.Viewed))
	for k, v := range s.st.Viewed {
		cp.Viewed[k] = v
	}
	cp.Drafts = make(map[string]string, len(s.st.Drafts))
	for k, v := range s.st.Drafts {
		cp.Drafts[k] = v
	}
	return cp
}

// SetViewed 替换「已 review」标记。
func (s *Store) SetViewed(m map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Viewed = make(map[string]string, len(m))
	for k, v := range m {
		s.st.Viewed[k] = v
	}
}

// SetDraft 记下某个仓库的提交信息草稿。msg 为空就把这条删掉 ——
// 空草稿留在文件里没有任何用处。
func (s *Store) SetDraft(repo, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Drafts == nil {
		s.st.Drafts = map[string]string{}
	}
	if msg == "" {
		delete(s.st.Drafts, repo)
		return
	}
	s.st.Drafts[repo] = msg
}

const maxRecentRepos = 10

// AddRecentRepo 将 path 记为最近打开的仓库（最新的在最前），并更新 LastRepo。
func (s *Store) AddRecentRepo(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.LastRepo = path
	var updated []string
	updated = append(updated, path)
	for _, p := range s.st.RecentRepos {
		if p != path && p != "" {
			updated = append(updated, p)
		}
		if len(updated) >= maxRecentRepos {
			break
		}
	}
	s.st.RecentRepos = updated
}

// ClearRecentRepos 清空最近打开的仓库列表。
func (s *Store) ClearRecentRepos() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.RecentRepos = nil
}

// SetLastRepo 记住最后打开的仓库，下次启动直接打开。
func (s *Store) SetLastRepo(path string) {
	if path == "" {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.st.LastRepo = ""
		return
	}
	s.AddRecentRepo(path)
}

// Lookup 取一条解释缓存。
func (s *Store) Lookup(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	return e.Text, ok
}

// Put 写入一条解释缓存。
func (s *Store) Put(key, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cacheEntry{Text: text, At: time.Now().Unix()}
}

// Save 把三个文件写回磁盘。
//
// 用法是「改完就存」：文件都很小，写一次在毫秒级，不值得为它做脏标记或者
// 防抖。整段加锁，所以并发调用是安全的（后一次写覆盖前一次，结果一致）。
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.prune()
	if s.dir == "" {
		return nil // 没有数据目录，没什么可存的
	}

	st := s.st
	if st.Viewed == nil {
		st.Viewed = map[string]string{}
	}
	entries := map[string]cacheEntry{}
	for k, v := range s.cache {
		entries[k] = v
	}

	if err := writeJSON(filepath.Join(s.dir, "config.json"), s.cfg, 0o600); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "state.json"), st, 0o644); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.dir, "cache.json"),
		struct {
			Entries map[string]cacheEntry `json:"entries"`
		}{entries}, 0o644)
}

// prune 在超过上限时丢掉最旧的缓存。调用方要持锁。
func (s *Store) prune() {
	if len(s.cache) <= maxCacheEntries {
		return
	}
	type kv struct {
		k  string
		at int64
	}
	all := make([]kv, 0, len(s.cache))
	for k, v := range s.cache {
		all = append(all, kv{k, v.At})
	}
	// 新的排前面，砍掉末尾
	sort.Slice(all, func(i, j int) bool { return all[i].at > all[j].at })
	for _, e := range all[maxCacheEntries:] {
		delete(s.cache, e.k)
	}
}

// readJSON 读一个 JSON 文件。任何失败都只是「没有这份配置」，不是错误。
func readJSON(path string, v any) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, v)
}

// writeJSON 原子地写一个 JSON 文件：先写同目录下的临时文件，再 rename 过去。
// 直接截断原文件的话，写到一半断电就只剩半截 JSON 了。
func writeJSON(path string, v any, perm fs.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // rename 成功后这里会失败，无害

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
