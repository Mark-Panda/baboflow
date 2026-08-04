package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
	"baboflow/internal/server/httputil"
)

// WsFrame 服务端 → 客户端统一帧。
type WsFrame struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
}

// wsInbound 客户端 → 服务端消息。
type wsInbound struct {
	Action  string `json:"action"` // subscribe/unsubscribe/input
	Channel string `json:"channel"`
	// agent-chat
	SessionID string  `json:"sessionId"`
	AgentKey  string  `json:"agentKey"`
	Content   string  `json:"content"`
	AssetIDs  []int64 `json:"assetIds"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// 仅允许同主机 Origin；无 Origin 时兼容非浏览器 WebSocket 客户端。
	CheckOrigin: sameHostOrigin,
}

func sameHostOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	requestHost := r.Host
	if host, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestHost = host
	}
	return strings.EqualFold(u.Hostname(), requestHost)
}

// WsHub 管理全部 WS 连接，按 sessionID 路由 agent-chat 事件。
type WsHub struct {
	agentUC *biz.AgentUsecase
	auth    *biz.AuthUsecase

	mu        sync.RWMutex
	bySess    map[string]map[*wsConn]bool // sessionID → 连接集
	conns     int
	chatSlots chan struct{}
}

type wsConn struct {
	conn   *websocket.Conn
	userID int64
	ctx    context.Context
	mu     sync.Mutex // 写串行化
}

func NewWsHub(agentUC *biz.AgentUsecase, auth *biz.AuthUsecase) *WsHub {
	return &WsHub{
		agentUC:   agentUC,
		auth:      auth,
		bySess:    map[string]map[*wsConn]bool{},
		chatSlots: make(chan struct{}, 32),
	}
}

// Handle 处理 GET /ws 升级。先经 cookie 鉴权。
func (h *WsHub) Handle(c *gin.Context) {
	sid, err := c.Cookie(biz.SessionCookieName)
	if err != nil || sid == "" {
		httputil.Unauthorized(c, "未登录")
		return
	}
	user, err := h.auth.Validate(c.Request.Context(), sid)
	if err != nil {
		httputil.Unauthorized(c, "会话已过期")
		return
	}
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	if h.conns >= 128 {
		h.mu.Unlock()
		_ = conn.Close()
		return
	}
	h.conns++
	h.mu.Unlock()
	wc := &wsConn{conn: conn, userID: user.ID, ctx: c.Request.Context()}
	biz.WsConnections.Inc()
	defer biz.WsConnections.Dec()
	defer func() {
		h.mu.Lock()
		h.conns--
		h.mu.Unlock()
	}()
	defer h.cleanup(wc)

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	})
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var in wsInbound
		if err := json.Unmarshal(raw, &in); err != nil {
			continue
		}
		h.dispatch(wc, &in)
	}
}

func (h *WsHub) dispatch(wc *wsConn, in *wsInbound) {
	switch in.Channel {
	case "agent-chat":
		switch in.Action {
		case "subscribe":
			h.subscribe(in.SessionID, wc)
		case "unsubscribe":
			h.unsubscribe(in.SessionID, wc)
		case "input":
			h.handleChatInput(wc, in)
		}
	}
}

func (h *WsHub) subscribe(sessionID string, wc *wsConn) bool {
	if sessionID == "" {
		return false
	}
	if err := h.agentUC.ValidateSessionAccess(wc.ctx, sessionID, wc.userID); err != nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bySess[sessionID] == nil {
		h.bySess[sessionID] = map[*wsConn]bool{}
	}
	h.bySess[sessionID][wc] = true
	return true
}

func (h *WsHub) unsubscribe(sessionID string, wc *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.bySess[sessionID]; ok {
		delete(set, wc)
		if len(set) == 0 {
			delete(h.bySess, sessionID)
		}
	}
}

func (h *WsHub) cleanup(wc *wsConn) {
	h.mu.Lock()
	for _, set := range h.bySess {
		delete(set, wc)
	}
	// 清掉空 session 集
	for sid, set := range h.bySess {
		if len(set) == 0 {
			delete(h.bySess, sid)
		}
	}
	h.mu.Unlock()
	_ = wc.conn.Close()
}

// broadcast 把一帧发给订阅了 sessionID 的全部连接。
func (h *WsHub) broadcast(sessionID string, frame *WsFrame) {
	h.mu.RLock()
	set := h.bySess[sessionID]
	conns := make([]*wsConn, 0, len(set))
	for wc := range set {
		conns = append(conns, wc)
	}
	h.mu.RUnlock()
	for _, wc := range conns {
		wc.mu.Lock()
		_ = wc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = wc.conn.WriteJSON(frame)
		wc.mu.Unlock()
	}
}

// handleChatInput 运行一轮对话并流式广播。
func (h *WsHub) handleChatInput(wc *wsConn, in *wsInbound) {
	if in.SessionID == "" {
		return
	}
	// 校验订阅，未订阅则先订阅（容错）
	if !h.subscribe(in.SessionID, wc) {
		return
	}

	atts := make([]biz.ChatAttachment, 0, len(in.AssetIDs))
	for _, id := range in.AssetIDs {
		atts = append(atts, biz.ChatAttachment{AssetID: id})
	}

	onEvent := func(ev *agentkit.StreamEvent) {
		h.broadcast(in.SessionID, toWsFrame(in.SessionID, ev))
	}

	// 异步执行，避免阻塞该连接的读循环（支持并发订阅其它会话）
	select {
	case h.chatSlots <- struct{}{}:
	default:
		h.broadcast(in.SessionID, &WsFrame{
			Channel: "agent-chat",
			Type:    "error",
			Data:    map[string]any{"sessionId": in.SessionID, "err": "当前并发对话数已达上限"},
		})
		return
	}
	go func() {
		defer func() { <-h.chatSlots }()
		_, err := h.agentUC.Chat(wc.ctx, in.SessionID, in.Content, atts, wc.userID, onEvent)
		if err != nil {
			h.broadcast(in.SessionID, &WsFrame{
				Channel: "agent-chat",
				Type:    "error",
				Data:    map[string]any{"sessionId": in.SessionID, "err": err.Error()},
			})
		}
	}()
}

// toWsFrame 把 agentkit 流事件映射为前端契约帧。
func toWsFrame(sessionID string, ev *agentkit.StreamEvent) *WsFrame {
	switch ev.Type {
	case "text":
		return &WsFrame{Channel: "agent-chat", Type: "delta", Data: map[string]any{
			"sessionId": sessionID, "delta": ev.Delta, "agent": ev.Agent, "done": false,
		}}
	case "tool_call":
		return &WsFrame{Channel: "agent-chat", Type: "tool_call", Data: map[string]any{
			"sessionId": sessionID, "tool": ev.ToolName, "input": ev.ToolArgs, "status": "running",
		}}
	case "tool_result":
		return &WsFrame{Channel: "agent-chat", Type: "tool_call", Data: map[string]any{
			"sessionId": sessionID, "tool": ev.ToolName, "output": ev.ToolOut, "status": "done",
		}}
	case "done":
		return &WsFrame{Channel: "agent-chat", Type: "delta", Data: map[string]any{
			"sessionId": sessionID, "delta": "", "done": true,
		}}
	case "error":
		return &WsFrame{Channel: "agent-chat", Type: "error", Data: map[string]any{
			"sessionId": sessionID, "err": ev.Err,
		}}
	default:
		return &WsFrame{Channel: "agent-chat", Type: ev.Type, Data: map[string]any{"sessionId": sessionID}}
	}
}
