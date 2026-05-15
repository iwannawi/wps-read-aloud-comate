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
)

//go:embed web
var webFS embed.FS

const AppVersion = "1.0.10"

const audioProbePath = "/var/lib/wps-read-aloud/audio-player.json"

type Config struct {
	Listen string
	Sherpa SherpaConfig
}

type SherpaConfig struct {
	Bin              string
	NumThreads       int
	TargetSampleRate int
	ZhMatchaModel    string
	ZhMatchaVocoder  string
	ZhMatchaLexicon  string
	ZhMatchaTokens   string
	ZhRuleFsts       string
	EnMatchaModel    string
	EnMatchaVocoder  string
	EnMatchaTokens   string
	EnMatchaDataDir  string
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
	mux.HandleFunc("/audio/probe", server.audioProbe)
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
		Sherpa: SherpaConfig{
			Bin:              "/opt/wps-read-aloud/engines/sherpa-onnx/sherpa-onnx-offline-tts",
			NumThreads:       2,
			TargetSampleRate: 22050,
			ZhMatchaModel:    "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/model-steps-3.onnx",
			ZhMatchaVocoder:  "/opt/wps-read-aloud/voices/sherpa/vocos-22khz-univ.onnx",
			ZhMatchaLexicon:  "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/lexicon.txt",
			ZhMatchaTokens:   "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/tokens.txt",
			ZhRuleFsts:       "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/phone.fst,/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/date.fst,/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/number.fst",
			EnMatchaModel:    "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-en_US-ljspeech/model-steps-3.onnx",
			EnMatchaVocoder:  "/opt/wps-read-aloud/voices/sherpa/vocos-22khz-univ.onnx",
			EnMatchaTokens:   "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-en_US-ljspeech/tokens.txt",
			EnMatchaDataDir:  "/opt/wps-read-aloud/voices/sherpa/matcha-icefall-en_US-ljspeech/espeak-ng-data",
		},
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	engine := detectEngine(s.cfg)
	probe := loadAudioProbe()
	players := prioritizedAudioPlayers("", 80)
	playerName := ""
	if probe.Selected != "" {
		playerName = probe.Selected
	} else if len(players) > 0 {
		playerName = filepath.Base(players[0].bin)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           engine != "none",
		"version":      AppVersion,
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
			{"id": "zh_CN", "name": "中文普通话 Sherpa Matcha Baker"},
			{"id": "en_US", "name": "English Sherpa Matcha LJSpeech"},
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
	if err := applyWavVolume(wavPath, req.Volume); err != nil {
		log.Printf("volume adjustment skipped for %s: %v", id, err)
	}

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
		if err := applyWavVolume(wavPath, req.Volume); err != nil {
			log.Printf("volume adjustment skipped for %s: %v", id, err)
		}
		err = s.playAudio(ctx, group, wavPath, req.Volume)
		if err != nil {
			log.Printf("system audio playback failed; re-probing audio players: %v", err)
			probe := s.probeAudioPlayers(context.Background())
			if probe.Selected != "" {
				err = s.playAudio(ctx, group, wavPath, req.Volume)
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
	if !isMostlyLatin(req.Text) {
		req.Text = normalizeMandarinText(req.Text)
	}
	switch engine {
	case "sherpa-onnx":
		return s.runSherpaMixed(ctx, group, req)
	default:
		return "", errors.New("no available tts engine")
	}
}

type textSegment struct {
	lang string
	text string
}

var mixedTextTokenRE = regexp.MustCompile(`[\p{Han}]+|[A-Za-z]+(?:[A-Za-z0-9'._%:/+-]*[A-Za-z0-9])?|[0-9]+(?:[._%:/+-][0-9]+)*|[^\p{Han}A-Za-z0-9]+`)

func (s *Server) runSherpaMixed(ctx context.Context, group *processGroup, req SpeakRequest) (string, error) {
	segments := splitMixedLanguageText(req.Text)
	if len(segments) == 0 {
		return "", errors.New("no available tts text")
	}
	var wavs []string
	for _, segment := range segments {
		segmentReq := req
		segmentReq.Text = segment.text
		wavPath, err := s.runSherpaSegment(ctx, group, segmentReq, segment.lang)
		if err != nil {
			for _, path := range wavs {
				os.Remove(path)
			}
			return "", err
		}
		wavs = append(wavs, wavPath)
	}
	if len(wavs) == 1 {
		return wavs[0], nil
	}
	out, err := concatenateWavFiles(wavs, s.cfg.Sherpa.TargetSampleRate)
	for _, path := range wavs {
		os.Remove(path)
	}
	if err != nil {
		return "", fmt.Errorf("sherpa-onnx failed: %w", err)
	}
	return out, nil
}

func (s *Server) runSherpaSegment(ctx context.Context, group *processGroup, req SpeakRequest, lang string) (string, error) {
	tmp, err := os.CreateTemp("", "wps-read-aloud-*.wav")
	if err != nil {
		return "", fmt.Errorf("sherpa-onnx failed: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()

	args := []string{
		"--num-threads=" + strconv.Itoa(sherpaNumThreads(s.cfg.Sherpa.NumThreads)),
		"--speed=" + fmt.Sprintf("%.2f", clampRate(req.Rate)),
		"--output-filename=" + tmpPath,
	}
	if lang == "en" {
		args = append(args,
			"--matcha-acoustic-model="+s.cfg.Sherpa.EnMatchaModel,
			"--matcha-vocoder="+s.cfg.Sherpa.EnMatchaVocoder,
			"--matcha-tokens="+s.cfg.Sherpa.EnMatchaTokens,
			"--matcha-data-dir="+s.cfg.Sherpa.EnMatchaDataDir,
		)
	} else {
		args = append(args,
			"--matcha-acoustic-model="+s.cfg.Sherpa.ZhMatchaModel,
			"--matcha-vocoder="+s.cfg.Sherpa.ZhMatchaVocoder,
			"--matcha-lexicon="+s.cfg.Sherpa.ZhMatchaLexicon,
			"--matcha-tokens="+s.cfg.Sherpa.ZhMatchaTokens,
		)
		if fsts := existingRuleFsts(s.cfg.Sherpa.ZhRuleFsts); fsts != "" {
			args = append(args, "--tts-rule-fsts="+fsts)
		}
	}
	args = append(args, req.Text)

	cmd := exec.CommandContext(ctx, s.cfg.Sherpa.Bin, args...)
	cmd.Env = runtimeEnv(os.Environ(), filepath.Join(filepath.Dir(s.cfg.Sherpa.Bin), "lib"), s.cfg.Sherpa.EnMatchaDataDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	startProcess(group, cmd)
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("sherpa-onnx failed: %w", err)
	}
	return tmpPath, nil
}

func (s *Server) playAudio(ctx context.Context, group *processGroup, wavPath string, volume int) error {
	_ = volume
	players := prioritizedAudioPlayers(wavPath, volume)
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
		Version:  AppVersion,
		ProbedAt: time.Now().Format(time.RFC3339),
	}
	wavPath, err := createProbeWav()
	if err != nil {
		result.Results = append(result.Results, audioProbeItem{Name: "probe-wav", Status: "failed", Message: err.Error()})
		_ = saveAudioProbe(result)
		return result
	}
	defer os.Remove(wavPath)

	for _, player := range resolveAudioPlayers(wavPath, 80) {
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
	data, err := os.ReadFile(audioProbePath)
	if err != nil {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return audioProbeResult{}
	}
	if result.Version != AppVersion {
		return audioProbeResult{}
	}
	return result
}

func saveAudioProbe(result audioProbeResult) error {
	if err := os.MkdirAll(filepath.Dir(audioProbePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(audioProbePath, append(data, '\n'), 0o644)
}

func applyWavVolume(path string, volume int) error {
	if volume <= 0 {
		volume = 80
	}
	if volume == 80 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return errors.New("unsupported wav format")
	}
	offset := 12
	var audioFormat uint16
	var bitsPerSample uint16
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
				audioFormat = binary.LittleEndian.Uint16(data[chunkData : chunkData+2])
				bitsPerSample = binary.LittleEndian.Uint16(data[chunkData+14 : chunkData+16])
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
	if audioFormat != 1 || bitsPerSample != 16 || dataStart < 0 {
		return errors.New("unsupported wav encoding")
	}
	gain := float64(volume) / 80.0
	end := dataStart + dataSize
	for i := dataStart; i+1 < end; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		scaled := int(float64(sample) * gain)
		if scaled > 32767 {
			scaled = 32767
		}
		if scaled < -32768 {
			scaled = -32768
		}
		binary.LittleEndian.PutUint16(data[i:i+2], uint16(int16(scaled)))
	}
	return os.WriteFile(path, data, 0o600)
}

func normalizeMandarinText(text string) string {
	replacer := strings.NewReplacer(
		"　", " ",
		"0", "零",
		"1", "一",
		"2", "二",
		"3", "三",
		"4", "四",
		"5", "五",
		"6", "六",
		"7", "七",
		"8", "八",
		"9", "九",
		"%", "百分之",
		"℃", "摄氏度",
		"&", "和",
	)
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func splitMixedLanguageText(text string) []textSegment {
	matches := mixedTextTokenRE.FindAllString(text, -1)
	var segments []textSegment
	for _, token := range matches {
		if token == "" {
			continue
		}
		lang := segmentLanguage(token)
		if len(segments) == 0 {
			segments = append(segments, textSegment{lang: lang, text: token})
			continue
		}
		last := &segments[len(segments)-1]
		if lang == "punct" {
			last.text += token
			continue
		}
		if last.lang == lang || last.lang == "punct" {
			last.lang = lang
			last.text += token
			continue
		}
		segments = append(segments, textSegment{lang: lang, text: token})
	}
	out := segments[:0]
	for _, segment := range segments {
		text := strings.TrimSpace(segment.text)
		if text != "" {
			out = append(out, textSegment{lang: segment.lang, text: text})
		}
	}
	return out
}

func segmentLanguage(text string) string {
	var latin, cjk int
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			latin++
		case r >= 0x4E00 && r <= 0x9FFF:
			cjk++
		}
	}
	if latin == 0 && cjk == 0 {
		return "punct"
	}
	if latin > cjk {
		return "en"
	}
	return "zh"
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

func prioritizedAudioPlayers(wavPath string, volume int) []audioPlayer {
	players := resolveAudioPlayers(wavPath, volume)
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
	if fileExists(cfg.Sherpa.Bin) &&
		fileExists(cfg.Sherpa.ZhMatchaModel) &&
		fileExists(cfg.Sherpa.ZhMatchaVocoder) &&
		fileExists(cfg.Sherpa.ZhMatchaLexicon) &&
		fileExists(cfg.Sherpa.ZhMatchaTokens) &&
		fileExists(cfg.Sherpa.EnMatchaModel) &&
		fileExists(cfg.Sherpa.EnMatchaVocoder) &&
		fileExists(cfg.Sherpa.EnMatchaTokens) &&
		dirExists(cfg.Sherpa.EnMatchaDataDir) {
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
		return "Sherpa-onnx Matcha TTS engine is available."
	default:
		return "No TTS engine is available. Please reinstall the package or check /opt/wps-read-aloud/engines/sherpa-onnx and /opt/wps-read-aloud/voices/sherpa."
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
	case strings.Contains(msg, "sherpa-onnx failed"):
		return "Sherpa-onnx 语音引擎启动失败，已记录到系统日志。请联系管理员执行 journalctl -u wps-tts.service -n 80 --no-pager 查看原因。"
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

func runtimeEnv(base []string, libDir string, dataDir string) []string {
	env := append([]string{}, base...)
	if dirExists(libDir) {
		env = append(env, "LD_LIBRARY_PATH="+prependEnv(os.Getenv("LD_LIBRARY_PATH"), libDir))
	}
	if dirExists(dataDir) {
		env = append(env, "ESPEAK_DATA_PATH="+dataDir)
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

func clampRate(rate float64) float64 {
	if rate < 0.6 {
		return 0.6
	}
	if rate > 1.6 {
		return 1.6
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
		case "sherpa.zh_matcha_model":
			cfg.Sherpa.ZhMatchaModel = value
		case "sherpa.zh_matcha_vocoder":
			cfg.Sherpa.ZhMatchaVocoder = value
		case "sherpa.zh_matcha_lexicon":
			cfg.Sherpa.ZhMatchaLexicon = value
		case "sherpa.zh_matcha_tokens":
			cfg.Sherpa.ZhMatchaTokens = value
		case "sherpa.zh_rule_fsts":
			cfg.Sherpa.ZhRuleFsts = value
		case "sherpa.en_matcha_model":
			cfg.Sherpa.EnMatchaModel = value
		case "sherpa.en_matcha_vocoder":
			cfg.Sherpa.EnMatchaVocoder = value
		case "sherpa.en_matcha_tokens":
			cfg.Sherpa.EnMatchaTokens = value
		case "sherpa.en_matcha_data_dir":
			cfg.Sherpa.EnMatchaDataDir = value
		}
	}
	cfg.Sherpa.Bin = filepath.Clean(cfg.Sherpa.Bin)
	cfg.Sherpa.ZhMatchaModel = filepath.Clean(cfg.Sherpa.ZhMatchaModel)
	cfg.Sherpa.ZhMatchaVocoder = filepath.Clean(cfg.Sherpa.ZhMatchaVocoder)
	cfg.Sherpa.ZhMatchaLexicon = filepath.Clean(cfg.Sherpa.ZhMatchaLexicon)
	cfg.Sherpa.ZhMatchaTokens = filepath.Clean(cfg.Sherpa.ZhMatchaTokens)
	cfg.Sherpa.EnMatchaModel = filepath.Clean(cfg.Sherpa.EnMatchaModel)
	cfg.Sherpa.EnMatchaVocoder = filepath.Clean(cfg.Sherpa.EnMatchaVocoder)
	cfg.Sherpa.EnMatchaTokens = filepath.Clean(cfg.Sherpa.EnMatchaTokens)
	cfg.Sherpa.EnMatchaDataDir = filepath.Clean(cfg.Sherpa.EnMatchaDataDir)
	return nil
}
