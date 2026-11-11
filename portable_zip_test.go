package main

// 便携包的规矩：只收 Gloss.exe 和它运行时要用的原生库，不收发布流程的
// 其他产物。scripts/portable-zip.sh 是打包的那一个命令，这些测试真的
// 跑它，再用 archive/zip 读打出来的包 —— 断言落在用户解压后得到的
// 文件上，而不是去 grep workflow 的某一行（那只能证明某行字还在，
// 证明不了包是对的）。
//
// 背景见 portable-zip.sh 顶上的注释：v0.3.32 的便携包 30MB，其中 21MB
// 是安装包、更新用的 tar.gz / 增量 / 清单和已经 embed 进二进制的 PNG。

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runPortableZip 跑一次打包，返回 zip 里的文件名（不含目录）。
func runPortableZip(t *testing.T, files ...string) []string {
	t.Helper()
	stage := t.TempDir()
	for _, name := range files {
		path := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("造 %s 失败: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("造 %s 失败: %v", name, err)
		}
	}
	out := filepath.Join(t.TempDir(), "portable.zip")
	cmd := exec.Command("sh", "scripts/portable-zip.sh", stage, out)
	cmd.Dir = repoRoot(t)
	if err := cmd.Run(); err != nil {
		t.Fatalf("portable-zip.sh 失败: %v", err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("打不开 %s: %v", out, err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		if strings.Contains(f.Name, "/") {
			t.Errorf("便携包里 %q 带目录层级，解压出来该直接是文件", f.Name)
		}
		names = append(names, f.Name)
	}
	return names
}

// repoRoot 找到仓库根：测试在仓库根的包里跑，但用 -run 选中时工作目录
// 仍是仓库根，这里显式写出来，免得将来在别处跑时找不到脚本。
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("拿不到工作目录: %v", err)
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			t.Fatalf("从 %s 往上找不到 go.mod", wd)
		}
	}
}

// TestPortableZipKeepsOnlyWhatRuns 断言便携包里只有 exe 和原生库。
//
// 这份清单就是 build/windows-amd64 在一次真实发布后的样子（v0.3.32）：
// 除 Gloss.exe 和 ghostty-vt.dll 之外的九项都不该出现。
func TestPortableZipKeepsOnlyWhatRuns(t *testing.T) {
	got := runPortableZip(t,
		"Gloss.exe",
		"ghostty-vt.dll",
		// resources 里的图标：go:embed 进二进制了，磁盘上没人读。
		"icon.png",
		"logo.png",
		"logo-dark.png",
		"icon-pure-black.png",
		"icon-pure-white.png",
		// 安装包走安装那条路，便携包是它的替代。
		"Gloss Setup 0.3.32.exe",
		// 自动更新的产物：客户端只认 update-windows-amd64.json 里的 URL。
		"gloss-0.3.32-windows-amd64.tar.gz",
		"gloss-0.3.31-to-0.3.32-windows-amd64.delta",
		"update-windows-amd64.json",
	)

	want := []string{"Gloss.exe", "ghostty-vt.dll"}
	if len(got) != len(want) {
		t.Fatalf("便携包收了 %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("第 %d 项 = %q, want %q", i+1, got[i], w)
		}
	}
}

// TestPortableZipKeepsTheDLL 断言原生库跟着 exe 走：终端的 library.Find
// 依次找资源目录、exe 旁边、用户缓存，exe 旁边那份是唯一一定在的。少了
// 它，便携包一开终端就起不来。
func TestPortableZipKeepsTheDLL(t *testing.T) {
	got := runPortableZip(t, "Gloss.exe", "ghostty-vt.dll")
	want := []string{"Gloss.exe", "ghostty-vt.dll"}
	if len(got) != len(want) {
		t.Fatalf("便携包收了 %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("第 %d 项 = %q, want %q", i+1, got[i], w)
		}
	}
}

// TestPortableZipFailsWithoutAnExe 断言没有 exe 时打包失败，而不是留下
// 一个跑不起来的包。
//
// 这条守的是「别把原生库单独打成一个 zip」：Gloss.exe 才是入口，缺了它
// 这一包对用户没有意义，还会让人以为便携包可以只发一个 dll。workflow 里
// mygo build 刚跑完，exe 一定在；这里防的是将来改了脚本、把 exe 从清单
// 里漏掉。
func TestPortableZipFailsWithoutAnExe(t *testing.T) {
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "ghostty-vt.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "portable.zip")
	cmd := exec.Command("sh", "scripts/portable-zip.sh", stage, out)
	cmd.Dir = repoRoot(t)
	if err := cmd.Run(); err == nil {
		t.Error("没有 Gloss.exe 时 portable-zip.sh 该失败")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("失败了却留下了 zip")
	}
}
