package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"
)

type ConnectRequest struct {
	Host     string
	Port     int
	User     string
	Password string
	KeyPath  string
	Cols     int
	Rows     int
	Logging  bool
}

type ResizeRequest struct {
	Cols int
	Rows int
}

type SSHStateEvent struct {
	Connected bool   `json:"connected"`
	Message   string `json:"message"`
}

type QuickCommandResult struct {
	Title   string `json:"title"`
	Command string `json:"command"`
	Output  string `json:"output"`
}

type QuickCommandDefinition struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Builtin bool   `json:"builtin,omitempty"`
	Source  string `json:"source,omitempty"`
}

type JSONShellOption struct {
	FileName string `json:"fileName"`
	Label    string `json:"label"`
}

type CommandRequest struct {
	Title   string `json:"title"`
	Command string `json:"command"`
}

type ServerRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Password    string `json:"password"`
	KeyPath     string `json:"keyPath"`
	LastUsedAt  string `json:"lastUsedAt"`
	Description string `json:"description"`
}

type SSHService struct {
	app *application.App

	mu sync.Mutex

	client         *ssh.Client
	session        *ssh.Session
	stdin          io.WriteCloser
	outputDone     chan struct{}
	outputEventBuf []byte
	sessionDone    chan struct{}
	connectSeq     uint64
	nextConnectSeq uint64
	connectCancel  context.CancelFunc

	secondaryClient         *ssh.Client
	secondarySession        *ssh.Session
	secondaryStdin          io.WriteCloser
	secondaryOutputDone     chan struct{}
	secondaryOutputEventBuf []byte
	secondarySessionDone    chan struct{}

	terminalDetached bool
	logDir           string
	terminalLogPath  string
	commandLogPath   string
	terminalLogFile  *os.File
	commandLogFile   *os.File
}

const terminalWindowName = "terminal-window"
const outputFlushInterval = 16 * time.Millisecond
const outputFlushMaxBytes = 16 * 1024
const terminalType = "xterm"

var quickCommandIDPattern = regexp.MustCompile(`[^a-z0-9]+`)

func NewSSHService() *SSHService {
	return &SSHService{}
}

func (s *SSHService) SetApp(app *application.App) {
	s.app = app
}

func connectCancelledError() error {
	return fmt.Errorf("SSH 连接已取消")
}

func createSSHSession(ctx context.Context, req ConnectRequest) (*ssh.Client, *ssh.Session, io.WriteCloser, io.Reader, io.Reader, error) {
	authMethod, err := buildAuthMethod(req.Password, req.KeyPath)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if req.Port <= 0 {
		req.Port = 22
	}
	if req.Cols <= 0 {
		req.Cols = 120
	}
	if req.Rows <= 0 {
		req.Rows = 32
	}

	config := &ssh.ClientConfig{
		User:            req.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, nil, nil, connectCancelledError()
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	cancelDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-cancelDone:
		}
	}()
	defer close(cancelDone)

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, nil, nil, connectCancelledError()
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	client := ssh.NewClient(clientConn, chans, reqs)
	if ctx.Err() != nil {
		client.Close()
		return nil, nil, nil, nil, nil, connectCancelledError()
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		if ctx.Err() != nil {
			return nil, nil, nil, nil, nil, connectCancelledError()
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("创建会话失败: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("创建 stdin 管道失败: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty(terminalType, req.Rows, req.Cols, modes); err != nil {
		session.Close()
		client.Close()
		if ctx.Err() != nil {
			return nil, nil, nil, nil, nil, connectCancelledError()
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("申请 PTY 失败: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		if ctx.Err() != nil {
			return nil, nil, nil, nil, nil, connectCancelledError()
		}
		return nil, nil, nil, nil, nil, fmt.Errorf("启动远端 shell 失败: %w", err)
	}
	if ctx.Err() != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, nil, nil, connectCancelledError()
	}

	return client, session, stdin, stdout, stderr, nil
}

func (s *SSHService) beginConnectAttempt(cancel context.CancelFunc) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextConnectSeq++
	s.connectSeq = s.nextConnectSeq
	s.connectCancel = cancel
	return s.connectSeq
}

func (s *SSHService) clearConnectAttempt(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connectSeq == seq {
		s.connectSeq = 0
		s.connectCancel = nil
	}
}

func (s *SSHService) Connect(req ConnectRequest) error {
	if req.Host == "" || req.User == "" {
		return fmt.Errorf("host 和 user 不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())
	seq := s.beginConnectAttempt(cancel)
	defer cancel()
	defer s.clearConnectAttempt(seq)

	client, session, stdin, stdout, stderr, err := createSSHSession(ctx, req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.connectSeq != seq {
		s.mu.Unlock()
		_ = session.Close()
		_ = client.Close()
		return connectCancelledError()
	}
	s.closeLocked()
	s.client = client
	s.session = session
	s.stdin = stdin
	s.logDir = ""
	s.terminalLogPath = ""
	s.commandLogPath = ""
	s.terminalLogFile = nil
	s.commandLogFile = nil
	s.outputEventBuf = nil
	s.outputDone = make(chan struct{})
	s.sessionDone = make(chan struct{})
	s.connectSeq = 0
	s.connectCancel = nil
	s.mu.Unlock()

	if req.Logging {
		if err := s.initConnectionLogs(req); err != nil {
			_ = s.Disconnect()
			return err
		}
	}

	_ = s.TouchServerRecord(req.Host, req.Port, req.User, req.Password, req.KeyPath)
	s.emitState(true, fmt.Sprintf("已连接到 %s@%s:%d", req.User, req.Host, req.Port))

	go s.flushOutputEvents()
	go s.streamOutput(stdout)
	go s.streamOutput(stderr)
	go s.waitForExit(session, s.sessionDone)

	return nil
}

func (s *SSHService) ConnectSecondary(req ConnectRequest) error {
	if req.Host == "" || req.User == "" {
		return fmt.Errorf("host 和 user 不能为空")
	}

	client, session, stdin, stdout, stderr, err := createSSHSession(context.Background(), req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.closeSecondaryLocked()
	s.secondaryClient = client
	s.secondarySession = session
	s.secondaryStdin = stdin
	s.secondaryOutputEventBuf = nil
	s.secondaryOutputDone = make(chan struct{})
	s.secondarySessionDone = make(chan struct{})
	s.mu.Unlock()

	s.emitSecondaryState(true, fmt.Sprintf("已连接到 %s@%s:%d", req.User, req.Host, req.Port))

	go s.flushSecondaryOutputEvents()
	go s.streamSecondaryOutput(stdout)
	go s.streamSecondaryOutput(stderr)
	go s.waitForSecondaryExit(session, s.secondarySessionDone)

	return nil
}

func (s *SSHService) SendInput(data string) error {
	s.mu.Lock()
	stdin := s.stdin
	s.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("当前没有活动连接")
	}

	_, err := io.WriteString(stdin, data)
	if err != nil {
		return fmt.Errorf("写入远端终端失败: %w", err)
	}
	return nil
}

func (s *SSHService) SendSecondaryInput(data string) error {
	s.mu.Lock()
	stdin := s.secondaryStdin
	s.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("当前没有活动独立终端连接")
	}

	_, err := io.WriteString(stdin, data)
	if err != nil {
		return fmt.Errorf("写入独立终端失败: %w", err)
	}
	return nil
}

func (s *SSHService) Resize(req ResizeRequest) error {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()

	if session == nil {
		return nil
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		return nil
	}

	if err := session.WindowChange(req.Rows, req.Cols); err != nil {
		return fmt.Errorf("同步终端尺寸失败: %w", err)
	}
	return nil
}

func (s *SSHService) ResizeSecondary(req ResizeRequest) error {
	s.mu.Lock()
	session := s.secondarySession
	s.mu.Unlock()

	if session == nil {
		return nil
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		return nil
	}

	if err := session.WindowChange(req.Rows, req.Cols); err != nil {
		return fmt.Errorf("同步独立终端尺寸失败: %w", err)
	}
	return nil
}

func (s *SSHService) Disconnect() error {
	s.mu.Lock()
	cancel := s.connectCancel
	s.connectSeq = 0
	s.connectCancel = nil
	if s.session == nil && s.client == nil {
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil
	}
	stdin := s.stdin
	done := s.sessionDone
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if stdin != nil {
		_ = stdin.Close()
	}

	if done != nil {
		select {
		case <-done:
			return nil
		case <-time.After(600 * time.Millisecond):
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil && s.client == nil {
		return nil
	}
	s.closeLocked()
	s.emitState(false, "连接已断开")
	return nil
}

func (s *SSHService) CancelConnect() {
	s.mu.Lock()
	cancel := s.connectCancel
	s.connectSeq = 0
	s.connectCancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (s *SSHService) DisconnectSecondary() error {
	s.mu.Lock()
	if s.secondarySession == nil && s.secondaryClient == nil {
		s.mu.Unlock()
		return nil
	}
	stdin := s.secondaryStdin
	done := s.secondarySessionDone
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}

	if done != nil {
		select {
		case <-done:
			return nil
		case <-time.After(600 * time.Millisecond):
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.secondarySession == nil && s.secondaryClient == nil {
		return nil
	}
	s.closeSecondaryLocked()
	s.emitSecondaryState(false, "独立终端已断开")
	return nil
}

func (s *SSHService) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session != nil && s.client != nil
}

func (s *SSHService) SecondaryConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secondarySession != nil && s.secondaryClient != nil
}

func (s *SSHService) GetTerminalDetached() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminalDetached
}

func (s *SSHService) OpenTerminalWindow() error {
	if s.app == nil {
		return fmt.Errorf("应用未初始化")
	}

	s.setTerminalDetached(true)

	if window, exists := s.app.Window.GetByName(terminalWindowName); exists {
		window.Show()
		window.Focus()
		return nil
	}

	s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             terminalWindowName,
		Title:            "LinuxSafeTools Terminal",
		Width:            1240,
		Height:           860,
		MinWidth:         1000,
		MinHeight:        680,
		Frameless:        true,
		BackgroundColour: application.NewRGB(13, 19, 33),
		URL:              "/?window=terminal",
	})
	return nil
}

func (s *SSHService) ReturnTerminalToMain() {
	s.setTerminalDetached(false)
	if s.app == nil {
		return
	}

	for _, window := range s.app.Window.GetAll() {
		if window.Name() == terminalWindowName {
			continue
		}
		window.Show()
		window.Focus()
		return
	}
}

func (s *SSHService) CloseTerminalWindow() {
	s.setTerminalDetached(false)
	if s.app == nil {
		return
	}
	if window, exists := s.app.Window.GetByName(terminalWindowName); exists {
		window.Close()
	}
}

func (s *SSHService) RunQuickCommand(name string) (*QuickCommandResult, error) {
	client := s.currentClient()
	if client == nil {
		return nil, fmt.Errorf("当前没有活动连接")
	}

	definition, err := s.quickCommandDefinition(name)
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建采集会话失败: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	runErr := make(chan error, 1)
	go func() {
		runErr <- session.Run(definition.Command)
	}()

	select {
	case err := <-runErr:
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n[stderr]\n" + stderr.String()
		}
		if output == "" {
			output = "(无输出)"
		}
		if err != nil {
			return &QuickCommandResult{
				Title:   definition.Name,
				Command: definition.Command,
				Output:  output,
			}, fmt.Errorf("执行失败: %w", err)
		}
		return &QuickCommandResult{
			Title:   definition.Name,
			Command: definition.Command,
			Output:  output,
		}, nil
	case <-time.After(20 * time.Second):
		_ = session.Close()
		return nil, fmt.Errorf("命令执行超时")
	}
}

func (s *SSHService) ListQuickCommands() ([]QuickCommandDefinition, error) {
	definitions, err := s.loadQuickCommands()
	if err != nil {
		return nil, err
	}
	return definitions, nil
}

func (s *SSHService) ListJSONShellFiles() ([]JSONShellOption, error) {
	dir, err := jsonShellDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建 jsonshell 目录失败: %w", err)
		}
		return []JSONShellOption{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 jsonshell 目录失败: %w", err)
	}

	options := make([]JSONShellOption, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != ".json" {
			continue
		}
		options = append(options, JSONShellOption{
			FileName: name,
			Label:    strings.TrimSuffix(name, filepath.Ext(name)),
		})
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].Label < options[j].Label
	})
	return options, nil
}

func (s *SSHService) LoadJSONShellCommands(fileName string) ([]QuickCommandDefinition, error) {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" {
		return []QuickCommandDefinition{}, nil
	}
	if strings.ToLower(filepath.Ext(fileName)) != ".json" {
		return nil, fmt.Errorf("仅支持 json 文件: %s", fileName)
	}

	dir, err := jsonShellDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 jsonshell 文件失败: %w", err)
	}

	definitions, err := parseJSONShellDefinitions(data)
	if err != nil {
		return nil, err
	}

	sourceLabel := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	result := make([]QuickCommandDefinition, 0, len(definitions))
	seen := map[string]int{}
	for index, item := range definitions {
		item.Name = strings.TrimSpace(item.Name)
		item.Command = strings.TrimSpace(item.Command)
		if item.Name == "" || item.Command == "" {
			continue
		}
		item.ID = normalizeQuickCommandID(item.ID, item.Name)
		if item.ID == "" {
			item.ID = fmt.Sprintf("jsonshell-%d", index+1)
		}
		if count, exists := seen[item.ID]; exists {
			count++
			seen[item.ID] = count
			item.ID = fmt.Sprintf("%s-%d", item.ID, count)
		} else {
			seen[item.ID] = 1
		}
		item.Builtin = false
		item.Source = sourceLabel
		result = append(result, item)
	}

	return result, nil
}

func (s *SSHService) AppendCommandToJSONShell(fileName string, definition QuickCommandDefinition) error {
	fileName = normalizeJSONShellFileName(fileName)
	if fileName == "" {
		return fmt.Errorf("扩展 JSON 文件名不能为空")
	}

	definition.Name = strings.TrimSpace(definition.Name)
	definition.Command = strings.TrimSpace(definition.Command)
	if definition.Name == "" || definition.Command == "" {
		return fmt.Errorf("命令名称和内容不能为空")
	}

	dir, err := jsonShellDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 jsonshell 目录失败: %w", err)
	}

	path := filepath.Join(dir, fileName)
	var existing []QuickCommandDefinition
	data, err := os.ReadFile(path)
	if err == nil {
		existing, err = parseJSONShellDefinitions(data)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 jsonshell 文件失败: %w", err)
	}

	definition.ID = nextQuickCommandID(definition.ID, definition.Name, existing)
	definition.Builtin = false
	definition.Source = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	existing = append(existing, definition)

	data, err = json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 jsonshell 命令失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 jsonshell 文件失败: %w", err)
	}
	return nil
}

func (s *SSHService) AddQuickCommand(definition QuickCommandDefinition) ([]QuickCommandDefinition, error) {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Command = strings.TrimSpace(definition.Command)
	if definition.Name == "" {
		return nil, fmt.Errorf("命令名称不能为空")
	}
	if definition.Command == "" {
		return nil, fmt.Errorf("命令内容不能为空")
	}

	customDefinitions, err := s.loadStoredQuickCommands()
	if err != nil {
		return nil, err
	}

	allDefinitions := append(defaultQuickCommands(), customDefinitions...)
	definition.ID = nextQuickCommandID(definition.ID, definition.Name, allDefinitions)
	for _, item := range allDefinitions {
		if item.ID == definition.ID {
			return nil, fmt.Errorf("命令 ID 已存在: %s", definition.ID)
		}
	}

	definition.Builtin = false
	customDefinitions = append(customDefinitions, definition)
	if err := s.storeStoredQuickCommands(customDefinitions); err != nil {
		return nil, err
	}
	return s.loadQuickCommands()
}

func (s *SSHService) DeleteQuickCommand(id string) ([]QuickCommandDefinition, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("命令 ID 不能为空")
	}
	if isBuiltinQuickCommandID(id) {
		return nil, fmt.Errorf("内置命令不允许删除: %s", id)
	}

	customDefinitions, err := s.loadStoredQuickCommands()
	if err != nil {
		return nil, err
	}

	nextDefinitions := make([]QuickCommandDefinition, 0, len(customDefinitions))
	removed := false
	for _, item := range customDefinitions {
		if item.ID == id {
			removed = true
			continue
		}
		nextDefinitions = append(nextDefinitions, item)
	}
	if !removed {
		return nil, fmt.Errorf("未找到自定义命令: %s", id)
	}

	if err := s.storeStoredQuickCommands(nextDefinitions); err != nil {
		return nil, err
	}
	return s.loadQuickCommands()
}

func (s *SSHService) RunCommand(req CommandRequest) (*QuickCommandResult, error) {
	client := s.currentClient()
	if client == nil {
		return nil, fmt.Errorf("当前没有活动连接")
	}
	if req.Command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}

	title := req.Title
	if title == "" {
		title = "自定义命令"
	}

	return s.runCommand(client, title, req.Command)
}

func (s *SSHService) ListServerRecords() ([]ServerRecord, error) {
	records, err := s.loadServerRecords()
	if err != nil {
		return nil, err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].LastUsedAt > records[j].LastUsedAt
	})
	return records, nil
}

func (s *SSHService) SaveServerRecord(record ServerRecord) ([]ServerRecord, error) {
	if record.Host == "" || record.User == "" {
		return nil, fmt.Errorf("host 和 user 不能为空")
	}

	records, err := s.loadServerRecords()
	if err != nil {
		return nil, err
	}

	if record.Port <= 0 {
		record.Port = 22
	}
	if record.ID == "" {
		record.ID = fmt.Sprintf("%s@%s:%d", record.User, record.Host, record.Port)
	}
	if record.Name == "" {
		record.Name = fmt.Sprintf("%s@%s", record.User, record.Host)
	}
	record.LastUsedAt = time.Now().Format(time.RFC3339)

	updated := false
	for i := range records {
		if records[i].ID == record.ID {
			records[i] = record
			updated = true
			break
		}
	}
	if !updated {
		records = append(records, record)
	}

	if err := s.storeServerRecords(records); err != nil {
		return nil, err
	}
	return s.ListServerRecords()
}

func (s *SSHService) DeleteServerRecord(id string) ([]ServerRecord, error) {
	if id == "" {
		return nil, fmt.Errorf("记录 ID 不能为空")
	}

	records, err := s.loadServerRecords()
	if err != nil {
		return nil, err
	}

	filtered := records[:0]
	for _, record := range records {
		if record.ID != id {
			filtered = append(filtered, record)
		}
	}

	if err := s.storeServerRecords(filtered); err != nil {
		return nil, err
	}
	return s.ListServerRecords()
}

func (s *SSHService) TouchServerRecord(host string, port int, user string, password string, keyPath string) error {
	records, err := s.loadServerRecords()
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%s@%s:%d", user, host, port)
	for i := range records {
		if records[i].ID == id {
			records[i].LastUsedAt = time.Now().Format(time.RFC3339)
			if password != "" {
				records[i].Password = password
			}
			if keyPath != "" {
				records[i].KeyPath = keyPath
			}
			return s.storeServerRecords(records)
		}
	}
	return nil
}

func (s *SSHService) waitForExit(session *ssh.Session, done chan struct{}) {
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	err := session.Wait()

	s.mu.Lock()
	if s.session != session {
		s.mu.Unlock()
		return
	}

	message := "远端 shell 已退出"
	if err != nil && !errors.Is(err, io.EOF) {
		message = fmt.Sprintf("远端 shell 已退出: %v", err)
	}
	s.closeLocked()
	s.mu.Unlock()

	s.emitState(false, message)
}

func (s *SSHService) waitForSecondaryExit(session *ssh.Session, done chan struct{}) {
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	err := session.Wait()

	s.mu.Lock()
	if s.secondarySession != session {
		s.mu.Unlock()
		return
	}

	message := "独立终端已退出"
	if err != nil && !errors.Is(err, io.EOF) {
		message = fmt.Sprintf("独立终端已退出: %v", err)
	}
	s.closeSecondaryLocked()
	s.mu.Unlock()

	s.emitSecondaryState(false, message)
}

func (s *SSHService) streamOutput(reader io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			s.appendTerminalLog(chunk)
			s.emitOutput(chunk)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.emitOutput(fmt.Sprintf("\r\n[output stream error: %v]\r\n", err))
			}
			return
		}
	}
}

func (s *SSHService) streamSecondaryOutput(reader io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			s.emitSecondaryOutput(chunk)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.emitSecondaryOutput(fmt.Sprintf("\r\n[secondary output stream error: %v]\r\n", err))
			}
			return
		}
	}
}

func (s *SSHService) AppendCommandQueryLog(title string, command string, output string) error {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(command) == "" {
		return nil
	}
	return s.appendCommandLog(title, command, output, nil)
}

func (s *SSHService) initConnectionLogs(req ConnectRequest) error {
	baseDir, err := appBaseDir()
	if err != nil {
		return fmt.Errorf("åˆ›å»ºè¿žæŽ¥æ—¥å¿—ç›®å½•å¤±è´¥: %w", err)
	}

	sessionName := fmt.Sprintf("%s_%s_%s", time.Now().Format("20060102_150405"), sanitizeLogName(req.User), sanitizeLogName(req.Host))
	logDir := filepath.Join(baseDir, sessionName)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("åˆ›å»ºè¿žæŽ¥æ—¥å¿—ç›®å½•å¤±è´¥: %w", err)
	}

	terminalLogPath := filepath.Join(logDir, "terminal.txt")
	commandLogPath := filepath.Join(logDir, "commands.txt")
	header := fmt.Sprintf("connected_at: %s\nhost: %s\nport: %d\nuser: %s\n\n", time.Now().Format(time.RFC3339), req.Host, req.Port, req.User)

	terminalLogFile, err := os.OpenFile(terminalLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("åˆå§‹åŒ–ç»ˆç«¯æ—¥å¿—å¤±è´¥: %w", err)
	}
	if _, err := terminalLogFile.WriteString(header); err != nil {
		_ = terminalLogFile.Close()
		return fmt.Errorf("åˆå§‹åŒ–ç»ˆç«¯æ—¥å¿—å¤±è´¥: %w", err)
	}

	commandLogFile, err := os.OpenFile(commandLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_APPEND, 0o644)
	if err != nil {
		_ = terminalLogFile.Close()
		return fmt.Errorf("åˆå§‹åŒ–å‘½ä»¤æŸ¥è¯¢æ—¥å¿—å¤±è´¥: %w", err)
	}
	if _, err := commandLogFile.WriteString(header); err != nil {
		_ = terminalLogFile.Close()
		_ = commandLogFile.Close()
		return fmt.Errorf("åˆå§‹åŒ–å‘½ä»¤æŸ¥è¯¢æ—¥å¿—å¤±è´¥: %w", err)
	}

	s.mu.Lock()
	s.logDir = logDir
	s.terminalLogPath = terminalLogPath
	s.commandLogPath = commandLogPath
	s.terminalLogFile = terminalLogFile
	s.commandLogFile = commandLogFile
	s.mu.Unlock()
	return nil
}

func (s *SSHService) appendTerminalLog(data string) {
	if data == "" {
		return
	}

	s.mu.Lock()
	file := s.terminalLogFile
	s.mu.Unlock()
	if file == nil {
		return
	}

	_, _ = file.WriteString(data)
}

func (s *SSHService) appendCommandLog(title string, command string, output string, runErr error) error {
	s.mu.Lock()
	file := s.commandLogFile
	s.mu.Unlock()
	if file == nil {
		return nil
	}

	var buf strings.Builder
	buf.WriteString("==================================================\n")
	buf.WriteString("time: ")
	buf.WriteString(time.Now().Format(time.RFC3339))
	buf.WriteByte('\n')
	buf.WriteString("title: ")
	buf.WriteString(title)
	buf.WriteByte('\n')
	buf.WriteString("command:\n")
	buf.WriteString(command)
	buf.WriteByte('\n')
	if runErr != nil {
		buf.WriteString("error: ")
		buf.WriteString(runErr.Error())
		buf.WriteByte('\n')
	}
	buf.WriteString("output:\n")
	buf.WriteString(output)
	if !strings.HasSuffix(output, "\n") {
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')

	_, err := file.WriteString(buf.String())
	return err
}

func (s *SSHService) emitOutput(data string) {
	if data == "" {
		return
	}
	s.mu.Lock()
	s.outputEventBuf = append(s.outputEventBuf, data...)
	s.mu.Unlock()
}

func (s *SSHService) emitSecondaryOutput(data string) {
	if data == "" {
		return
	}
	s.mu.Lock()
	s.secondaryOutputEventBuf = append(s.secondaryOutputEventBuf, data...)
	s.mu.Unlock()
}

func (s *SSHService) emitState(connected bool, message string) {
	if s.app != nil {
		s.app.Event.Emit("ssh:state", SSHStateEvent{
			Connected: connected,
			Message:   message,
		})
	}
}

func (s *SSHService) emitSecondaryState(connected bool, message string) {
	if s.app != nil {
		s.app.Event.Emit("ssh:state:secondary", SSHStateEvent{
			Connected: connected,
			Message:   message,
		})
	}
}

func (s *SSHService) setTerminalDetached(detached bool) {
	s.mu.Lock()
	s.terminalDetached = detached
	s.mu.Unlock()

	if s.app != nil {
		s.app.Event.Emit("terminal:detached", detached)
	}
}

func (s *SSHService) closeLocked() {
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}
	if s.session != nil {
		_ = s.session.Close()
		s.session = nil
	}
	if s.client != nil {
		_ = s.client.Close()
		s.client = nil
	}
	if s.terminalLogFile != nil {
		_ = s.terminalLogFile.Close()
		s.terminalLogFile = nil
	}
	if s.commandLogFile != nil {
		_ = s.commandLogFile.Close()
		s.commandLogFile = nil
	}
	if s.outputDone != nil {
		close(s.outputDone)
		s.outputDone = nil
	}
	s.outputEventBuf = nil
	s.sessionDone = nil
	s.logDir = ""
	s.terminalLogPath = ""
	s.commandLogPath = ""
}

func (s *SSHService) closeSecondaryLocked() {
	if s.secondaryStdin != nil {
		_ = s.secondaryStdin.Close()
		s.secondaryStdin = nil
	}
	if s.secondarySession != nil {
		_ = s.secondarySession.Close()
		s.secondarySession = nil
	}
	if s.secondaryClient != nil {
		_ = s.secondaryClient.Close()
		s.secondaryClient = nil
	}
	if s.secondaryOutputDone != nil {
		close(s.secondaryOutputDone)
		s.secondaryOutputDone = nil
	}
	s.secondaryOutputEventBuf = nil
	s.secondarySessionDone = nil
}

func (s *SSHService) flushOutputEvents() {
	s.mu.Lock()
	done := s.outputDone
	app := s.app
	s.mu.Unlock()

	if done == nil || app == nil {
		return
	}

	ticker := time.NewTicker(outputFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushPendingOutput(app)
		case <-done:
			s.flushPendingOutput(app)
			return
		}
	}
}

func (s *SSHService) flushSecondaryOutputEvents() {
	s.mu.Lock()
	done := s.secondaryOutputDone
	app := s.app
	s.mu.Unlock()

	if done == nil || app == nil {
		return
	}

	ticker := time.NewTicker(outputFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushPendingSecondaryOutput(app)
		case <-done:
			s.flushPendingSecondaryOutput(app)
			return
		}
	}
}

func (s *SSHService) flushPendingOutput(app *application.App) {
	s.mu.Lock()
	if len(s.outputEventBuf) == 0 {
		s.mu.Unlock()
		return
	}

	size := len(s.outputEventBuf)
	if size > outputFlushMaxBytes {
		size = outputFlushMaxBytes
	}
	chunk := make([]byte, size)
	copy(chunk, s.outputEventBuf[:size])
	s.outputEventBuf = s.outputEventBuf[size:]
	s.mu.Unlock()

	app.Event.Emit("ssh:output", string(chunk))
}

func (s *SSHService) flushPendingSecondaryOutput(app *application.App) {
	s.mu.Lock()
	if len(s.secondaryOutputEventBuf) == 0 {
		s.mu.Unlock()
		return
	}

	size := len(s.secondaryOutputEventBuf)
	if size > outputFlushMaxBytes {
		size = outputFlushMaxBytes
	}
	chunk := make([]byte, size)
	copy(chunk, s.secondaryOutputEventBuf[:size])
	s.secondaryOutputEventBuf = s.secondaryOutputEventBuf[size:]
	s.mu.Unlock()

	app.Event.Emit("ssh:output:secondary", string(chunk))
}

func (s *SSHService) currentClient() *ssh.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func (s *SSHService) quickCommandDefinition(id string) (*QuickCommandDefinition, error) {
	definitions, err := s.loadQuickCommands()
	if err != nil {
		return nil, err
	}

	for _, definition := range definitions {
		if definition.ID == id {
			if definition.Name == "" || definition.Command == "" {
				return nil, fmt.Errorf("命令配置不完整: %s", id)
			}
			return &definition, nil
		}
	}

	return nil, fmt.Errorf("未找到命令配置: %s", id)
}

func (s *SSHService) loadQuickCommands() ([]QuickCommandDefinition, error) {
	customDefinitions, err := s.loadStoredQuickCommands()
	if err != nil {
		return nil, err
	}

	definitions := append(defaultQuickCommands(), customDefinitions...)
	return definitions, nil
}

func (s *SSHService) loadStoredQuickCommands() ([]QuickCommandDefinition, error) {
	path, err := quickCommandPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := storeQuickCommands(path, nil); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取应急响应命令失败: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		if err := storeQuickCommands(path, nil); err != nil {
			return nil, err
		}
		return nil, nil
	}

	var rawDefinitions []QuickCommandDefinition
	if err := json.Unmarshal(data, &rawDefinitions); err != nil {
		return nil, fmt.Errorf("解析应急响应命令失败: %w", err)
	}

	customDefinitions := make([]QuickCommandDefinition, 0, len(rawDefinitions))
	needsRewrite := false
	for _, definition := range rawDefinitions {
		definition.ID = strings.TrimSpace(definition.ID)
		definition.Name = strings.TrimSpace(definition.Name)
		definition.Command = strings.TrimSpace(definition.Command)
		if definition.ID == "" {
			return nil, fmt.Errorf("应急响应命令存在空 ID")
		}
		if definition.Name == "" || definition.Command == "" {
			return nil, fmt.Errorf("命令配置不完整: %s", definition.ID)
		}
		if isBuiltinQuickCommandID(definition.ID) {
			needsRewrite = true
			continue
		}
		if definition.Builtin {
			needsRewrite = true
		}
		definition.Builtin = false
		customDefinitions = append(customDefinitions, definition)
	}

	if needsRewrite {
		if err := storeQuickCommands(path, customDefinitions); err != nil {
			return nil, err
		}
	}
	return customDefinitions, nil
}

func (s *SSHService) storeStoredQuickCommands(definitions []QuickCommandDefinition) error {
	path, err := quickCommandPath()
	if err != nil {
		return err
	}
	return storeQuickCommands(path, definitions)
}

func storeQuickCommands(path string, definitions []QuickCommandDefinition) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建应急响应命令目录失败: %w", err)
	}

	data, err := json.MarshalIndent(definitions, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化应急响应命令失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入应急响应命令失败: %w", err)
	}
	return nil
}

func defaultQuickCommands() []QuickCommandDefinition {
	return []QuickCommandDefinition{
		{
			ID:      "cpu",
			Name:    "CPU",
			Command: `ps -w -eo pid,ppid,%mem,%cpu,cmd --sort=-%cpu | head -n 11`,
			Builtin: true,
		},
		{
			ID:      "memory",
			Name:    "内存",
			Command: `ps -w -eo pid,ppid,%mem,%cpu,cmd --sort=-%mem | head -n 11`,
			Builtin: true,
		},
		{
			ID:      "process",
			Name:    "进程",
			Command: `ps aux`,
			Builtin: true,
		},
		{
			ID:      "network",
			Name:    "网络连接",
			Command: `sh -lc 'echo "### ip addr"; ip addr 2>/dev/null || ifconfig 2>/dev/null; echo; echo "### ss -tunap"; ss -tunap 2>/dev/null || netstat -tunap 2>/dev/null'`,
			Builtin: true,
		},
		{
			ID:      "tasks",
			Name:    "计划任务",
			Command: `sh -lc 'echo "### current user crontab"; crontab -l 2>/dev/null || echo "no user crontab"; echo; echo "### /etc/cron*"; ls -al /etc/cron* 2>/dev/null; echo; echo "### systemd timers"; systemctl list-timers --all --no-pager 2>/dev/null || echo "systemd timers unavailable"'`,
			Builtin: true,
		},
	}
}

func isBuiltinQuickCommandID(id string) bool {
	for _, definition := range defaultQuickCommands() {
		if definition.ID == id {
			return true
		}
	}
	return false
}

func normalizeQuickCommandID(rawID string, fallbackName string) string {
	value := strings.ToLower(strings.TrimSpace(rawID))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(fallbackName))
	}
	value = quickCommandIDPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func nextQuickCommandID(rawID string, fallbackName string, existing []QuickCommandDefinition) string {
	baseID := normalizeQuickCommandID(rawID, fallbackName)
	if baseID == "" {
		baseID = fmt.Sprintf("custom-%d", time.Now().UnixMilli())
	}
	if !quickCommandIDExists(baseID, existing) {
		return baseID
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", baseID, index)
		if !quickCommandIDExists(candidate, existing) {
			return candidate
		}
	}
}

func quickCommandIDExists(id string, definitions []QuickCommandDefinition) bool {
	for _, definition := range definitions {
		if definition.ID == id {
			return true
		}
	}
	return false
}

func (s *SSHService) runCommand(client *ssh.Client, title string, command string) (*QuickCommandResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建采集会话失败: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	runErr := make(chan error, 1)
	go func() {
		runErr <- session.Run(command)
	}()

	select {
	case err := <-runErr:
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n[stderr]\n" + stderr.String()
		}
		if output == "" {
			output = "(无输出)"
		}
		if err != nil {
			return &QuickCommandResult{
				Title:   title,
				Command: command,
				Output:  output,
			}, fmt.Errorf("执行失败: %w", err)
		}
		return &QuickCommandResult{
			Title:   title,
			Command: command,
			Output:  output,
		}, nil
	case <-time.After(20 * time.Second):
		_ = session.Close()
		return nil, fmt.Errorf("命令执行超时")
	}
}

func (s *SSHService) loadServerRecords() ([]ServerRecord, error) {
	path, err := serverRecordPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []ServerRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取服务器记录失败: %w", err)
	}

	var records []ServerRecord
	if len(data) == 0 {
		return []ServerRecord{}, nil
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("解析服务器记录失败: %w", err)
	}
	return records, nil
}

func (s *SSHService) storeServerRecords(records []ServerRecord) error {
	path, err := serverRecordPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建服务器记录目录失败: %w", err)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化服务器记录失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入服务器记录失败: %w", err)
	}
	return nil
}

func serverRecordPath() (string, error) {
	baseDir, err := appBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "servers.json"), nil
}

func quickCommandPath() (string, error) {
	baseDir, err := appBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "quick_commands.json"), nil
}

func jsonShellDir() (string, error) {
	baseDir, err := appBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "jsonshell"), nil
}

func normalizeJSONShellFileName(fileName string) string {
	name := filepath.Base(strings.TrimSpace(fileName))
	name = sanitizeLogName(strings.TrimSuffix(name, filepath.Ext(name)))
	if name == "" || name == "unknown" {
		return ""
	}
	return name + ".json"
}

func appBaseDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取应用目录失败: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		exePath = resolvedPath
	}
	return filepath.Dir(exePath), nil
}

func sanitizeLogName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "._-")
	if value == "" {
		return "unknown"
	}
	return value
}

func appendFile(path string, data string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(data)
	return err
}

func parseJSONShellDefinitions(data []byte) ([]QuickCommandDefinition, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []QuickCommandDefinition{}, nil
	}

	var definitions []QuickCommandDefinition
	if err := json.Unmarshal(trimmed, &definitions); err == nil {
		return definitions, nil
	}

	var payload struct {
		Commands []QuickCommandDefinition `json:"commands"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("解析 jsonshell 命令失败: %w", err)
	}
	return payload.Commands, nil
}

func buildAuthMethod(password, keyPath string) (ssh.AuthMethod, error) {
	switch {
	case password != "":
		return ssh.Password(password), nil
	case keyPath != "":
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥失败: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}

		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("必须提供 password 或 key")
	}
}
