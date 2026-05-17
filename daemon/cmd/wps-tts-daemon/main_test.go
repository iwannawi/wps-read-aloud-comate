//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestPreprocessFanchenTextSpeaksAsciiCharacters(t *testing.T) {
	got := preprocessFanchenText("深度学习是AI的核心技术，使用Python 3.11进行WPS开发。", 1.2)
	want := "深度学习是 诶 爱 的核心技术， 使用 批 歪 提 艾尺 欧 恩 三 点 一 一 进行 达不溜 批 艾丝 开发。"
	if got != want {
		t.Fatalf("preprocessed text = %q, want %q", got, want)
	}
}

func TestPreprocessFanchenTextAddsPauseOnlyForSemanticPunctuation(t *testing.T) {
	got := preprocessFanchenText("他说：“你好，WPS！”《标题》继续。", 1.2)
	want := "他说： “你好， 达不溜 批 艾丝！ ”《标题》继续。"
	if got != want {
		t.Fatalf("preprocessed text = %q, want %q", got, want)
	}
}

func TestSilencePCMUsesExpectedDuration(t *testing.T) {
	wav := wavPCM{channels: 1, sampleRate: 16000, bitsPerSample: 16}
	got := silencePCM(wav, sentenceEndPauseMs(1.2))
	want := 16000 * 2 * sentenceEndPauseMsAtBaseRate / 1000
	if len(got) != want {
		t.Fatalf("silence length = %d, want %d", len(got), want)
	}
}

func TestPauseDurationsScaleWithRate(t *testing.T) {
	tests := []struct {
		rate              float64
		wantStandardMs    int
		wantSentenceEndMs int
	}{
		{rate: 1.2, wantStandardMs: 400, wantSentenceEndMs: 600},
		{rate: 0.75, wantStandardMs: 640, wantSentenceEndMs: 960},
		{rate: 1.0, wantStandardMs: 480, wantSentenceEndMs: 720},
		{rate: 1.5, wantStandardMs: 320, wantSentenceEndMs: 480},
	}
	for _, tt := range tests {
		if got := standardPauseMs(tt.rate); got != tt.wantStandardMs {
			t.Fatalf("standardPauseMs(%v) = %d, want %d", tt.rate, got, tt.wantStandardMs)
		}
		if got := sentenceEndPauseMs(tt.rate); got != tt.wantSentenceEndMs {
			t.Fatalf("sentenceEndPauseMs(%v) = %d, want %d", tt.rate, got, tt.wantSentenceEndMs)
		}
	}
}

func TestPrefetchCountUsesDynamicTextWindow(t *testing.T) {
	rs := &readSession{sentences: []ReadSentence{
		{Text: strings.Repeat("一", 101)},
		{Text: "第二句"},
		{Text: "第三句"},
	}}
	if got := rs.prefetchCount(0); got != 1 {
		t.Fatalf("prefetchCount for long first sentence = %d, want 1", got)
	}

	rs.sentences = []ReadSentence{
		{Text: strings.Repeat("一", 30)},
		{Text: strings.Repeat("二", 30)},
		{Text: strings.Repeat("三", 30)},
		{Text: strings.Repeat("四", 30)},
	}
	if got := rs.prefetchCount(0); got != 4 {
		t.Fatalf("prefetchCount for short sentences = %d, want 4", got)
	}
}
