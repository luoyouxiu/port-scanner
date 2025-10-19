package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PortScanner struct {
	timeout time.Duration
}

type ScanResult struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Open    bool   `json:"open"`
	Service string `json:"service"`
	Error   string `json:"error,omitempty"`
}

type ScanRequest struct {
	Target string `json:"target"`
	Ports  string `json:"ports"` // 可以是单个端口、端口范围(1-1000)或"all"
}

type ProgressMessage struct {
	Type      string `json:"type"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Percent   int    `json:"percent"`
	OpenPorts int    `json:"openPorts"`
	Speed     int    `json:"speed"`               // 每秒扫描的端口数
	BatchInfo string `json:"batchInfo,omitempty"` // 批次信息
}

type ScanSession struct {
	ID        string
	Progress  chan ProgressMessage
	Results   []ScanResult
	StartTime time.Time
	mu        sync.RWMutex
}

var sessions = make(map[string]*ScanSession)
var sessionsMu sync.RWMutex

func NewPortScanner(timeout time.Duration) *PortScanner {
	return &PortScanner{timeout: timeout}
}

// 解析端口范围
func (ps *PortScanner) parsePorts(portStr string) ([]int, error) {
	if portStr == "all" {
		// 扫描常用端口
		return []int{21, 22, 23, 25, 53, 80, 110, 143, 443, 993, 995, 3389, 5432, 3306, 6379, 27017}, nil
	}

	var ports []int
	parts := strings.Split(portStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			// 端口范围
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的端口范围: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("无效的起始端口: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("无效的结束端口: %s", rangeParts[1])
			}

			if start > end {
				return nil, fmt.Errorf("起始端口不能大于结束端口: %s", part)
			}

			for i := start; i <= end; i++ {
				ports = append(ports, i)
			}
		} else {
			// 单个端口
			port, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("无效的端口号: %s", part)
			}
			ports = append(ports, port)
		}
	}

	return ports, nil
}

// 获取端口对应的服务名
func (ps *PortScanner) getServiceName(port int) string {
	services := map[int]string{
		21:    "FTP",
		22:    "SSH",
		23:    "Telnet",
		25:    "SMTP",
		53:    "DNS",
		80:    "HTTP",
		110:   "POP3",
		143:   "IMAP",
		443:   "HTTPS",
		993:   "IMAPS",
		995:   "POP3S",
		3389:  "RDP",
		5432:  "PostgreSQL",
		3306:  "MySQL",
		6379:  "Redis",
		27017: "MongoDB",
	}

	if service, exists := services[port]; exists {
		return service
	}
	return "Unknown"
}

// 扫描单个端口
func (ps *PortScanner) scanPort(host string, port int) ScanResult {
	address := fmt.Sprintf("%s:%d", host, port)

	conn, err := net.DialTimeout("tcp", address, ps.timeout)
	if err != nil {
		return ScanResult{
			Host:    host,
			Port:    port,
			Open:    false,
			Service: ps.getServiceName(port),
			Error:   err.Error(),
		}
	}

	conn.Close()
	return ScanResult{
		Host:    host,
		Port:    port,
		Open:    true,
		Service: ps.getServiceName(port),
	}
}

// 扫描多个端口（带进度回调）
func (ps *PortScanner) ScanPortsWithProgress(host string, ports []int, sessionID string) []ScanResult {
	total := len(ports)
	startTime := time.Now()

	// 获取会话
	sessionsMu.RLock()
	session, exists := sessions[sessionID]
	sessionsMu.RUnlock()

	// 如果端口数量超过1000，则分批处理
	if total > 1000 {
		fmt.Printf("大扫描检测到 %d 个端口，将分批处理（每批1000个）\n", total)
		return ps.scanPortsInBatches(host, ports, sessionID, session, exists, startTime)
	}

	// 小扫描直接处理
	return ps.scanPortsDirectly(host, ports, sessionID, session, exists, startTime)
}

// 分批扫描端口
func (ps *PortScanner) scanPortsInBatches(host string, ports []int, sessionID string, session *ScanSession, exists bool, startTime time.Time) []ScanResult {
	var allResults []ScanResult
	total := len(ports)
	batchSize := 1000
	totalBatches := (total + batchSize - 1) / batchSize // 向上取整

	fmt.Printf("开始分批扫描，总端口数: %d, 批次大小: %d, 总批次数: %d\n", total, batchSize, totalBatches)

	for batchIndex := 0; batchIndex < totalBatches; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > total {
			end = total
		}

		batchPorts := ports[start:end]
		batchNum := batchIndex + 1

		fmt.Printf("开始扫描第 %d/%d 批，端口范围: %d-%d (共%d个端口)\n",
			batchNum, totalBatches, batchPorts[0], batchPorts[len(batchPorts)-1], len(batchPorts))

		// 发送批次开始进度
		if exists && session != nil {
			progressMsg := ProgressMessage{
				Type:      "progress",
				Completed: start,
				Total:     total,
				Percent:   int(float64(start) / float64(total) * 100),
				OpenPorts: len(allResults),
				Speed:     0,
				BatchInfo: fmt.Sprintf("批次 %d/%d", batchNum, totalBatches),
			}

			select {
			case session.Progress <- progressMsg:
			default:
				// 如果通道满了，跳过这次更新
			}
		}

		// 扫描当前批次
		batchResults := ps.scanPortsDirectly(host, batchPorts, sessionID, session, exists, startTime)
		allResults = append(allResults, batchResults...)

		// 发送批次完成进度
		if exists && session != nil {
			completed := end
			percent := float64(completed) / float64(total) * 100
			elapsed := time.Since(startTime).Seconds()
			speed := 0
			if elapsed > 0 {
				speed = int(float64(completed) / elapsed)
			}

			progressMsg := ProgressMessage{
				Type:      "progress",
				Completed: completed,
				Total:     total,
				Percent:   int(percent),
				OpenPorts: len(allResults),
				Speed:     speed,
				BatchInfo: fmt.Sprintf("批次 %d/%d", batchNum, totalBatches),
			}

			select {
			case session.Progress <- progressMsg:
			default:
				// 如果通道满了，跳过这次更新
			}
		}

		// 批次间短暂延迟，避免资源过度占用
		if batchIndex < totalBatches-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("分批扫描完成，总结果数: %d\n", len(allResults))
	return allResults
}

// 直接扫描端口（小批次或单个批次）
func (ps *PortScanner) scanPortsDirectly(host string, ports []int, sessionID string, session *ScanSession, exists bool, startTime time.Time) []ScanResult {
	var results []ScanResult
	var wg sync.WaitGroup
	var mu sync.Mutex
	var completed int
	var openPorts int

	total := len(ports)

	// 调整并发数
	concurrency := 1000 // 提高并发数到1000
	if len(ports) > 10000 {
		concurrency = 500 // 大扫描降低并发数
	}
	if len(ports) > 50000 {
		concurrency = 200 // 超大扫描进一步降低并发数
	}
	semaphore := make(chan struct{}, concurrency)

	// 添加超时保护
	timeout := time.After(30 * time.Minute) // 30分钟超时
	done := make(chan bool, 1)

	// 启动扫描goroutine
	go func() {
		for _, port := range ports {
			wg.Add(1)
			go func(p int) {
				defer func() {
					wg.Done()
					// 恢复panic，防止程序崩溃
					if r := recover(); r != nil {
						fmt.Printf("扫描端口 %d 时发生错误: %v\n", p, r)
					}
				}()

				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				result := ps.scanPort(host, p)

				mu.Lock()
				results = append(results, result)
				completed++
				if result.Open {
					openPorts++
				}

				// 计算进度和速度
				percent := (completed * 100) / total
				elapsed := time.Since(startTime).Seconds()
				speed := 0
				if elapsed > 0 {
					speed = int(float64(completed) / elapsed)
				}

				// 发送进度更新
				updateFreq := 50 // 批次内更频繁的更新
				if total > 5000 {
					updateFreq = 100
				}

				if exists && session != nil && (completed%updateFreq == 0 || completed == total || (completed < 1000 && completed%10 == 0)) {
					progressMsg := ProgressMessage{
						Type:      "progress",
						Completed: completed,
						Total:     total,
						Percent:   percent,
						OpenPorts: openPorts,
						Speed:     speed,
						BatchInfo: "", // 批次内不显示批次信息
					}

					select {
					case session.Progress <- progressMsg:
					default:
						// 如果通道满了，跳过这次更新
					}
				}
				mu.Unlock()
			}(port)
		}
		wg.Wait()
		done <- true
	}()

	// 等待扫描完成或超时
	select {
	case <-done:
		fmt.Printf("批次扫描完成，会话: %s, 结果数量: %d\n", sessionID, len(results))
	case <-timeout:
		fmt.Printf("批次扫描超时，会话: %s, 已完成: %d/%d\n", sessionID, completed, total)
		// 等待所有goroutine完成
		wg.Wait()
		fmt.Printf("超时后等待完成，会话: %s, 最终结果数量: %d\n", sessionID, len(results))
	}

	return results
}

// 扫描多个端口（保持向后兼容）
func (ps *PortScanner) ScanPorts(host string, ports []int) []ScanResult {
	return ps.ScanPortsWithProgress(host, ports, "")
}

// 解析端口范围
func parsePorts(portStr string) ([]int, error) {
	if portStr == "all" {
		// 扫描常用端口
		return []int{21, 22, 23, 25, 53, 80, 110, 143, 443, 993, 995, 3389, 5432, 3306, 6379, 27017}, nil
	}

	var ports []int
	parts := strings.Split(portStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			// 端口范围
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的端口范围: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("无效的起始端口: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("无效的结束端口: %s", rangeParts[1])
			}

			if start > end {
				return nil, fmt.Errorf("起始端口不能大于结束端口: %s", part)
			}

			for i := start; i <= end; i++ {
				ports = append(ports, i)
			}
		} else {
			// 单个端口
			port, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("无效的端口号: %s", part)
			}
			ports = append(ports, port)
		}
	}

	return ports, nil
}

// 解析域名或IP
func resolveHost(host string) (string, error) {
	// 如果是IP地址，直接返回
	if net.ParseIP(host) != nil {
		return host, nil
	}

	// 解析域名
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("无法解析域名: %s", host)
	}

	return ips[0].String(), nil
}

// 处理扫描请求
func handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析表单数据
	target := r.FormValue("target")
	ports := r.FormValue("ports")

	if target == "" {
		http.Error(w, "目标地址不能为空", http.StatusBadRequest)
		return
	}

	if ports == "" {
		ports = "all"
	}

	// 解析目标地址
	host, err := resolveHost(target)
	if err != nil {
		http.Error(w, fmt.Sprintf("解析目标地址失败: %v", err), http.StatusBadRequest)
		return
	}

	// 解析端口
	portList, err := parsePorts(ports)
	if err != nil {
		http.Error(w, fmt.Sprintf("解析端口失败: %v", err), http.StatusBadRequest)
		return
	}

	// 创建扫描器，根据端口数量调整超时时间
	timeout := 2 * time.Second
	if len(portList) > 10000 {
		timeout = 3 * time.Second // 大扫描增加超时时间
	}
	if len(portList) > 50000 {
		timeout = 5 * time.Second // 超大扫描进一步增加超时时间
	}
	scanner := NewPortScanner(timeout)

	// 创建会话
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	session := &ScanSession{
		ID:        sessionID,
		Progress:  make(chan ProgressMessage, 100),
		StartTime: time.Now(),
	}

	sessionsMu.Lock()
	sessions[sessionID] = session
	sessionsMu.Unlock()

	fmt.Printf("创建新会话: %s, 端口数量: %d\n", sessionID, len(portList))

	// 异步执行扫描
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("扫描过程中发生错误，会话: %s, 错误: %v\n", sessionID, r)
				// 即使出错也要保存部分结果
				session.mu.Lock()
				if len(session.Results) == 0 {
					session.Results = []ScanResult{}
				}
				session.mu.Unlock()
			}
			// 强制垃圾回收，释放内存
			runtime.GC()
		}()

		results := scanner.ScanPortsWithProgress(host, portList, sessionID)

		fmt.Printf("扫描完成，会话: %s, 结果数量: %d\n", sessionID, len(results))

		// 保存结果到会话
		session.mu.Lock()
		session.Results = results
		session.mu.Unlock()

		fmt.Printf("结果已保存到会话: %s, 结果数量: %d\n", sessionID, len(results))

		// 等待一小段时间确保结果已保存
		time.Sleep(100 * time.Millisecond)

		// 发送完成消息
		completeMsg := ProgressMessage{
			Type:      "complete",
			Completed: len(results),
			Total:     len(portList),
			Percent:   100,
		}

		select {
		case session.Progress <- completeMsg:
			fmt.Printf("完成消息已发送，会话: %s\n", sessionID)
		default:
			fmt.Printf("完成消息发送失败，会话: %s\n", sessionID)
		}

		// 延迟删除会话，给客户端时间获取结果
		go func() {
			// 根据扫描的端口数量调整等待时间
			waitTime := 30 * time.Second
			if len(portList) > 1000 {
				waitTime = 60 * time.Second // 大扫描等待更长时间
			}
			if len(portList) > 10000 {
				waitTime = 120 * time.Second // 超大扫描等待2分钟
			}

			time.Sleep(waitTime)
			sessionsMu.Lock()
			delete(sessions, sessionID)
			sessionsMu.Unlock()
			fmt.Printf("会话已删除: %s (等待了 %v)\n", sessionID, waitTime)
		}()
	}()

	// 返回会话ID
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"sessionId":"%s"}`, sessionID)
}

// 处理进度查询
func handleProgress(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "缺少sessionId参数", http.StatusBadRequest)
		return
	}

	sessionsMu.RLock()
	session, exists := sessions[sessionID]
	sessionsMu.RUnlock()

	if !exists {
		http.Error(w, "会话不存在", http.StatusNotFound)
		return
	}

	// 设置SSE头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 发送进度更新
	for {
		select {
		case msg := <-session.Progress:
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()

			if msg.Type == "complete" {
				fmt.Printf("进度推送完成，会话: %s\n", sessionID)
				return
			}
		case <-time.After(300 * time.Second): // 增加到5分钟超时
			// 超时
			fmt.Printf("进度推送超时，会话: %s\n", sessionID)
			return
		}
	}
}

// 处理结果查询
func handleResults(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	fmt.Printf("收到结果查询请求，sessionID: %s\n", sessionID)

	if sessionID == "" {
		http.Error(w, "缺少sessionId参数", http.StatusBadRequest)
		return
	}

	sessionsMu.RLock()
	session, exists := sessions[sessionID]
	sessionCount := len(sessions)
	sessionsMu.RUnlock()

	fmt.Printf("当前会话数: %d, 查找的会话存在: %t\n", sessionCount, exists)

	if !exists {
		http.Error(w, "会话不存在或已过期", http.StatusNotFound)
		return
	}

	session.mu.RLock()
	results := session.Results
	resultCount := len(results)
	session.mu.RUnlock()

	fmt.Printf("会话结果数量: %d\n", resultCount)

	// 检查结果是否为空
	if len(results) == 0 {
		fmt.Printf("警告: 会话 %s 的结果为空\n", sessionID)
		// 不返回错误，返回空结果
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"results":[]}`)
		return
	}

	// 返回JSON结果
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"results":[`)
	for i, result := range results {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, `{"host":"%s","port":%d,"open":%t,"service":"%s"`,
			result.Host, result.Port, result.Open, result.Service)
		if result.Error != "" {
			fmt.Fprintf(w, `,"error":"%s"`, result.Error)
		}
		fmt.Fprintf(w, "}")
	}
	fmt.Fprintf(w, "]}")
}

// 处理主页
func handleHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

// 处理静态文件
func handleStatic(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/"+r.URL.Path[8:]) // 移除 "/static/" 前缀
}

func main() {
	// 设置路由
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/static/", handleStatic)
	http.HandleFunc("/api/scan", handleScan)
	http.HandleFunc("/api/progress", handleProgress)
	http.HandleFunc("/api/results", handleResults)

	fmt.Println("端口扫描器启动在 http://localhost:8080")
	fmt.Println("按 Ctrl+C 停止服务")

	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
	}
}
