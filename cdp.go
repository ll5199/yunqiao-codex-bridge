package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:embed inject.js
var rendererInjection string

//go:embed native_menu.js
var nativeMenuInjection string

type cdpTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

const rendererBridgeVersion = "1.4.5"

func rendererExpression(models []string, defaultModel string) string {
	modelJSON, _ := json.Marshal(models)
	defaultJSON, _ := json.Marshal(defaultModel)
	return "window.__YUNQIAO_INJECT_MODELS__=" + string(modelJSON) +
		";window.__YUNQIAO_INJECT_DEFAULT__=" + string(defaultJSON) + ";\n" + rendererInjection
}

func injectIntoCodex(port int, models []string, defaultModel string, progress func(string)) error {
	if len(models) == 0 {
		return errors.New("没有可注入的模型")
	}
	expression := rendererExpression(models, defaultModel)

	client := &http.Client{Timeout: 1500 * time.Millisecond}
	deadline := time.Now().Add(28 * time.Second)
	injected := make(map[string]bool)
	var firstSuccess time.Time
	var lastError error

	for time.Now().Before(deadline) {
		targets, err := listCDPTargets(client, port)
		if err != nil {
			lastError = err
			time.Sleep(300 * time.Millisecond)
			continue
		}
		for _, target := range targets {
			if target.WebSocketDebuggerURL == "" || injected[target.ID] ||
				(target.Type != "page" && target.Type != "webview") ||
				strings.HasPrefix(target.URL, "devtools://") {
				continue
			}
			if progress != nil {
				progress("正在向官方 Codex 注入模型列表…")
			}
			if err := injectTarget(target.WebSocketDebuggerURL, expression); err != nil {
				lastError = err
				continue
			}
			injected[target.ID] = true
			if firstSuccess.IsZero() {
				firstSuccess = time.Now()
			}
		}
		if !firstSuccess.IsZero() && time.Since(firstSuccess) > 3*time.Second {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if len(injected) > 0 {
		return nil
	}
	if lastError != nil {
		return fmt.Errorf("没有连接到 Codex 调试页面：%w", lastError)
	}
	return errors.New("没有发现 Codex 调试页面；请确认官方 Codex 已完全退出后再启动")
}

func maintainCodexInjection(port int, models []string, defaultModel string, done <-chan struct{}, logger func(string, string)) {
	if len(models) == 0 {
		return
	}
	expression := rendererExpression(models, defaultModel)
	healthExpression := rendererHealthExpression(len(models), defaultModel)
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	failures := make(map[string]int)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		targets, err := listCDPTargets(client, port)
		if err != nil {
			continue
		}
		for _, target := range targets {
			if target.WebSocketDebuggerURL == "" ||
				(target.Type != "page" && target.Type != "webview") ||
				strings.HasPrefix(target.URL, "devtools://") {
				continue
			}
			healthy, healthErr := targetBridgeHealthy(target.WebSocketDebuggerURL, healthExpression)
			if healthErr == nil && healthy {
				delete(failures, target.ID)
				continue
			}
			if err := injectTarget(target.WebSocketDebuggerURL, expression); err != nil {
				failures[target.ID]++
				count := failures[target.ID]
				if logger != nil && (count == 1 || count%10 == 0) {
					logger("bridge.reinject_failed", fmt.Sprintf("target=%s error=%s", safeLogID(target.ID), err.Error()))
				}
				continue
			}
			delete(failures, target.ID)
			if logger != nil {
				logger("bridge.reinjected", fmt.Sprintf("target=%s", safeLogID(target.ID)))
			}
		}
	}
}

func rendererHealthExpression(modelCount int, defaultModel string) string {
	defaultJSON, _ := json.Marshal(defaultModel)
	return fmt.Sprintf("(() => { const s=window.__yunqiaoCodexBridgeStatus; return window.__yunqiaoCodexBridgeInstalled===%q && s?.installed===true && s.models===%d && window.__yunqiaoCodexDefaultModel===%s && Date.now()-Number(s.heartbeat||0)<15000; })()", rendererBridgeVersion, modelCount, defaultJSON)
}

func targetBridgeHealthy(webSocketURL, expression string) (bool, error) {
	socket, err := dialWebSocket(webSocketURL)
	if err != nil {
		return false, err
	}
	defer socket.Close()
	result, err := socket.command("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return false, err
	}
	if exception, ok := result["exceptionDetails"]; ok && exception != nil {
		return false, nil
	}
	remote, _ := result["result"].(map[string]any)
	value, _ := remote["value"].(bool)
	return value, nil
}

func listCDPTargets(client *http.Client, port int) ([]cdpTarget, error) {
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DevTools HTTP %d", response.StatusCode)
	}
	var targets []cdpTarget
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func injectTarget(webSocketURL, expression string) error {
	socket, err := dialWebSocket(webSocketURL)
	if err != nil {
		return err
	}
	defer socket.Close()

	_, pageError := socket.command("Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": expression,
	})
	result, err := socket.command("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"awaitPromise":  false,
		"returnByValue": true,
	})
	if err != nil {
		return fmt.Errorf("执行页面注入失败：%w", err)
	}
	if exception, ok := result["exceptionDetails"]; ok && exception != nil {
		encoded, _ := json.Marshal(exception)
		return fmt.Errorf("页面脚本异常：%s", encoded)
	}
	if pageError != nil {
		// Current page was injected successfully. A future reload may require
		// launching through the bridge again when this target lacks Page domain.
		return nil
	}
	return nil
}

func localizeNativeMenu(port int) error {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	deadline := time.Now().Add(14 * time.Second)
	var lastError error
	for time.Now().Before(deadline) {
		targets, err := listCDPTargets(client, port)
		if err != nil {
			lastError = err
			time.Sleep(400 * time.Millisecond)
			continue
		}
		var selected *cdpTarget
		for index := range targets {
			if targets[index].Type == "node" && targets[index].WebSocketDebuggerURL != "" {
				selected = &targets[index]
				break
			}
		}
		if selected == nil {
			for index := range targets {
				if targets[index].WebSocketDebuggerURL != "" {
					selected = &targets[index]
					break
				}
			}
		}
		if selected == nil {
			lastError = errors.New("没有发现 Electron 主进程")
			time.Sleep(400 * time.Millisecond)
			continue
		}
		socket, err := dialWebSocket(selected.WebSocketDebuggerURL)
		if err != nil {
			lastError = err
			time.Sleep(400 * time.Millisecond)
			continue
		}
		result, commandErr := socket.command("Runtime.evaluate", map[string]any{
			"expression":    nativeMenuInjection,
			"awaitPromise":  true,
			"returnByValue": true,
		})
		socket.Close()
		if commandErr == nil {
			if exception, ok := result["exceptionDetails"]; ok && exception != nil {
				encoded, _ := json.Marshal(exception)
				lastError = fmt.Errorf("菜单脚本异常：%s", encoded)
			} else {
				return nil
			}
		} else {
			lastError = commandErr
		}
		time.Sleep(400 * time.Millisecond)
	}
	if lastError == nil {
		lastError = errors.New("Electron 主进程调试接口不可用")
	}
	return lastError
}

func syncProxyImagesToCodex(port int, proxy *apiProxy) {
	if proxy == nil {
		return
	}
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	sent := make(map[string]map[string]bool)
	deliveryErrors := make(map[string]int)
	for {
		select {
		case <-proxy.done:
			return
		case <-ticker.C:
		}

		images := proxy.store.list()
		if len(images) == 0 {
			continue
		}
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		targets, err := listCDPTargets(client, port)
		if err != nil {
			continue
		}
		for _, target := range targets {
			if target.WebSocketDebuggerURL == "" ||
				(target.Type != "page" && target.Type != "webview") ||
				strings.HasPrefix(target.URL, "devtools://") {
				continue
			}
			targetSent := sent[target.ID]
			if targetSent == nil {
				targetSent = make(map[string]bool)
				sent[target.ID] = targetSent
			}
			pending := make([]proxyImage, 0, len(images))
			for _, image := range images {
				isNewUnassigned := image.ConversationKey == "" && image.CreatedAt >= proxy.startedAt-5000
				if isNewUnassigned && !targetSent[image.ID] {
					pending = append(pending, image)
				}
			}
			if len(pending) == 0 {
				continue
			}
			payload, err := json.Marshal(pending)
			if err != nil {
				continue
			}
			expression := "(() => { if (typeof window.__yunqiaoAcceptProxyImages !== 'function') " +
				"throw new Error('Yunqiao renderer bridge is not ready'); " +
				"return window.__yunqiaoAcceptProxyImages(" + string(payload) + "); })()"
			if err := evaluateTarget(target.WebSocketDebuggerURL, expression); err != nil {
				deliveryErrors[target.ID]++
				count := deliveryErrors[target.ID]
				if proxy.logger != nil && (count == 1 || count == 10 || count%100 == 0) {
					proxy.logger("image.delivery_error", fmt.Sprintf("target=%s error=%s", safeLogID(target.ID), err.Error()))
				}
				continue
			}
			delete(deliveryErrors, target.ID)
			for _, image := range pending {
				targetSent[image.ID] = true
			}
			if proxy.logger != nil {
				proxy.logger("image.delivered", fmt.Sprintf("target=%s count=%d", safeLogID(target.ID), len(pending)))
			}
		}
	}
}

func evaluateTarget(webSocketURL, expression string) error {
	socket, err := dialWebSocket(webSocketURL)
	if err != nil {
		return err
	}
	defer socket.Close()
	result, err := socket.command("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"awaitPromise":  false,
		"returnByValue": true,
	})
	if err != nil {
		return err
	}
	if exception, ok := result["exceptionDetails"]; ok && exception != nil {
		encoded, _ := json.Marshal(exception)
		return fmt.Errorf("页面脚本异常：%s", encoded)
	}
	return nil
}

type websocketClient struct {
	connection net.Conn
	reader     *bufio.Reader
	nextID     int
}

func dialWebSocket(rawURL string) (*websocketClient, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" {
		return nil, errors.New("只接受本机 ws:// DevTools 地址")
	}
	host := parsed.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(context.Background(), "tcp", host)
	if err != nil {
		return nil, err
	}
	client := &websocketClient{connection: connection, reader: bufio.NewReader(connection)}
	if err := client.handshake(parsed); err != nil {
		connection.Close()
		return nil, err
	}
	return client, nil
}

func (client *websocketClient) handshake(parsed *url.URL) error {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Origin: http://127.0.0.1:" + parsed.Port() + "\r\n\r\n"
	if _, err := io.WriteString(client.connection, request); err != nil {
		return err
	}
	status, err := client.reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(status, " 101 ") {
		return fmt.Errorf("WebSocket 握手失败：%s", strings.TrimSpace(status))
	}
	headers := make(map[string]string)
	for {
		line, err := client.reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok {
			headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
		}
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	expected := base64.StdEncoding.EncodeToString(sum[:])
	if headers["sec-websocket-accept"] != expected {
		return errors.New("WebSocket 服务端校验失败")
	}
	return nil
}

func (client *websocketClient) Close() error {
	_ = client.writeFrame(8, nil)
	return client.connection.Close()
}

func (client *websocketClient) command(method string, params any) (map[string]any, error) {
	client.nextID++
	id := client.nextID
	payload, err := json.Marshal(map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	})
	if err != nil {
		return nil, err
	}
	_ = client.connection.SetDeadline(time.Now().Add(8 * time.Second))
	if err := client.writeFrame(1, payload); err != nil {
		return nil, err
	}
	for {
		message, err := client.readMessage()
		if err != nil {
			return nil, err
		}
		var response map[string]any
		if json.Unmarshal(message, &response) != nil {
			continue
		}
		responseID, _ := response["id"].(float64)
		if int(responseID) != id {
			continue
		}
		if cdpError, ok := response["error"]; ok {
			encoded, _ := json.Marshal(cdpError)
			return nil, errors.New(string(encoded))
		}
		if result, ok := response["result"].(map[string]any); ok {
			return result, nil
		}
		return map[string]any{}, nil
	}
}

func (client *websocketClient) writeFrame(opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 65535:
		header = append(header, 0x80|126, byte(length>>8), byte(length))
	default:
		header = append(header, 0x80|127)
		size := make([]byte, 8)
		binary.BigEndian.PutUint64(size, uint64(length))
		header = append(header, size...)
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header = append(header, mask...)
	masked := make([]byte, length)
	for index := range payload {
		masked[index] = payload[index] ^ mask[index%4]
	}
	if _, err := client.connection.Write(header); err != nil {
		return err
	}
	_, err := client.connection.Write(masked)
	return err
}

func (client *websocketClient) readMessage() ([]byte, error) {
	var combined []byte
	started := false
	for {
		first, err := client.reader.ReadByte()
		if err != nil {
			return nil, err
		}
		second, err := client.reader.ReadByte()
		if err != nil {
			return nil, err
		}
		final := first&0x80 != 0
		opcode := first & 0x0f
		masked := second&0x80 != 0
		length := uint64(second & 0x7f)
		switch length {
		case 126:
			var buffer [2]byte
			if _, err := io.ReadFull(client.reader, buffer[:]); err != nil {
				return nil, err
			}
			length = uint64(binary.BigEndian.Uint16(buffer[:]))
		case 127:
			var buffer [8]byte
			if _, err := io.ReadFull(client.reader, buffer[:]); err != nil {
				return nil, err
			}
			length = binary.BigEndian.Uint64(buffer[:])
		}
		if length > 16<<20 {
			return nil, errors.New("WebSocket 消息过大：" + strconv.FormatUint(length, 10))
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(client.reader, mask[:]); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(client.reader, payload); err != nil {
			return nil, err
		}
		if masked {
			for index := range payload {
				payload[index] ^= mask[index%4]
			}
		}
		switch opcode {
		case 8:
			return nil, errors.New("WebSocket 已关闭")
		case 9:
			if err := client.writeFrame(10, payload); err != nil {
				return nil, err
			}
			continue
		case 1:
			combined = append(combined[:0], payload...)
			started = true
		case 0:
			if started {
				combined = append(combined, payload...)
			}
		default:
			continue
		}
		if final && started {
			return combined, nil
		}
	}
}
