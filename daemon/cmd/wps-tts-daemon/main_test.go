//go:build linux

package main

import "testing"

func TestPreprocessFanchenTextSpeaksAsciiCharacters(t *testing.T) {
	got := preprocessFanchenText("深度学习是AI的核心技术，使用Python 3.11进行WPS开发。")
	want := "深度学习是 诶 爱 的核心技术，使用 批 歪 提 艾尺 欧 恩 三 点 一 一 进行 达不溜 批 艾丝 开发。"
	if got != want {
		t.Fatalf("preprocessed text = %q, want %q", got, want)
	}
}
