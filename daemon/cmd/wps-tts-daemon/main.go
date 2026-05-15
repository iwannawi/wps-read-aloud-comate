//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"embed"
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

const AppVersion = "1.0.6"

type Config struct {
	Listen string
	Piper  PiperConfig
	Espeak EspeakConfig
}

type PiperConfig struct {
	Bin   string
	Model string
}

type EspeakConfig struct {
	Bin          string
	Voice        string
	EnglishVoice string
}

type SpeakRequest struct {
	Text   string  `json:"text"`
	Voice  string  `json:"voice"`
	Rate   float64 `json:"rate"`
	Volume int     `json:"volume"`
}

type Server struct {
	cfg     Config
	mu      sync.Mutex
	current *processGroup
	engine  string
}

type processGroup struct {
	id     string
	cancel context.CancelFunc
	cmds   []*exec.Cmd
}

type audioPlayer struct {
	bin        string
	args       []string
	env        []string
	uid        uint32
	gid        uint32
	credential bool
}

func main() {
	configPath := flag.String("config", "/etc/wps-read-aloud/config.yaml", "config file path")
	flag.Parse()

	cfg := defaultConfig()
	if err := loadSimpleYAML(*configPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("load config failed, using defaults: %v", err)
	}

	server := &Server{cfg: cfg, engine: detectEngine(cfg)}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/selftest", server.selftest)
	mux.HandleFunc("/voices", server.voices)
	mux.HandleFunc("/play", server.play)
	mux.HandleFunc("/speak", server.speak)
	mux.HandleFunc("/synthesize", server.synthesize)
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
		Piper: PiperConfig{
			Bin:   "/opt/wps-read-aloud/engines/piper/piper",
			Model: "/opt/wps-read-aloud/voices/zh_CN.onnx",
		},
		Espeak: EspeakConfig{
			Bin:          "/opt/wps-read-aloud/engines/espeak-ng/espeak-ng",
			Voice:        "zh",
			EnglishVoice: "en",
		},
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	engine := detectEngine(s.cfg)
	players := resolveAudioPlayers("", 80)
	playerName := ""
	if len(players) > 0 {
		playerName = filepath.Base(players[0].bin)
	} else if resolveEspeakBin(s.cfg) != "" {
		playerName = "bundled-espeak-ng"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           engine != "none",
		"version":      AppVersion,
		"engine":       engine,
		"audio_player": playerName,
		"message":      healthMessage(engine),
	})
}

func (s *Server) selftest(w http.ResponseWriter, r *http.Request) {
	req := SpeakRequest{
		Text:   "测试",
		Voice:  "zh_CN",
		Rate:   1,
		Volume: 80,
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
			{"id": "zh_CN", "name": "中文普通话"},
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
	fileRequest := r.Clone(r.Context())
	fileRequest.URL.Path = path
	http.FileServer(http.FS(sub)).ServeHTTP(w, fileRequest)
}

func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	allowed := map[string]bool{
		"RELEASE_NOTES.md":       true,
		"ACCEPTANCE_TEST.md":     true,
		"THIRD_PARTY_NOTICES.md": true,
		"SOURCE_OFFER.md":        true,
	}
	if !allowed[name] {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("/usr/share/doc/wps-read-aloud-zhangjingyao", name))
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
	if req.Volume <= 0 || req.Volume > 100 {
		req.Volume = 80
	}
	return req, true
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
	if len(resolveAudioPlayers("", req.Volume)) == 0 {
		err = s.speakDirect(ctx, group, req)
	} else {
		wavPath, err = s.synthesizeSpeech(ctx, group, req)
	}
	if err == nil && wavPath != "" {
		err = s.playAudio(ctx, group, wavPath, req.Volume)
		if err != nil {
			log.Printf("system audio playback failed; trying bundled eSpeak fallback: %v", err)
			err = s.speakDirect(ctx, group, req)
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
	defer s.mu.Unlock()
	if s.current == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "idle"})
		return
	}
	for _, cmd := range s.current.cmds {
		signalProcessGroup(cmd, sig)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

func (s *Server) stopLocked() {
	if s.current == nil {
		return
	}
	s.current.cancel()
	for _, cmd := range s.current.cmds {
		terminateProcessGroup(cmd)
	}
	s.current = nil
}

func (s *Server) synthesizeSpeech(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	engine := detectEngine(s.cfg)
	s.engine = engine
	switch engine {
	case "piper":
		if isMostlyLatin(req.Text) && resolveEspeakBin(s.cfg) != "" {
			return s.runEspeak(ctx, group, req)
		}
		return s.runPiper(ctx, group, req)
	case "espeak-ng":
		return s.runEspeak(ctx, group, req)
	default:
		return "", errors.New("no available tts engine")
	}
}

func (s *Server) runPiper(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", fmt.Errorf("piper failed: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()

	lengthScale := fmt.Sprintf("%.2f", piperLengthScale(req.Rate))
	cmd := exec.CommandContext(ctx, s.cfg.Piper.Bin, "--model", s.cfg.Piper.Model, "--length_scale", lengthScale, "--output_file", tmpPath)
	cmd.Env = runtimeEnv(os.Environ(), filepath.Join(filepath.Dir(s.cfg.Piper.Bin), "lib"), filepath.Join(filepath.Dir(s.cfg.Espeak.Bin), "espeak-ng-data"))
	cmd.Stdin = strings.NewReader(req.Text)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("piper failed: %w", err)
	}
	return tmpPath, nil
}

func (s *Server) runEspeak(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()

	speed := strconv.Itoa(rateToEspeakSpeed(req.Rate))
	amplitude := strconv.Itoa(req.Volume * 2)
	bin := resolveEspeakBin(s.cfg)
	if bin == "" {
		os.Remove(tmpPath)
		return "", errors.New("espeak-ng is unavailable")
	}
	voice := s.cfg.Espeak.Voice
	if isMostlyLatin(req.Text) && s.cfg.Espeak.EnglishVoice != "" {
		voice = s.cfg.Espeak.EnglishVoice
	}
	cmd := exec.CommandContext(ctx, bin, "-v", voice, "-s", speed, "-a", amplitude, "-w", tmpPath, req.Text)
	cmd.Env = runtimeEnv(os.Environ(), filepath.Join(filepath.Dir(bin), "lib"), filepath.Join(filepath.Dir(bin), "espeak-ng-data"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func (s *Server) playAudio(ctx context.Context, group *processGroup, wavPath string, volume int) error {
	_ = volume
	players := resolveAudioPlayers(wavPath, volume)
	if len(players) == 0 {
		return errors.New("no available audio player")
	}
	var failures []string
	for _, player := range players {
		if err := prepareAudioFileForPlayer(wavPath, player); err != nil {
			failures = append(failures, filepath.Base(player.bin)+": prepare failed: "+err.Error())
			continue
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
		if err := cmd.Run(); err != nil {
			failures = append(failures, filepath.Base(player.bin)+": "+err.Error())
			log.Printf("audio player %s failed: %v", filepath.Base(player.bin), err)
			continue
		}
		return nil
	}
	return fmt.Errorf("audio playback failed: %s", strings.Join(failures, "; "))
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

func (s *Server) speakDirect(ctx context.Context, group *processGroup, req SpeakRequest) error {
	speed := strconv.Itoa(rateToEspeakSpeed(req.Rate))
	amplitude := strconv.Itoa(req.Volume * 2)
	bin := resolveEspeakBin(s.cfg)
	if bin == "" {
		return errors.New("no available audio player")
	}
	voice := s.cfg.Espeak.Voice
	if isMostlyLatin(req.Text) && s.cfg.Espeak.EnglishVoice != "" {
		voice = s.cfg.Espeak.EnglishVoice
	}
	cmd := exec.CommandContext(ctx, bin, "-v", voice, "-s", speed, "-a", amplitude, req.Text)
	cmd.Env = runtimeEnv(os.Environ(), filepath.Join(filepath.Dir(bin), "lib"), filepath.Join(filepath.Dir(bin), "espeak-ng-data"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("audio playback failed: %w", err)
	}
	return nil
}

func resolveAudioPlayers(wavPath string, volume int) []audioPlayer {
	_ = volume
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
	group.cmds = append(group.cmds, cmd)
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
	if fileExists(cfg.Piper.Bin) && fileExists(cfg.Piper.Model) {
		return "piper"
	}
	if resolveEspeakBin(cfg) != "" {
		return "espeak-ng"
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
	case "piper":
		return "Piper engine is available."
	case "espeak-ng":
		return "Piper is unavailable; eSpeak NG fallback is available."
	default:
		return "No TTS engine is available. Please reinstall the package or check /opt/wps-read-aloud/engines and /opt/wps-read-aloud/voices."
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
		return "朗读引擎不可用，请重新安装加载项安装包，或联系管理员检查 /opt/wps-read-aloud/engines 和 /opt/wps-read-aloud/voices。"
	case strings.Contains(msg, "piper failed"):
		return "Piper 语音引擎启动失败，已记录到系统日志。请联系管理员执行 journalctl -u wps-tts.service -n 80 --no-pager 查看原因。"
	case strings.Contains(msg, "espeak-ng"):
		return "备用语音引擎启动失败，已记录到系统日志。请联系管理员检查安装包是否完整。"
	case strings.Contains(msg, "no available audio player"):
		return "系统音频播放器不可用，请确认系统已安装 aplay、pw-play 或 paplay，并检查声卡输出是否正常。"
	case strings.Contains(msg, "prepare audio file failed"):
		return "系统音频临时文件权限设置失败，请联系管理员查看 wps-tts.service 日志。"
	case strings.Contains(msg, "audio playback failed"):
		return "系统音频播放失败，请检查扬声器、声卡输出和系统音量；如仍失败，请联系管理员查看 wps-tts.service 日志。"
	case errors.Is(err, context.Canceled):
		return "朗读已取消。"
	default:
		return "朗读失败，请稍后重试；如果仍失败，请联系管理员查看 wps-tts.service 日志。"
	}
}

func resolveEspeakBin(cfg Config) string {
	if fileExists(cfg.Espeak.Bin) {
		return cfg.Espeak.Bin
	}
	return ""
}

func runtimeEnv(base []string, libDir string, espeakDataDir string) []string {
	env := append([]string{}, base...)
	if dirExists(libDir) {
		env = append(env, "LD_LIBRARY_PATH="+prependEnv(os.Getenv("LD_LIBRARY_PATH"), libDir))
	}
	if dirExists(espeakDataDir) {
		env = append(env, "ESPEAK_DATA_PATH="+espeakDataDir)
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
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func rateToEspeakSpeed(rate float64) int {
	if rate < 0.6 {
		rate = 0.6
	}
	if rate > 1.6 {
		rate = 1.6
	}
	return int(175 * rate)
}

func piperLengthScale(rate float64) float64 {
	if rate < 0.6 {
		rate = 0.6
	}
	if rate > 1.6 {
		rate = 1.6
	}
	return 1 / rate
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
		line := strings.TrimSpace(raw)
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
		case "piper.bin":
			cfg.Piper.Bin = value
		case "piper.model":
			cfg.Piper.Model = value
		case "espeak.bin":
			cfg.Espeak.Bin = value
		case "espeak.voice":
			cfg.Espeak.Voice = value
		case "espeak.english_voice":
			cfg.Espeak.EnglishVoice = value
		}
	}
	cfg.Piper.Bin = filepath.Clean(cfg.Piper.Bin)
	cfg.Piper.Model = filepath.Clean(cfg.Piper.Model)
	return nil
}
