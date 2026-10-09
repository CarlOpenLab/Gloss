package main

import (
	"fmt"
	"sync"
)

// Locale 代表当前界面的语言设定。
type Locale string

const (
	LocaleZh Locale = "zh" // 简体中文
	LocaleEn Locale = "en" // 英文
)

var (
	localeMu      sync.RWMutex
	currentLocale = LocaleZh
)

// SetLocale 设置全局当前界面语言。
func SetLocale(loc string) {
	localeMu.Lock()
	defer localeMu.Unlock()
	if loc == "en" {
		currentLocale = LocaleEn
	} else {
		currentLocale = LocaleZh
	}
}

// CurrentLocale 获取全局当前界面语言。
func CurrentLocale() Locale {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return currentLocale
}

// IsEnglish 判断当前是否为英文环境。
func IsEnglish() bool {
	return CurrentLocale() == LocaleEn
}

// T 提供轻量、高可读性的双语文本映射。若当前为英文返回 en，否则返回 zh。
func T(zh, en string) string {
	if IsEnglish() {
		return en
	}
	return zh
}

// Tf 提供格式化双语文本输出。
func Tf(zh, en string, args ...any) string {
	if IsEnglish() {
		return fmt.Sprintf(en, args...)
	}
	return fmt.Sprintf(zh, args...)
}
