//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
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
	"unicode/utf8"
)

//go:embed web
var webFS embed.FS

const prefetchTextTarget = 240
const prefetchSentenceLimit = 8
const pauseBaseRate = 1.2
const standardPauseMsAtBaseRate = 400
const sentenceEndPauseMsAtBaseRate = 600
const maxReadRequestBytes = 64 << 20
const maxReadSentences = 20000
const maxReadTextRunes = 2000000
const maxSentenceRunes = 1000

type AppInfo struct {
	Name         string `json:"name"`
	Package      string `json:"package"`
	Version      string `json:"version"`
	ReleaseDate  string `json:"release_date"`
	Distro       string `json:"distro"`
	Architecture string `json:"architecture"`
	InstallRoot  string `json:"install_root"`
}

func appRoot() string {
	if value := strings.TrimSpace(os.Getenv("WPS_READ_ALOUD_ROOT")); value != "" {
		return filepath.Clean(value)
	}
	return "/opt/wps-read-aloud-comate"
}

func appVersionPath() string {
	return filepath.Join(appRoot(), "version.json")
}

func addinDiskDir() string {
	return filepath.Join(appRoot(), "addin")
}

func docDir() string {
	if value := strings.TrimSpace(os.Getenv("WPS_READ_ALOUD_DOC_DIR")); value != "" {
		return filepath.Clean(value)
	}
	return "/usr/share/doc/wps-read-aloud-comate"
}

func audioProbePath() string {
	if value := strings.TrimSpace(os.Getenv("WPS_READ_ALOUD_AUDIO_PROBE")); value != "" {
		return filepath.Clean(value)
	}
	return "/var/lib/wps-read-aloud/audio-player.json"
}

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

type SpeakRequest struct {
	Text  string  `json:"text"`
	Voice string  `json:"voice"`
	Rate  float64 `json:"rate"`
}

type Server struct {
	cfg     Config
	mu      sync.Mutex
	current *processGroup
	session *readSession
	engine  string
}

type processGroup struct {
	mu     sync.Mutex
	id     string
	cancel context.CancelFunc
	cmds   []*exec.Cmd
}

func (pg *processGroup) commands() []*exec.Cmd {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	return append([]*exec.Cmd(nil), pg.cmds...)
}

type ReadSentence struct {
	Text string `json:"text"`
}

type ReadStartRequest struct {
	Sentences []ReadSentence `json:"sentences"`
	Rate      float64        `json:"rate"`
	Prefetch  int            `json:"prefetch"`
}

type ReadSettingsRequest struct {
	Rate float64 `json:"rate"`
}

type readSession struct {
	id        string
	sentences []ReadSentence
	prefetch  int
	server    *Server
	group     *processGroup
	ctx       context.Context
	cancel    context.CancelFunc

	mu           sync.Mutex
	cond         *sync.Cond
	state        string
	message      string
	currentIndex int
	rate         float64
	rateVersion  int
	cache        map[int]*audioCacheEntry
	lastError    string
}

type audioCacheEntry struct {
	index       int
	rate        float64
	rateVersion int
	path        string
	err         error
	ready       chan struct{}
}

type audioPlayer struct {
	bin        string
	args       []string
	env        []string
	uid        uint32
	gid        uint32
	credential bool
}

type audioProbeResult struct {
	Version     string           `json:"version"`
	Selected    string           `json:"selected"`
	SelectedBin string           `json:"selected_bin,omitempty"`
	ProbedAt    string           `json:"probed_at"`
	Results     []audioProbeItem `json:"results"`
}

type audioProbeItem struct {
	Name    string `json:"name"`
	Bin     string `json:"bin,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func main() {
	configPath := flag.String("config", "/etc/wps-read-aloud-comate/config.yaml", "config file path")
	flag.Parse()

	cfg := defaultConfig()
	if err := loadSimpleYAML(*configPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("load config failed, using defaults: %v", err)
	}

	server := &Server{cfg: cfg, engine: detectEngine(cfg)}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/selftest", server.selftest)
	mux.HandleFunc("/audio/probe", server.audioProbe)
	mux.HandleFunc("/voices", server.voices)
	mux.HandleFunc("/play", server.play)
	mux.HandleFunc("/speak", server.speak)
	mux.HandleFunc("/synthesize", server.synthesize)
	mux.HandleFunc("/read/start", server.readStart)
	mux.HandleFunc("/read/settings", server.readSettings)
	mux.HandleFunc("/read/status", server.readStatus)
	mux.HandleFunc("/read/stop", server.stop)
	mux.HandleFunc("/read/pause", server.pause)
	mux.HandleFunc("/read/resume", server.resume)
	mux.HandleFunc("/stop", server.stop)
	mux.HandleFunc("/pause", server.pause)
	mux.HandleFunc("/resume", server.resume)
	mux.HandleFunc("/docs/", server.docs)
	mux.HandleFunc("/", server.web)

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           cors(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("wps-tts-daemon listening on http://%s, engine=%s", cfg.Listen, server.engine)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func defaultConfig() Config {
	return Config{
		Listen: "127.0.0.1:19860",
		Sherpa: SherpaConfig{
			Bin:              "/opt/wps-read-aloud-comate/engines/sherpa-onnx/sherpa-onnx-offline-tts",
			NumThreads:       2,
			TargetSampleRate: 16000,
			VitsModel:        "/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx",
			VitsLexicon:      "/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/lexicon.txt",
			VitsTokens:       "/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/tokens.txt",
			VitsRuleFsts:     "/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/phone.fst,/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/date.fst,/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/number.fst,/opt/wps-read-aloud-comate/voices/sherpa/vits-zh-hf-fanchen-C/new_heteronym.fst",
			VitsSpeakerID:    14,
		},
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	engine := detectEngine(s.cfg)
	probe := loadAudioProbe()
	players := prioritizedAudioPlayers("")
	playerName := ""
	if probe.Selected != "" {
		playerName = probe.Selected
	} else if len(players) > 0 {
		playerName = filepath.Base(players[0].bin)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           engine != "none",
		"version":      appVersion(),
		"engine":       engine,
		"audio_player": playerName,
		"audio_probe":  probe,
		"message":      healthMessage(engine),
	})
}

func (s *Server) audioProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result := s.probeAudioPlayers(r.Context())
	status := http.StatusOK
	if result.Selected == "" {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, result)
}

func (s *Server) selftest(w http.ResponseWriter, r *http.Request) {
	req := SpeakRequest{
		Text:  "测试",
		Voice: "zh_CN",
		Rate:  1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	group := &processGroup{id: "selftest", cancel: cancel}
	wavPath, err := s.synthesizeSpeech(ctx, group, req)
	if err != nil {
		log.Printf("selftest failed: %v", err)
		writeError(w, http.StatusInternalServerError, friendlyError(err))
		return
	}
	defer os.Remove(wavPath)
	info, err := os.Stat(wavPath)
	if err != nil || info.Size() == 0 {
		writeError(w, http.StatusInternalServerError, "语音引擎未生成有效音频，请联系管理员检查安装包完整性。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"engine": detectEngine(s.cfg),
		"bytes":  info.Size(),
	})
}

func (s *Server) voices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"voices": []map[string]string{
			{"id": "zh_CN", "name": "中文普通话 Sherpa VITS fanchen-C"},
		},
	})
}

func (s *Server) web(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "web assets unavailable")
		return
	}
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	} else {
		path = strings.TrimPrefix(path, "/addin")
		if path == "" || path == "/" {
			path = "/index.html"
		}
	}
	if diskPath := diskIconPath(path); diskPath != "" {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, diskPath)
		return
	}
	fileRequest := r.Clone(r.Context())
	fileRequest.URL.Path = path
	http.FileServer(http.FS(sub)).ServeHTTP(w, fileRequest)
}

func diskIconPath(requestPath string) string {
	if !strings.HasPrefix(requestPath, "/assets/icons/") || !strings.HasSuffix(strings.ToLower(requestPath), ".png") {
		return ""
	}
	name := filepath.Base(requestPath)
	switch name {
	case "start.png", "stop.png", "mode.png", "rate.png", "status.png", "about.png":
		path := filepath.Join(addinDiskDir(), "assets", "icons", name)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	allowed := map[string]bool{
		"RELEASE_NOTES.md":         true,
		"THIRD_PARTY_NOTICES.md":   true,
		"SOURCE_OFFER.md":          true,
		"BUILD_RELEASE_LESSONS.md": true,
	}
	if !allowed[name] {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(docDir(), name))
}

func (s *Server) speak(w http.ResponseWriter, r *http.Request) {
	s.play(w, r)
}

func (s *Server) decodeSpeakRequest(w http.ResponseWriter, r *http.Request) (SpeakRequest, bool) {
	var req SpeakRequest
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return req, false
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确，请重新打开 WPS 后再试。")
		return req, false
	}
	req.Text = cleanText(req.Text)
	req.Voice = normalizeVoice(req.Voice)
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "没有可朗读的文本，请先选中文本或打开包含正文的文档。")
		return req, false
	}
	if len([]rune(req.Text)) > 200000 {
		writeError(w, http.StatusBadRequest, "文档内容过长，请先选中一部分内容朗读。")
		return req, false
	}
	if len([]rune(req.Text)) > 1000 {
		writeError(w, http.StatusBadRequest, "单句内容过长，请缩短选区或按段落朗读。")
		return req, false
	}
	if req.Rate <= 0 {
		req.Rate = 1
	}
	return req, true
}

func (s *Server) decodeReadStartRequest(w http.ResponseWriter, r *http.Request) (ReadStartRequest, bool) {
	var req ReadStartRequest
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return req, false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxReadRequestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确，请重新打开 WPS 后再试。")
		return req, false
	}
	if req.Rate <= 0 {
		req.Rate = 1
	}
	req.Rate = clampRate(req.Rate)
	if req.Prefetch <= 0 {
		req.Prefetch = 3
	}
	if req.Prefetch > 5 {
		req.Prefetch = 5
	}
	if len(req.Sentences) == 0 {
		writeError(w, http.StatusBadRequest, "没有可朗读的句子，请先选择文档内容。")
		return req, false
	}
	var total int
	out := req.Sentences[:0]
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
		total += len(runes)
		if total > maxReadTextRunes {
			break
		}
		out = append(out, ReadSentence{Text: text})
		if len(out) >= maxReadSentences {
			break
		}
	}
	req.Sentences = out
	if len(req.Sentences) == 0 {
		writeError(w, http.StatusBadRequest, "没有可朗读的有效句子。")
		return req, false
	}
	return req, true
}

func (s *Server) readStart(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeReadStartRequest(w, r)
	if !ok {
		return
	}
	if detectEngine(s.cfg) == "none" {
		writeError(w, http.StatusInternalServerError, healthMessage("none"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := newID()
	session := &readSession{
		id:           id,
		sentences:    req.Sentences,
		prefetch:     req.Prefetch,
		server:       s,
		group:        &processGroup{id: id, cancel: cancel},
		ctx:          ctx,
		cancel:       cancel,
		state:        "starting",
		currentIndex: -1,
		rate:         req.Rate,
		cache:        make(map[int]*audioCacheEntry),
	}
	session.cond = sync.NewCond(&session.mu)

	s.mu.Lock()
	s.stopLocked()
	if s.session != nil {
		s.session.stop()
	}
	s.current = session.group
	s.session = session
	s.mu.Unlock()

	go session.run()
	writeJSON(w, http.StatusOK, session.status())
}

func (s *Server) readSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req ReadSettingsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "idle"})
		return
	}
	session.updateSettings(req.Rate)
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

func (s *Server) synthesize(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeSpeakRequest(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	id := newID()
	group := &processGroup{id: id, cancel: cancel}

	s.mu.Lock()
	s.stopLocked()
	s.current = group
	s.mu.Unlock()

	wavPath, err := s.synthesizeSpeech(ctx, group, req)
	s.mu.Lock()
	if s.current == group {
		s.current = nil
	}
	s.mu.Unlock()
	if err != nil {
		log.Printf("synthesize %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, friendlyError(err))
		return
	}
	defer os.Remove(wavPath)

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, wavPath)
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeSpeakRequest(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	id := newID()
	group := &processGroup{id: id, cancel: cancel}

	s.mu.Lock()
	s.stopLocked()
	s.current = group
	s.mu.Unlock()

	var wavPath string
	var err error
	wavPath, err = s.synthesizeSpeech(ctx, group, req)
	if err == nil && wavPath != "" {
		err = s.playAudio(ctx, group, wavPath)
		if err != nil {
			log.Printf("system audio playback failed; re-probing audio players: %v", err)
			probe := s.probeAudioPlayers(context.Background())
			if probe.Selected != "" {
				err = s.playAudio(ctx, group, wavPath)
			}
		}
	}
	s.mu.Lock()
	if s.current == group {
		s.current = nil
	}
	s.mu.Unlock()
	if wavPath != "" {
		defer os.Remove(wavPath)
	}
	if err != nil {
		log.Printf("play %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, friendlyError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	if s.session != nil {
		s.session.stop()
		s.session = nil
	}
	s.stopLocked()
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": "stopped"})
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.signalCurrent(w, syscall.SIGSTOP, "paused")
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.signalCurrent(w, syscall.SIGCONT, "resumed")
}

func (s *Server) signalCurrent(w http.ResponseWriter, sig syscall.Signal, status string) {
	s.mu.Lock()
	session := s.session
	if session != nil {
		if status == "paused" {
			session.setPaused(true)
		} else if status == "resumed" {
			session.setPaused(false)
		}
	}
	defer s.mu.Unlock()
	if s.current == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "idle"})
		return
	}
	for _, cmd := range s.current.commands() {
		signalProcessGroup(cmd, sig)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (s *Server) stopLocked() {
	if s.current == nil {
		return
	}
	s.current.cancel()
	for _, cmd := range s.current.commands() {
		terminateProcessGroup(cmd)
	}
	s.current = nil
}

func (rs *readSession) run() {
	defer rs.cleanup()
	rs.setState("preparing", "朗读服务正在启动，请耐心等待", -1)
	if len(rs.sentences) == 0 {
		rs.setState("done", "朗读完成", -1)
		return
	}
	first := rs.ensureAudio(0)
	rs.ensurePrefetch(1)
	if err := rs.waitEntry(first); err != nil {
		if rs.ctx.Err() != nil {
			rs.setState("stopped", "朗读已停止", -1)
			return
		}
		rs.fail(err)
		return
	}
	for i := range rs.sentences {
		if rs.ctx.Err() != nil {
			rs.setState("stopped", "朗读已停止", i)
			return
		}
		entry := rs.ensureAudio(i)
		rs.ensurePrefetch(i + 1)
		rs.setState("synthesizing", "正在准备第 "+strconv.Itoa(i+1)+" 句", i)
		if err := rs.waitEntry(entry); err != nil {
			rs.fail(err)
			return
		}
		rs.setState("playing", "正在朗读第 "+strconv.Itoa(i+1)+" 句", i)
		if err := rs.server.playAudioDynamic(rs.ctx, rs.group, entry.path, rs.isPaused); err != nil {
			if rs.ctx.Err() != nil {
				rs.setState("stopped", "朗读已停止", i)
				return
			}
			log.Printf("session playback failed; falling back to file player: %v", err)
			if err := rs.server.playAudio(rs.ctx, rs.group, entry.path); err != nil {
				rs.fail(err)
				return
			}
		}
		rs.removeCache(i)
	}
	rs.setState("done", "朗读完成", len(rs.sentences)-1)
	rs.server.mu.Lock()
	if rs.server.session == rs {
		rs.server.session = nil
	}
	if rs.server.current == rs.group {
		rs.server.current = nil
	}
	rs.server.mu.Unlock()
}

func (rs *readSession) ensurePrefetch(start int) {
	count := rs.prefetchCount(start)
	for i := start; i < start+count; i++ {
		rs.ensureAudio(i)
	}
}

func (rs *readSession) prefetchCount(start int) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	total := len(rs.sentences)
	if start < 0 {
		start = 0
	}
	if start >= total {
		return 0
	}
	count := 0
	runes := 0
	for i := start; i < total && count < prefetchSentenceLimit; i++ {
		count++
		runes += utf8.RuneCountInString(rs.sentences[i].Text)
		if runes >= prefetchTextTarget {
			break
		}
	}
	return count
}

func (rs *readSession) ensureAudio(index int) *audioCacheEntry {
	rs.mu.Lock()
	rate := rs.rate
	version := rs.rateVersion
	if entry := rs.cache[index]; entry != nil && entry.rateVersion == version {
		rs.mu.Unlock()
		return entry
	}
	if old := rs.cache[index]; old != nil {
		go removeWhenReady(old)
	}
	entry := &audioCacheEntry{index: index, rate: rate, rateVersion: version, ready: make(chan struct{})}
	rs.cache[index] = entry
	text := rs.sentences[index].Text
	rs.mu.Unlock()

	go func() {
		req := SpeakRequest{Text: text, Voice: "default", Rate: rate}
		path, err := rs.server.synthesizeSpeech(rs.ctx, rs.group, req)
		entry.path = path
		entry.err = err
		close(entry.ready)
	}()
	return entry
}

func (rs *readSession) waitEntry(entry *audioCacheEntry) error {
	select {
	case <-rs.ctx.Done():
		return rs.ctx.Err()
	case <-entry.ready:
		if entry.err != nil {
			return entry.err
		}
		return nil
	}
}

func (rs *readSession) updateSettings(rate float64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rate <= 0 {
		rate = rs.rate
	}
	rate = clampRate(rate)
	if fmt.Sprintf("%.2f", rate) != fmt.Sprintf("%.2f", rs.rate) {
		rs.rate = rate
		rs.rateVersion++
		for index, entry := range rs.cache {
			if index > rs.currentIndex {
				delete(rs.cache, index)
				go removeWhenReady(entry)
			}
		}
		rs.message = "语速已切换为 " + fmt.Sprintf("%.1fx", rate) + "，将在下一句生效"
	}
	rs.cond.Broadcast()
}

func (rs *readSession) isPaused() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.state == "paused"
}

func (rs *readSession) setPaused(paused bool) {
	rs.mu.Lock()
	if paused {
		rs.state = "paused"
		rs.message = "朗读已暂停"
	} else if rs.state == "paused" {
		rs.state = "playing"
		rs.message = "朗读已继续"
	}
	rs.cond.Broadcast()
	rs.mu.Unlock()
}

func (rs *readSession) setState(state, message string, index int) {
	rs.mu.Lock()
	rs.state = state
	rs.message = message
	rs.currentIndex = index
	rs.cond.Broadcast()
	rs.mu.Unlock()
}

func (rs *readSession) fail(err error) {
	rs.mu.Lock()
	rs.state = "error"
	rs.lastError = friendlyError(err)
	rs.message = rs.lastError
	rs.cond.Broadcast()
	rs.mu.Unlock()
	log.Printf("read session %s failed: %v", rs.id, err)
}

func (rs *readSession) status() map[string]any {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return map[string]any{
		"id":            rs.id,
		"state":         rs.state,
		"message":       rs.message,
		"current_index": rs.currentIndex,
		"total":         len(rs.sentences),
		"rate":          rs.rate,
		"error":         rs.lastError,
	}
}

func (rs *readSession) stop() {
	rs.cancel()
	rs.cleanup()
}

func (rs *readSession) cleanup() {
	rs.mu.Lock()
	entries := rs.cache
	rs.cache = make(map[int]*audioCacheEntry)
	rs.mu.Unlock()
	for _, entry := range entries {
		go removeWhenReady(entry)
	}
}

func (rs *readSession) removeCache(index int) {
	rs.mu.Lock()
	entry := rs.cache[index]
	delete(rs.cache, index)
	rs.mu.Unlock()
	if entry != nil {
		removeWhenReady(entry)
	}
}

func removeWhenReady(entry *audioCacheEntry) {
	if entry == nil {
		return
	}
	<-entry.ready
	if entry.path != "" {
		_ = os.Remove(entry.path)
	}
}

func (s *Server) synthesizeSpeech(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	engine := detectEngine(s.cfg)
	s.engine = engine
	switch engine {
	case "sherpa-onnx":
		req.Rate = clampRate(req.Rate)
		req.Text = preprocessFanchenText(req.Text, req.Rate)
		var lastErr error
		for _, candidate := range ttsTextCandidates(req.Text) {
			req.Text = candidate
			wavPath, err := s.runSherpaVits(ctx, group, req)
			if err == nil {
				return wavPath, nil
			}
			lastErr = err
		}
		return "", lastErr
	default:
		return "", errors.New("no available tts engine")
	}
}

var asciiTokenRE = regexp.MustCompile(`[A-Za-z0-9]+(?:[._+\-][A-Za-z0-9]+)*`)
var percentValueRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*[%％]`)
var specialEnglishTokenRE = regexp.MustCompile(`(?i)\b(WPS|Office)\b`)
var tocLeaderPageRE = regexp.MustCompile(`[.·•…⋯・．\s]{2,}([0-9]+(?:[-–—][0-9]+)?)\s*$`)
var purePageNumberRE = regexp.MustCompile(`^\s*[0-9]+(?:[-–—][0-9]+)?\s*$`)
var wordTocFieldRE = regexp.MustCompile(`(?i)\bTOC\b(?:\s+\\[A-Za-z]+(?:\s+"[^"]*")?)*`)

func (s *Server) runSherpaVits(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", fmt.Errorf("sherpa-onnx failed: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()

	args := []string{
		"--num-threads=" + strconv.Itoa(sherpaNumThreads(s.cfg.Sherpa.NumThreads)),
		"--sid=" + strconv.Itoa(s.cfg.Sherpa.VitsSpeakerID),
		"--vits-model=" + s.cfg.Sherpa.VitsModel,
		"--vits-lexicon=" + s.cfg.Sherpa.VitsLexicon,
		"--vits-tokens=" + s.cfg.Sherpa.VitsTokens,
		"--vits-length-scale=" + fmt.Sprintf("%.2f", vitsLengthScale(req.Rate)),
		"--output-filename=" + tmpPath,
	}
	if fsts := existingRuleFsts(s.cfg.Sherpa.VitsRuleFsts); fsts != "" {
		args = append(args, "--tts-rule-fsts="+fsts)
	}
	args = append(args, req.Text)

	cmd := exec.CommandContext(ctx, s.cfg.Sherpa.Bin, args...)
	cmd.Env = runtimeEnv(os.Environ(), filepath.Join(filepath.Dir(s.cfg.Sherpa.Bin), "lib"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("sherpa-onnx failed: %w", err)
	}
	if err := appendSilenceToWavFile(tmpPath, sentenceEndPauseMs(req.Rate)); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("append sentence pause failed: %w", err)
	}
	return tmpPath, nil
}

func (s *Server) playAudio(ctx context.Context, group *processGroup, wavPath string) error {
	players := prioritizedAudioPlayers(wavPath)
	if len(players) == 0 {
		return errors.New("no available audio player")
	}
	var failures []string
	for _, player := range players {
		if err := runAudioPlayer(ctx, group, wavPath, player); err != nil {
			failures = append(failures, filepath.Base(player.bin)+": "+err.Error())
			log.Printf("audio player %s failed: %v", filepath.Base(player.bin), err)
			continue
		}
		return nil
	}
	return fmt.Errorf("audio playback failed: %s", strings.Join(failures, "; "))
}

func (s *Server) playAudioDynamic(ctx context.Context, group *processGroup, wavPath string, pausedFn func() bool) error {
	aplay := resolveStreamingAplay()
	if aplay == "" {
		return errors.New("streaming aplay is unavailable")
	}
	wav, err := readPCM16Wav(wavPath)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, aplay, "-q", "-t", "raw", "-f", "S16_LE", "-c", strconv.Itoa(int(wav.channels)), "-r", strconv.Itoa(int(wav.sampleRate)), "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	frameBytes := int(wav.channels) * int(wav.bitsPerSample) / 8
	if frameBytes <= 0 {
		frameBytes = 2
	}
	chunkBytes := int(wav.sampleRate) * frameBytes / 20
	if chunkBytes < frameBytes {
		chunkBytes = frameBytes
	}
	chunkBytes -= chunkBytes % frameBytes
	if chunkBytes <= 0 {
		chunkBytes = frameBytes
	}
	for offset := 0; offset < len(wav.data); {
		if err := ctx.Err(); err != nil {
			stdin.Close()
			terminateProcessGroup(cmd)
			_ = cmd.Wait()
			return err
		}
		for pausedFn() {
			if err := ctx.Err(); err != nil {
				stdin.Close()
				terminateProcessGroup(cmd)
				_ = cmd.Wait()
				return err
			}
			time.Sleep(80 * time.Millisecond)
		}
		end := offset + chunkBytes
		if end > len(wav.data) {
			end = len(wav.data)
		}
		if _, err := stdin.Write(wav.data[offset:end]); err != nil {
			stdin.Close()
			_ = cmd.Wait()
			return err
		}
		offset = end
		time.Sleep(50 * time.Millisecond)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

func resolveStreamingAplay() string {
	for _, candidate := range []string{"/usr/bin/aplay", "/bin/aplay", "aplay"} {
		if bin := resolveCommand(candidate); bin != "" {
			return bin
		}
	}
	return ""
}

func runAudioPlayer(ctx context.Context, group *processGroup, wavPath string, player audioPlayer) error {
	if err := prepareAudioFileForPlayer(wavPath, player); err != nil {
		return fmt.Errorf("prepare failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, player.bin, player.args...)
	if len(player.env) > 0 {
		cmd.Env = append(os.Environ(), player.env...)
	}
	if player.credential {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: player.uid, Gid: player.gid},
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	return cmd.Run()
}

func prepareAudioFileForPlayer(wavPath string, player audioPlayer) error {
	if wavPath == "" || !player.credential {
		return nil
	}
	if err := os.Chown(wavPath, int(player.uid), int(player.gid)); err != nil {
		return err
	}
	return os.Chmod(wavPath, 0o600)
}

func (s *Server) probeAudioPlayers(parent context.Context) audioProbeResult {
	result := audioProbeResult{
		Version:  appVersion(),
		ProbedAt: time.Now().Format(time.RFC3339),
	}
	wavPath, err := createProbeWav()
	if err != nil {
		result.Results = append(result.Results, audioProbeItem{Name: "probe-wav", Status: "failed", Message: err.Error()})
		_ = saveAudioProbe(result)
		return result
	}
	defer os.Remove(wavPath)

	for _, player := range resolveAudioPlayers(wavPath) {
		name := filepath.Base(player.bin)
		ctx, cancel := context.WithTimeout(parent, 6*time.Second)
		group := &processGroup{id: "audio-probe-" + name, cancel: cancel}
		err := runAudioPlayer(ctx, group, wavPath, player)
		cancel()
		if err != nil {
			log.Printf("audio probe %s failed: %v", name, err)
			result.Results = append(result.Results, audioProbeItem{
				Name:    name,
				Bin:     player.bin,
				Status:  "failed",
				Message: err.Error(),
			})
			continue
		}
		result.Results = append(result.Results, audioProbeItem{
			Name:   name,
			Bin:    player.bin,
			Status: "ok",
		})
		result.Selected = name
		result.SelectedBin = player.bin
		break
	}
	if err := saveAudioProbe(result); err != nil {
		log.Printf("save audio probe failed: %v", err)
	}
	return result
}

func createProbeWav() (string, error) {
	tmp, err := os.CreateTemp("", "wps-read-aloud-probe-*.wav")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	const sampleRate = 8000
	const channels = 1
	const bitsPerSample = 16
	const samples = sampleRate / 5
	dataSize := samples * channels * bitsPerSample / 8

	if _, err := tmp.Write([]byte("RIFF")); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return "", err
	}
	if _, err := tmp.Write([]byte("WAVEfmt ")); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint32(16)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint16(1)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint16(channels)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint16(channels*bitsPerSample/8)); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint16(bitsPerSample)); err != nil {
		return "", err
	}
	if _, err := tmp.Write([]byte("data")); err != nil {
		return "", err
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint32(dataSize)); err != nil {
		return "", err
	}
	if _, err := tmp.Write(make([]byte, dataSize)); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func loadAudioProbe() audioProbeResult {
	var result audioProbeResult
	data, err := os.ReadFile(audioProbePath())
	if err != nil {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return audioProbeResult{}
	}
	if result.Version != appVersion() {
		return audioProbeResult{}
	}
	return result
}

func appVersion() string {
	info := loadAppInfo()
	if strings.TrimSpace(info.Version) == "" {
		return "dev"
	}
	return info.Version
}

func loadAppInfo() AppInfo {
	data, err := os.ReadFile(appVersionPath())
	if err != nil {
		return AppInfo{}
	}
	var info AppInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return AppInfo{}
	}
	return info
}

func saveAudioProbe(result audioProbeResult) error {
	if err := os.MkdirAll(filepath.Dir(audioProbePath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(audioProbePath(), append(data, '\n'), 0o644)
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
	replacer := strings.NewReplacer(
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
	)
	text = replacer.Replace(text)
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
		parts := make([]string, 0, len(token))
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
			trimTrailingSpaces(&out)
			out = append(out, r)
			out = append(out, punctuationPauseRunes(rate)...)
		case isPairedBoundaryPunctuation(r):
			out = append(out, r)
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

func punctuationPauseRunes(rate float64) []rune {
	pauseMs := standardPauseMs(rate)
	count := (pauseMs + 399) / 400
	if count < 1 {
		count = 1
	}
	if count > 3 {
		count = 3
	}
	out := make([]rune, count)
	for i := range out {
		out[i] = ' '
	}
	return out
}

func trimTrailingSpaces(runes *[]rune) {
	for len(*runes) > 0 && unicode.IsSpace((*runes)[len(*runes)-1]) {
		*runes = (*runes)[:len(*runes)-1]
	}
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
	case '.', '。':
		return "点"
	case '-', '－':
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

func existingRuleFsts(value string) string {
	var kept []string
	for _, item := range strings.Split(value, ",") {
		path := strings.TrimSpace(item)
		if fileExists(path) {
			kept = append(kept, path)
		}
	}
	return strings.Join(kept, ",")
}

type wavPCM struct {
	format        uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	data          []byte
}

func concatenateWavFiles(paths []string, expectedSampleRate int) (string, error) {
	var combined []byte
	var spec *wavPCM
	for _, path := range paths {
		wav, err := readPCM16Wav(path)
		if err != nil {
			return "", err
		}
		if expectedSampleRate > 0 && int(wav.sampleRate) != expectedSampleRate {
			return "", fmt.Errorf("unexpected wav sample rate: %d", wav.sampleRate)
		}
		if spec == nil {
			copySpec := wav
			copySpec.data = nil
			spec = &copySpec
		} else if wav.format != spec.format || wav.channels != spec.channels || wav.sampleRate != spec.sampleRate || wav.bitsPerSample != spec.bitsPerSample {
			return "", errors.New("wav formats do not match")
		}
		combined = append(combined, wav.data...)
	}
	if spec == nil {
		return "", errors.New("no wav files to concatenate")
	}
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := writePCM16Wav(tmp, *spec, combined); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
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

func standardPauseMs(rate float64) int {
	return scaledPauseMs(standardPauseMsAtBaseRate, rate)
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

func prioritizedAudioPlayers(wavPath string) []audioPlayer {
	players := resolveAudioPlayers(wavPath)
	probe := loadAudioProbe()
	if probe.Selected == "" {
		return players
	}
	var preferred []audioPlayer
	var others []audioPlayer
	for _, player := range players {
		name := filepath.Base(player.bin)
		if name == probe.Selected || player.bin == probe.SelectedBin {
			preferred = append(preferred, player)
		} else {
			others = append(others, player)
		}
	}
	return append(preferred, others...)
}

func resolveAudioPlayers(wavPath string) []audioPlayer {
	var players []audioPlayer
	for _, session := range audioSessions() {
		if session.pipewire && commandExists("pw-play") {
			players = append(players, audioPlayer{
				bin:        mustLookPath("pw-play"),
				args:       []string{wavPath},
				env:        audioSessionEnv(session),
				uid:        session.uid,
				gid:        session.gid,
				credential: os.Geteuid() == 0,
			})
		}
		if session.pulse && commandExists("paplay") {
			players = append(players, audioPlayer{
				bin:        mustLookPath("paplay"),
				args:       []string{wavPath},
				env:        append(audioSessionEnv(session), "PULSE_SERVER=unix:"+filepath.Join(session.runtimeDir, "pulse/native")),
				uid:        session.uid,
				gid:        session.gid,
				credential: os.Geteuid() == 0,
			})
		}
	}
	for _, candidate := range []string{"/usr/bin/aplay", "/bin/aplay", "aplay"} {
		if bin := resolveCommand(candidate); bin != "" {
			players = append(players, audioPlayer{bin: bin, args: []string{"-q", wavPath}})
		}
	}
	return players
}

func audioSessionEnv(session audioSession) []string {
	return []string{
		"XDG_RUNTIME_DIR=" + session.runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(session.runtimeDir, "bus"),
	}
}

type audioSession struct {
	runtimeDir string
	uid        uint32
	gid        uint32
	pipewire   bool
	pulse      bool
}

func audioSessions() []audioSession {
	entries, err := os.ReadDir("/run/user")
	if err != nil {
		return nil
	}
	var sessions []audioSession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		runtimeDir := filepath.Join("/run/user", entry.Name())
		info, err := os.Stat(runtimeDir)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		session := audioSession{
			runtimeDir: runtimeDir,
			uid:        stat.Uid,
			gid:        stat.Gid,
			pipewire:   fileExists(filepath.Join(runtimeDir, "pipewire-0")),
			pulse:      fileExists(filepath.Join(runtimeDir, "pulse/native")),
		}
		if session.pipewire || session.pulse {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

func resolveCommand(name string) string {
	if filepath.IsAbs(name) {
		if fileExists(name) {
			return name
		}
		return ""
	}
	if found, err := exec.LookPath(name); err == nil {
		return found
	}
	return ""
}

func commandExists(name string) bool {
	return resolveCommand(name) != ""
}

func mustLookPath(name string) string {
	if found := resolveCommand(name); found != "" {
		return found
	}
	return name
}

func startProcess(group *processGroup, cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	group.mu.Lock()
	group.cmds = append(group.cmds, cmd)
	group.mu.Unlock()
}

func signalProcessGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

func terminateProcessGroup(cmd *exec.Cmd) {
	signalProcessGroup(cmd, syscall.SIGTERM)
	go func() {
		time.Sleep(2 * time.Second)
		signalProcessGroup(cmd, syscall.SIGKILL)
	}()
}

func detectEngine(cfg Config) string {
	if fileExists(cfg.Sherpa.Bin) &&
		fileExists(cfg.Sherpa.VitsModel) &&
		fileExists(cfg.Sherpa.VitsLexicon) &&
		fileExists(cfg.Sherpa.VitsTokens) {
		return "sherpa-onnx"
	}
	return "none"
}

func normalizeVoice(voice string) string {
	switch strings.ToLower(strings.TrimSpace(voice)) {
	case "male", "female", "default", "zh_cn", "zh-cn", "":
		return voice
	default:
		return "default"
	}
}

func healthMessage(engine string) string {
	switch engine {
	case "sherpa-onnx":
		return "Sherpa-onnx VITS fanchen-C TTS engine is available."
	default:
		return "No TTS engine is available. Please reinstall the package or check the bundled engines and voices under the application install directory."
	}
}

func isMostlyLatin(text string) bool {
	var latin, cjk int
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			latin++
		case r >= 0x4E00 && r <= 0x9FFF:
			cjk++
		}
	}
	return latin > 0 && cjk == 0
}

func friendlyError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no available tts engine"):
		return "朗读引擎不可用，请重新安装对应系统的加载项安装包，或联系管理员检查安装目录下的 engines 和 voices。"
	case strings.Contains(msg, "sherpa-onnx failed"):
		return "Sherpa-onnx 语音引擎启动失败，已记录到系统日志。请联系管理员执行 journalctl -u wps-read-aloud-comate.service -n 80 --no-pager 查看原因。"
	case strings.Contains(msg, "no available audio player"):
		return "系统音频播放器不可用，请确认系统已安装 aplay、pw-play 或 paplay，并检查声卡输出是否正常。"
	case strings.Contains(msg, "prepare audio file failed"):
		return "系统音频临时文件权限设置失败，请联系管理员查看朗读服务日志。"
	case strings.Contains(msg, "audio playback failed"):
		return "系统音频播放失败，请检查扬声器、声卡输出和系统音量；如仍失败，请联系管理员查看朗读服务日志。"
	case errors.Is(err, context.Canceled):
		return "朗读已取消。"
	default:
		return "朗读失败，请稍后重试；如果仍失败，请联系管理员查看朗读服务日志。"
	}
}

func runtimeEnv(base []string, libDir string) []string {
	env := append([]string{}, base...)
	if dirExists(libDir) {
		env = append(env, "LD_LIBRARY_PATH="+prependEnv(os.Getenv("LD_LIBRARY_PATH"), libDir))
	}
	return env
}

func prependEnv(current string, first string) string {
	if current == "" {
		return first
	}
	return first + ":" + current
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func clampRate(rate float64) float64 {
	if rate < 0.5 {
		return 0.5
	}
	if rate > 2.0 {
		return 2.0
	}
	return rate
}
func vitsLengthScale(rate float64) float64 {
	return 1.0 / clampRate(rate)
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

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
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
	cfg.Sherpa.Bin = filepath.Clean(cfg.Sherpa.Bin)
	cfg.Sherpa.VitsModel = filepath.Clean(cfg.Sherpa.VitsModel)
	cfg.Sherpa.VitsLexicon = filepath.Clean(cfg.Sherpa.VitsLexicon)
	cfg.Sherpa.VitsTokens = filepath.Clean(cfg.Sherpa.VitsTokens)
	return nil
}
