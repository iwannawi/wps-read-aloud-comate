package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

type Config struct {
	Listen string
	Sherpa SherpaConfig
}

type SherpaConfig struct {
	Bin              string
	NumThreads       int
	TargetSampleRate int
	VitsModel        string
	VitsLexicon      string
	VitsTokens       string
	VitsRuleFsts     string
	VitsSpeakerID    int
}

type ReadSentence struct {
	Text string `json:"text"`
}

type ReadStartRequest struct {
	Sentences []ReadSentence `json:"sentences"`
	Rate      float64        `json:"rate"`
	Prefetch  int            `json:"prefetch"`
}

type Server struct {
	root     string
	cfg      Config
	mu       sync.Mutex
	session  *Session
	current  map[*exec.Cmd]bool
	synthSem chan struct{}
	httpSrv  *http.Server
	stopOnce sync.Once
}

type Session struct {
	id        string
	server    *Server
	ctx       context.Context
	cancel    context.CancelFunc
	sentences []ReadSentence
	rate      float64

	mu      sync.Mutex
	state   string
	message string
	index   int
	total   int
	cache   map[int]*audioCacheEntry
}

type audioCacheEntry struct {
	index int
	path  string
	err   error
	ready chan struct{}
}

const (
	prefetchTextTarget           = 240
	prefetchSentenceLimit        = 8
	synthConcurrencyLimit        = 3
	pauseBaseRate                = 1.2
	standardPauseMsAtBaseRate    = 400
	sentenceEndPauseMsAtBaseRate = 600
	maxReadRequestBytes          = 64 << 20
	maxReadSentences             = 20000
	maxReadTextRunes             = 2000000
	maxSentenceRunes             = 1000
)

var asciiTokenRE = regexp.MustCompile(`[A-Za-z0-9]+(?:[._+\-][A-Za-z0-9]+)*`)
var percentValueRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*[%％]`)
var specialEnglishTokenRE = regexp.MustCompile(`(?i)\b(WPS|Office)\b`)
var tocLeaderPageRE = regexp.MustCompile(`[.·•…⋯・．\s]{2,}([0-9]+(?:[-–—][0-9]+)?)\s*$`)
var purePageNumberRE = regexp.MustCompile(`^\s*[0-9]+(?:[-–—][0-9]+)?\s*$`)
var wordTocFieldRE = regexp.MustCompile(`(?i)\bTOC\b(?:\s+\\[A-Za-z]+(?:\s+"[^"]*")?)*`)
var winmm = syscall.NewLazyDLL("winmm.dll")
var procPlaySoundW = winmm.NewProc("PlaySoundW")

func main() {
	configPath := flag.String("config", "config.yaml", "config file path")
	flag.Parse()
	root, _ := os.Getwd()
	cfg := defaultConfig()
	if err := loadSimpleYAML(*configPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("load config failed, using defaults: %v", err)
	}
	cfg = absolutizeConfig(root, cfg)
	server := &Server{root: root, cfg: cfg, current: make(map[*exec.Cmd]bool), synthSem: make(chan struct{}, synthConcurrencyLimit)}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/selftest", server.selftest)
	mux.HandleFunc("/read/start", server.readStart)
	mux.HandleFunc("/read/status", server.readStatus)
	mux.HandleFunc("/read/stop", server.stop)
	mux.HandleFunc("/stop", server.stop)
	mux.HandleFunc("/shutdown", server.shutdown)
	mux.HandleFunc("/audio/probe", server.audioProbe)
	mux.HandleFunc("/docs/", server.docs)
	mux.HandleFunc("/", server.web)
	httpSrv := &http.Server{Addr: cfg.Listen, Handler: cors(mux)}
	server.httpSrv = httpSrv
	log.Printf("wps-tts-daemon-windows listening on http://%s", cfg.Listen)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func defaultConfig() Config {
	return Config{
		Listen: "127.0.0.1:19860",
		Sherpa: SherpaConfig{
			Bin:              "engines/sherpa-onnx/sherpa-onnx-offline-tts.exe",
			NumThreads:       4,
			TargetSampleRate: 16000,
			VitsModel:        "voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx",
			VitsLexicon:      "voices/sherpa/vits-zh-hf-fanchen-C/lexicon.txt",
			VitsTokens:       "voices/sherpa/vits-zh-hf-fanchen-C/tokens.txt",
			VitsRuleFsts:     "voices/sherpa/vits-zh-hf-fanchen-C/phone.fst,voices/sherpa/vits-zh-hf-fanchen-C/date.fst,voices/sherpa/vits-zh-hf-fanchen-C/number.fst,voices/sherpa/vits-zh-hf-fanchen-C/new_heteronym.fst",
			VitsSpeakerID:    14,
		},
	}
}

func absolutizeConfig(root string, cfg Config) Config {
	cfg.Sherpa.Bin = abs(root, cfg.Sherpa.Bin)
	cfg.Sherpa.VitsModel = abs(root, cfg.Sherpa.VitsModel)
	cfg.Sherpa.VitsLexicon = abs(root, cfg.Sherpa.VitsLexicon)
	cfg.Sherpa.VitsTokens = abs(root, cfg.Sherpa.VitsTokens)
	var fsts []string
	for _, item := range strings.Split(cfg.Sherpa.VitsRuleFsts, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			fsts = append(fsts, abs(root, item))
		}
	}
	cfg.Sherpa.VitsRuleFsts = strings.Join(fsts, ",")
	return cfg
}

func abs(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	engine := "none"
	if fileExists(s.cfg.Sherpa.Bin) && fileExists(s.cfg.Sherpa.VitsModel) && fileExists(s.cfg.Sherpa.VitsLexicon) && fileExists(s.cfg.Sherpa.VitsTokens) {
		engine = "sherpa-onnx"
	}
	probe := audioProbeInfo()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           engine == "sherpa-onnx",
		"version":      appVersion(s.root),
		"engine":       engine,
		"audio_player": probe["selected"],
		"audio_probe":  probe,
		"message":      "Windows 本地离线朗读服务已启动。",
	})
}

func (s *Server) audioProbe(w http.ResponseWriter, r *http.Request) {
	probe := audioProbeInfo()
	probe["version"] = appVersion(s.root)
	writeJSON(w, http.StatusOK, probe)
}

func audioProbeInfo() map[string]any {
	return map[string]any{
		"selected":  "Windows WinMM",
		"probed_at": time.Now().Format("2006-01-02 15:04:05"),
		"results": []map[string]string{
			{
				"name":    "Windows WinMM",
				"status":  "ok",
				"message": "使用 Windows 原生 WinMM 接口播放 WAV 音频。",
			},
		},
	}
}

func (s *Server) selftest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	wav, err := s.synthesize(ctx, "测试", 1.2)
	if wav != "" {
		defer os.Remove(wav)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, friendlyError(err))
		return
	}
	info, err := os.Stat(wav)
	if err != nil || info.Size() == 0 {
		writeError(w, http.StatusInternalServerError, "语音引擎未生成有效音频。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": info.Size()})
}

func (s *Server) readStart(w http.ResponseWriter, r *http.Request) {
	var req ReadStartRequest
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxReadRequestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确，请重新打开 WPS 后再试。")
		return
	}
	req.Rate = clampRate(req.Rate)
	var sentences []ReadSentence
	var totalRunes int
	for _, sentence := range req.Sentences {
		text := cleanText(sentence.Text)
		if text == "" {
			continue
		}
		runes := []rune(text)
		if len(runes) > maxSentenceRunes {
			runes = runes[:maxSentenceRunes]
			text = string(runes)
		}
		totalRunes += len(runes)
		if totalRunes > maxReadTextRunes {
			break
		}
		sentences = append(sentences, ReadSentence{Text: text})
		if len(sentences) >= maxReadSentences {
			break
		}
	}
	if len(sentences) == 0 {
		writeError(w, http.StatusBadRequest, "没有可朗读的有效句子。")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := &Session{
		id:        newID(),
		server:    s,
		ctx:       ctx,
		cancel:    cancel,
		sentences: sentences,
		rate:      req.Rate,
		state:     "preparing",
		message:   "朗读服务正在启动，请耐心等待",
		index:     -1,
		total:     len(sentences),
		cache:     make(map[int]*audioCacheEntry),
	}
	s.mu.Lock()
	s.stopLocked()
	s.session = session
	s.mu.Unlock()
	go session.run()
	writeJSON(w, http.StatusOK, session.status())
}

func (s *Server) readStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle", "current_index": -1, "total": 0})
		return
	}
	writeJSON(w, http.StatusOK, session.status())
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	s.stopLocked()
	s.session = nil
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": "stopped"})
}

func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	s.stopLocked()
	s.session = nil
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": "shutting_down"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.shutdownServer()
	}()
}

func (s *Server) shutdownServer() {
	s.stopOnce.Do(func() {
		if s.httpSrv == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(ctx)
	})
}

func (s *Server) web(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/addin")
	if path == "" || path == "/" {
		path = "/index.html"
	}
	http.ServeFile(w, r, filepath.Join(s.root, "addin", filepath.FromSlash(path)))
}

func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	switch name {
	case "THIRD_PARTY_NOTICES.md":
		http.ServeFile(w, r, filepath.Join(s.root, "third_party_licenses", name))
	case "RELEASE_NOTES.md", "SOURCE_OFFER.md":
		http.ServeFile(w, r, filepath.Join(s.root, name))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) stopLocked() {
	if s.session != nil {
		s.session.cancel()
	}
	stopWavPlayback()
	for cmd := range s.current {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	s.current = make(map[*exec.Cmd]bool)
}

func (s *Server) setCurrent(cmd *exec.Cmd) {
	s.mu.Lock()
	if s.current == nil {
		s.current = make(map[*exec.Cmd]bool)
	}
	s.current[cmd] = true
	s.mu.Unlock()
}

func (s *Server) clearCurrent(cmd *exec.Cmd) {
	s.mu.Lock()
	delete(s.current, cmd)
	s.mu.Unlock()
}

func (ss *Session) run() {
	defer ss.cleanup()
	ss.setState("preparing", "朗读服务正在启动，请耐心等待", -1)
	if len(ss.sentences) == 0 {
		ss.setState("done", "朗读完成", -1)
		return
	}
	first := ss.ensureAudio(0)
	ss.ensurePrefetch(1)
	if err := ss.waitEntry(first); err != nil {
		if ss.ctx.Err() != nil {
			ss.setState("stopped", "朗读已停止", -1)
			return
		}
		ss.setState("error", friendlyErrorAt(err, 0), 0)
		return
	}
	for i := range ss.sentences {
		if ss.ctx.Err() != nil {
			ss.setState("stopped", "朗读已停止", i)
			return
		}
		entry := ss.ensureAudio(i)
		ss.ensurePrefetch(i + 1)
		ss.setState("synthesizing", "正在准备第 "+strconv.Itoa(i+1)+" 句", i)
		if err := ss.waitEntry(entry); err != nil {
			if ss.ctx.Err() != nil {
				ss.setState("stopped", "朗读已停止", i)
			} else {
				ss.setState("error", friendlyErrorAt(err, i), i)
			}
			return
		}
		ss.setState("playing", "正在朗读第 "+strconv.Itoa(i+1)+" 句", i)
		if err := ss.server.play(ss.ctx, entry.path); err != nil {
			if ss.ctx.Err() != nil {
				ss.setState("stopped", "朗读已停止", i)
			} else {
				ss.setState("error", friendlyErrorAt(err, i), i)
			}
			return
		}
		ss.removeCache(i)
	}
	ss.setState("done", "朗读完成", len(ss.sentences)-1)
	ss.server.mu.Lock()
	if ss.server.session == ss {
		ss.server.session = nil
	}
	ss.server.mu.Unlock()
}

func (ss *Session) ensurePrefetch(start int) {
	count := ss.prefetchCount(start)
	for i := start; i < start+count; i++ {
		ss.ensureAudio(i)
	}
}

func (ss *Session) prefetchCount(start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(ss.sentences) {
		return 0
	}
	count := 0
	runes := 0
	for i := start; i < len(ss.sentences) && count < prefetchSentenceLimit; i++ {
		count++
		runes += len([]rune(ss.sentences[i].Text))
		if runes >= prefetchTextTarget {
			break
		}
	}
	return count
}

func (ss *Session) ensureAudio(index int) *audioCacheEntry {
	ss.mu.Lock()
	if entry := ss.cache[index]; entry != nil {
		ss.mu.Unlock()
		return entry
	}
	entry := &audioCacheEntry{index: index, ready: make(chan struct{})}
	ss.cache[index] = entry
	text := ss.sentences[index].Text
	rate := ss.rate
	ss.mu.Unlock()

	go func() {
		path, err := ss.server.synthesize(ss.ctx, text, rate)
		if err == nil && path != "" {
			if pauseErr := appendSilenceToWavFile(path, sentenceEndPauseMs(rate)); pauseErr != nil {
				os.Remove(path)
				path = ""
				err = pauseErr
			}
		}
		entry.path = path
		entry.err = err
		close(entry.ready)
	}()
	return entry
}

func (ss *Session) waitEntry(entry *audioCacheEntry) error {
	select {
	case <-entry.ready:
		return entry.err
	case <-ss.ctx.Done():
		return ss.ctx.Err()
	}
}

func (ss *Session) removeCache(index int) {
	ss.mu.Lock()
	entry := ss.cache[index]
	delete(ss.cache, index)
	ss.mu.Unlock()
	if entry != nil && entry.path != "" {
		os.Remove(entry.path)
	}
}

func (ss *Session) cleanup() {
	ss.mu.Lock()
	entries := make([]*audioCacheEntry, 0, len(ss.cache))
	for _, entry := range ss.cache {
		entries = append(entries, entry)
	}
	ss.cache = make(map[int]*audioCacheEntry)
	ss.mu.Unlock()
	for _, entry := range entries {
		<-entry.ready
		if entry.path != "" {
			os.Remove(entry.path)
		}
	}
}

func (ss *Session) setState(state, message string, index int) {
	ss.mu.Lock()
	ss.state = state
	ss.message = message
	ss.index = index
	ss.mu.Unlock()
}

func (ss *Session) status() map[string]any {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return map[string]any{"id": ss.id, "state": ss.state, "message": ss.message, "current_index": ss.index, "total": ss.total}
}

func (s *Server) synthesize(ctx context.Context, text string, rate float64) (string, error) {
	if !fileExists(s.cfg.Sherpa.Bin) {
		return "", errors.New("no available tts engine")
	}
	select {
	case s.synthSem <- struct{}{}:
		defer func() { <-s.synthSem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	prepared := preprocessFanchenText(text, rate)
	candidates := ttsTextCandidates(prepared)
	var lastErr error
	for attempt, candidate := range candidates {
		if attempt > 0 {
			os.Remove(tmpPath)
			tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
			if err != nil {
				return "", err
			}
			tmpPath = tmp.Name()
			tmp.Close()
		}
		lastErr = s.runSherpa(ctx, tmpPath, candidate, rate)
		if lastErr == nil {
			return tmpPath, nil
		}
		if attempt+1 < len(candidates) {
			continue
		}
		if !isTokenIDFailure(lastErr) {
			break
		}
	}
	os.Remove(tmpPath)
	return "", lastErr
}

func (s *Server) runSherpa(ctx context.Context, outputPath string, text string, rate float64) error {
	args := []string{
		"--vits-model=" + s.cfg.Sherpa.VitsModel,
		"--vits-lexicon=" + s.cfg.Sherpa.VitsLexicon,
		"--vits-tokens=" + s.cfg.Sherpa.VitsTokens,
		"--sid=" + strconv.Itoa(s.cfg.Sherpa.VitsSpeakerID),
		"--num-threads=" + strconv.Itoa(sherpaNumThreads(s.cfg.Sherpa.NumThreads)),
		"--vits-noise-scale=0.667",
		"--vits-noise-scale-w=0.8",
		"--vits-length-scale=" + fmt.Sprintf("%.3f", 1/clampRate(rate)),
		"--output-filename=" + outputPath,
	}
	if fsts := existingRuleFsts(s.cfg.Sherpa.VitsRuleFsts); fsts != "" {
		args = append(args, "--tts-rule-fsts="+fsts)
	}
	args = append(args, text)
	cmd := exec.CommandContext(ctx, s.cfg.Sherpa.Bin, args...)
	cmd.Dir = s.root
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	s.setCurrent(cmd)
	err := cmd.Run()
	s.clearCurrent(cmd)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(output.String())
	if detail != "" {
		return fmt.Errorf("sherpa-onnx failed: %w: %s", err, detail)
	}
	return fmt.Errorf("sherpa-onnx failed: %w", err)
}

func (s *Server) play(ctx context.Context, wav string) error {
	duration, err := wavDuration(wav)
	if err != nil {
		return err
	}
	if err := playWavAsync(wav); err != nil {
		return err
	}
	timer := time.NewTimer(duration + 120*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		stopWavPlayback()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func playWavAsync(path string) error {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ret, _, callErr := procPlaySoundW.Call(
		uintptr(unsafe.Pointer(ptr)),
		0,
		uintptr(0x00020000|0x0001|0x0002),
	)
	if ret == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("Windows 音频播放失败：%w", callErr)
		}
		return errors.New("Windows 音频播放失败")
	}
	return nil
}

func wavDuration(path string) (time.Duration, error) {
	wav, err := readPCM16Wav(path)
	if err != nil {
		return 0, err
	}
	if wav.sampleRate == 0 || wav.channels == 0 || wav.bitsPerSample == 0 {
		return 0, errors.New("invalid wav duration")
	}
	bytesPerFrame := int(wav.channels) * int(wav.bitsPerSample) / 8
	if bytesPerFrame <= 0 {
		return 0, errors.New("invalid wav frame size")
	}
	frames := len(wav.data) / bytesPerFrame
	return time.Duration(frames) * time.Second / time.Duration(wav.sampleRate), nil
}

func stopWavPlayback() {
	procPlaySoundW.Call(0, 0, 0)
}

func preprocessFanchenText(text string, rate float64) string {
	text = normalizeTocIndexText(text)
	text = percentValueRE.ReplaceAllStringFunc(text, func(match string) string {
		parts := percentValueRE.FindStringSubmatch(match)
		if len(parts) < 2 {
			return " 百分号 "
		}
		return " 百分之" + numberSpeech(parts[1]) + " "
	})
	text = strings.NewReplacer(
		"　", " ",
		"℃", "摄氏度",
		"&", "和",
		"@", " 艾特 ",
		"#", " 井号 ",
		"$", " 美元 ",
		"￥", " 元 ",
		"¥", " 元 ",
		"€", " 欧元 ",
		"→", " 到 ",
		"←", " 到 ",
		"—", " ",
		"–", " ",
		"•", " ",
		"·", " ",
	).Replace(text)
	text = specialEnglishTokenRE.ReplaceAllStringFunc(text, func(token string) string {
		switch strings.ToLower(token) {
		case "wps":
			return " 达不溜屁挨思 "
		case "office":
			return " 凹斐思 "
		default:
			return token
		}
	})
	text = asciiTokenRE.ReplaceAllStringFunc(text, func(token string) string {
		var parts []string
		for _, r := range token {
			if spoken := asciiCharSpeech(r); spoken != "" {
				parts = append(parts, spoken)
			}
		}
		if len(parts) == 0 {
			return " "
		}
		return " " + strings.Join(parts, " ") + " "
	})
	text = strings.NewReplacer(
		"+", " 加 ",
		"＋", " 加 ",
		"-", " 减 ",
		"－", " 减 ",
		"−", " 减 ",
		"*", " 乘 ",
		"×", " 乘 ",
		"/", " 除 ",
		"÷", " 除 ",
		"=", " 等于 ",
		"＝", " 等于 ",
		"≈", " 约等于 ",
		"≠", " 不等于 ",
		"<=", " 小于等于 ",
		"≤", " 小于等于 ",
		">=", " 大于等于 ",
		"≥", " 大于等于 ",
		"<", " 小于 ",
		">", " 大于 ",
		"%", " 百分号 ",
		"％", " 百分号 ",
		"±", " 正负 ",
		"√", " 根号 ",
		"∑", " 求和 ",
		"∏", " 连乘 ",
		"∞", " 无穷大 ",
		"∫", " 积分 ",
		"∂", " 偏导 ",
		"∈", " 属于 ",
		"∉", " 不属于 ",
		"⊂", " 包含于 ",
		"⊆", " 包含于或等于 ",
		"∪", " 并集 ",
		"∩", " 交集 ",
		"∧", " 且 ",
		"∨", " 或 ",
		"¬", " 非 ",
		"°", " 度 ",
		"‰", " 千分号 ",
	).Replace(text)
	text = sanitizeTtsRunes(text)
	return normalizeTtsPunctuationSpacing(text, rate)
}

func numberSpeech(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.SplitN(value, ".", 2)
	integer := integerSpeech(parts[0])
	if len(parts) == 1 {
		return integer
	}
	var decimals []string
	for _, r := range parts[1] {
		if spoken := digitSpeech(r); spoken != "" {
			decimals = append(decimals, spoken)
		}
	}
	if len(decimals) == 0 {
		return integer
	}
	return integer + "点" + strings.Join(decimals, "")
}

func integerSpeech(value string) string {
	value = strings.TrimLeft(strings.TrimSpace(value), "0")
	if value == "" {
		return "零"
	}
	if len(value) > 12 {
		var digits []string
		for _, r := range value {
			if spoken := digitSpeech(r); spoken != "" {
				digits = append(digits, spoken)
			}
		}
		return strings.Join(digits, "")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		var digits []string
		for _, r := range value {
			if spoken := digitSpeech(r); spoken != "" {
				digits = append(digits, spoken)
			}
		}
		return strings.Join(digits, "")
	}
	return integerToChinese(n)
}

func integerToChinese(n int64) string {
	digits := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	units := []string{"", "十", "百", "千"}
	sectionUnits := []string{"", "万", "亿", "万亿"}
	if n == 0 {
		return "零"
	}
	var sections []int
	for n > 0 {
		sections = append(sections, int(n%10000))
		n /= 10000
	}
	var out []string
	needZero := false
	for i := len(sections) - 1; i >= 0; i-- {
		section := sections[i]
		if section == 0 {
			needZero = len(out) > 0
			continue
		}
		if needZero || (len(out) > 0 && section < 1000) {
			out = append(out, "零")
		}
		out = append(out, sectionToChinese(section, digits, units)+sectionUnits[i])
		needZero = section < 1000 && i > 0
	}
	result := strings.Join(out, "")
	if strings.HasPrefix(result, "一十") {
		result = strings.TrimPrefix(result, "一")
	}
	return result
}

func sectionToChinese(section int, digits []string, units []string) string {
	var out []string
	zero := false
	for i := 3; i >= 0; i-- {
		unitValue := 1
		for j := 0; j < i; j++ {
			unitValue *= 10
		}
		digit := section / unitValue
		section %= unitValue
		if digit == 0 {
			if len(out) > 0 {
				zero = true
			}
			continue
		}
		if zero {
			out = append(out, "零")
			zero = false
		}
		out = append(out, digits[digit]+units[i])
	}
	return strings.Join(out, "")
}

func digitSpeech(r rune) string {
	switch r {
	case '0':
		return "零"
	case '1':
		return "一"
	case '2':
		return "二"
	case '3':
		return "三"
	case '4':
		return "四"
	case '5':
		return "五"
	case '6':
		return "六"
	case '7':
		return "七"
	case '8':
		return "八"
	case '9':
		return "九"
	default:
		return ""
	}
}

func normalizeTocIndexText(text string) string {
	text = wordTocFieldRE.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	if purePageNumberRE.MatchString(text) {
		return "第 " + pageRangeText(text) + " 页"
	}
	if match := tocLeaderPageRE.FindStringSubmatchIndex(text); match != nil {
		page := strings.TrimSpace(text[match[2]:match[3]])
		prefix := strings.TrimSpace(text[:match[0]])
		if prefix == "" {
			return "第 " + pageRangeText(page) + " 页"
		}
		return prefix + " 第 " + pageRangeText(page) + " 页"
	}
	return text
}

func pageRangeText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.NewReplacer("-", " 到 ", "–", " 到 ", "—", " 到 ").Replace(text)
	return text
}

func ttsTextCandidates(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	candidates := []string{text}
	if shortCJKText(text) {
		contextual := "第 " + text + " 项"
		if contextual != text {
			candidates = append(candidates, contextual)
		}
	}
	return candidates
}

func shortCJKText(text string) bool {
	var cjk, other int
	for _, r := range text {
		if unicode.IsSpace(r) || isPairedBoundaryPunctuation(r) {
			continue
		}
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk++
		} else {
			other++
		}
	}
	return cjk > 0 && cjk <= 2 && other == 0
}

func isTokenIDFailure(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Failed to convert") || strings.Contains(msg, "token IDs")
}

func sanitizeTtsRunes(text string) string {
	var out []rune
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf) || isPrivateUseOrSurrogate(r):
			out = append(out, ' ')
		case unicode.IsSymbol(r):
			out = append(out, ' ')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func isPrivateUseOrSurrogate(r rune) bool {
	return (r >= 0xD800 && r <= 0xDFFF) ||
		(r >= 0xE000 && r <= 0xF8FF) ||
		(r >= 0xF0000 && r <= 0xFFFFD) ||
		(r >= 0x100000 && r <= 0x10FFFD)
}

func normalizeTtsPunctuationSpacing(text string, rate float64) string {
	var out []rune
	for _, r := range text {
		switch {
		case isSemanticPausePunctuation(r):
			for len(out) > 0 && unicode.IsSpace(out[len(out)-1]) {
				out = out[:len(out)-1]
			}
			out = append(out, r, ' ')
		case unicode.IsSpace(r):
			if len(out) > 0 && out[len(out)-1] != ' ' {
				out = append(out, ' ')
			}
		default:
			out = append(out, r)
		}
	}
	return strings.TrimSpace(string(out))
}

func isSemanticPausePunctuation(r rune) bool {
	switch r {
	case '，', ',', '、', '。', '；', ';', '：', ':', '！', '!', '？', '?':
		return true
	default:
		return false
	}
}

func isPairedBoundaryPunctuation(r rune) bool {
	switch r {
	case '“', '”', '‘', '’', '"', '\'', '《', '》', '〈', '〉', '（', '）', '(', ')', '【', '】', '[', ']', '「', '」', '『', '』':
		return true
	default:
		return false
	}
}

func asciiCharSpeech(r rune) string {
	switch r {
	case '0':
		return "零"
	case '1':
		return "一"
	case '2':
		return "二"
	case '3':
		return "三"
	case '4':
		return "四"
	case '5':
		return "五"
	case '6':
		return "六"
	case '7':
		return "七"
	case '8':
		return "八"
	case '9':
		return "九"
	case '.':
		return "点"
	case '-':
		return "杠"
	case '_':
		return "下划线"
	case '+':
		return "加"
	}
	switch unicode.ToUpper(r) {
	case 'A':
		return "诶"
	case 'B':
		return "必"
	case 'C':
		return "西"
	case 'D':
		return "迪"
	case 'E':
		return "伊"
	case 'F':
		return "艾弗"
	case 'G':
		return "吉"
	case 'H':
		return "艾尺"
	case 'I':
		return "爱"
	case 'J':
		return "杰"
	case 'K':
		return "开"
	case 'L':
		return "艾勒"
	case 'M':
		return "艾姆"
	case 'N':
		return "恩"
	case 'O':
		return "欧"
	case 'P':
		return "批"
	case 'Q':
		return "丘"
	case 'R':
		return "阿尔"
	case 'S':
		return "艾丝"
	case 'T':
		return "提"
	case 'U':
		return "优"
	case 'V':
		return "维"
	case 'W':
		return "达不溜"
	case 'X':
		return "艾克斯"
	case 'Y':
		return "歪"
	case 'Z':
		return "兹"
	}
	return ""
}

type wavPCM struct {
	format        uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	data          []byte
}

func appendSilenceToWavFile(path string, pauseMs int) error {
	if pauseMs <= 0 {
		return nil
	}
	wav, err := readPCM16Wav(path)
	if err != nil {
		return err
	}
	wav.data = append(wav.data, silencePCM(wav, pauseMs)...)
	tmp, err := os.CreateTemp(filepath.Dir(path), "wps-read-aloud-pause-*.wav")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := writePCM16Wav(tmp, wav, wav.data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func silencePCM(wav wavPCM, pauseMs int) []byte {
	if pauseMs <= 0 || wav.sampleRate == 0 || wav.channels == 0 || wav.bitsPerSample == 0 {
		return nil
	}
	bytesPerFrame := int(wav.channels) * int(wav.bitsPerSample) / 8
	if bytesPerFrame <= 0 {
		return nil
	}
	frames := int(uint64(wav.sampleRate) * uint64(pauseMs) / 1000)
	return make([]byte, frames*bytesPerFrame)
}

func readPCM16Wav(path string) (wavPCM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wavPCM{}, err
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavPCM{}, errors.New("unsupported wav format")
	}
	offset := 12
	var wav wavPCM
	dataStart := -1
	dataSize := 0
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkData := offset + 8
		if chunkData+chunkSize > len(data) {
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkSize >= 16 {
				wav.format = binary.LittleEndian.Uint16(data[chunkData : chunkData+2])
				wav.channels = binary.LittleEndian.Uint16(data[chunkData+2 : chunkData+4])
				wav.sampleRate = binary.LittleEndian.Uint32(data[chunkData+4 : chunkData+8])
				wav.bitsPerSample = binary.LittleEndian.Uint16(data[chunkData+14 : chunkData+16])
			}
		case "data":
			dataStart = chunkData
			dataSize = chunkSize
		}
		offset = chunkData + chunkSize
		if offset%2 == 1 {
			offset += 1
		}
	}
	if wav.format != 1 || wav.channels != 1 || wav.bitsPerSample != 16 || dataStart < 0 {
		return wavPCM{}, errors.New("unsupported wav encoding")
	}
	wav.data = append([]byte(nil), data[dataStart:dataStart+dataSize]...)
	return wav, nil
}

func writePCM16Wav(w io.Writer, spec wavPCM, pcm []byte) error {
	byteRate := spec.sampleRate * uint32(spec.channels) * uint32(spec.bitsPerSample) / 8
	blockAlign := spec.channels * spec.bitsPerSample / 8
	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(36+len(pcm))); err != nil {
		return err
	}
	if _, err := w.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, spec.format); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, spec.channels); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, spec.sampleRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, spec.bitsPerSample); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(pcm))); err != nil {
		return err
	}
	_, err := w.Write(pcm)
	return err
}

func sentenceEndPauseMs(rate float64) int {
	return scaledPauseMs(sentenceEndPauseMsAtBaseRate, rate)
}

func scaledPauseMs(baseMs int, rate float64) int {
	if baseMs <= 0 {
		return 0
	}
	rate = clampRate(rate)
	return int(float64(baseMs)*pauseBaseRate/rate + 0.5)
}

func existingRuleFsts(value string) string {
	var kept []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if fileExists(item) {
			kept = append(kept, item)
		}
	}
	return strings.Join(kept, ",")
}

func appVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "version.json"))
	if err != nil {
		return "dev"
	}
	var info struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &info) != nil || strings.TrimSpace(info.Version) == "" {
		return "dev"
	}
	return info.Version
}

func friendlyError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no available tts engine"):
		return "朗读引擎不可用，请重新安装加载项安装包。"
	case strings.Contains(msg, "sherpa-onnx failed"):
		return "语音合成失败。短文档可正常朗读时，通常不是安装包损坏，而是当前句包含模型不支持的特殊字符，或系统资源不足导致本句合成失败。"
	default:
		return "朗读失败，请稍后重试。"
	}
}

func friendlyErrorAt(err error, index int) string {
	message := friendlyError(err)
	if index >= 0 {
		message = "第 " + strconv.Itoa(index+1) + " 句" + message
	}
	detail := compactErrorDetail(err.Error())
	if detail != "" {
		message += " 详细原因：" + detail
	}
	return message
}

func compactErrorDetail(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "sherpa-onnx failed:")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var useful []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "parse-options.cc:Read") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(line, "Failed to convert") || strings.Contains(line, "Error in generating") || strings.Contains(lower, "failed") || strings.Contains(lower, "error") {
			useful = append(useful, line)
		}
	}
	if len(useful) > 0 {
		text = strings.Join(useful, "；")
	}
	if len([]rune(text)) > 180 {
		return string([]rune(text)[:180]) + "..."
	}
	return text
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 32 || r == '\ufeff' || r == '\ufffc' || r == '\ufffd' {
			return -1
		}
		return r
	}, text)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func clampRate(rate float64) float64 {
	if rate <= 0 {
		return 1.2
	}
	if rate < 0.5 {
		return 0.5
	}
	if rate > 2.0 {
		return 2.0
	}
	return rate
}

func sherpaNumThreads(numThreads int) int {
	if numThreads < 1 {
		return 1
	}
	if numThreads > 4 {
		return 4
	}
	return numThreads
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buf)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loadSimpleYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var section string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		switch section + "." + key {
		case ".listen":
			cfg.Listen = value
		case "sherpa.bin":
			cfg.Sherpa.Bin = value
		case "sherpa.num_threads":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.Sherpa.NumThreads = parsed
			}
		case "sherpa.target_sample_rate":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.Sherpa.TargetSampleRate = parsed
			}
		case "sherpa.vits_model":
			cfg.Sherpa.VitsModel = value
		case "sherpa.vits_lexicon":
			cfg.Sherpa.VitsLexicon = value
		case "sherpa.vits_tokens":
			cfg.Sherpa.VitsTokens = value
		case "sherpa.vits_rule_fsts":
			cfg.Sherpa.VitsRuleFsts = value
		case "sherpa.vits_speaker_id":
			if parsed, err := strconv.Atoi(value); err == nil {
				cfg.Sherpa.VitsSpeakerID = parsed
			}
		}
	}
	return nil
}
