// Package archeryclient 是对 archery-cli（github.com/rjchien728/archery-cli）HTTP 客户端的
// 精简移植，供 baboflow 的 archery 规则链节点查询 Archery（hhyo/Archery）平台。
//
// 与原 CLI 的差异：
//   - 会话 cookie 保存在进程内存（mutex 保护的 cookiejar），不写磁盘缓存
//     （原 CLI 缓存到 ~/.cache/archery/cookies.json 以便多次进程调用复用会话）。
//   - 每个连接一个 Client 实例；规则链可能并发跑同一连接，故加锁保证并发安全。
//   - 不实现 psql 风格的 table/csv 终端格式化；节点层直接消费结构化结果。
package archeryclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

const defaultTimeout = 30 * time.Second

// ErrAuthFailed 表示 Archery 登录被拒绝（用户名/密码错误或非 Archery 实例）。
var ErrAuthFailed = errors.New("archery 认证失败，请检查用户名/密码")

// Config 一个 Archery 连接的配置（与 archery_connection 表对应，密码为解密后的明文）。
type Config struct {
	Endpoint string // Archery 基础地址，如 https://archery.example.com
	Instance string // Archery 中配置的实例名
	Username string
	Password string
	Insecure bool   // 跳过 TLS 校验（不安全）
	CACert   string // 额外信任的 CA（PEM 文本，可空）
	Timeout  time.Duration
}

// Client 是面向单个 Archery 连接的 HTTP 客户端，并发安全。
type Client struct {
	cfg      Config
	httpc    *http.Client
	jar      *cookiejar.Jar
	endpoint *url.URL
	// mu 串行化请求：登录/重登会改写共享 cookiejar，且 Archery 会话为服务端状态，
	// 并发请求若各自触发重登会互相覆盖 cookie。规则链并发度不高，串行足够且最稳。
	mu sync.Mutex
}

// New 创建客户端。cfg.Endpoint 必须非空。
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("archery endpoint 不能为空")
	}
	endpoint, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("非法 archery endpoint: %w", err)
	}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg:      cfg,
		jar:      jar,
		endpoint: endpoint,
		httpc: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:       tlsCfg,
				Proxy:                 http.ProxyFromEnvironment,
				ResponseHeaderTimeout: timeout,
			},
			Jar:     jar,
			Timeout: timeout,
			// 不跟随重定向：登录态失效时 Archery 会 302 到 /login，需要识别后重登。
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	return c, nil
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{}
	if cfg.Insecure {
		tlsCfg.InsecureSkipVerify = true
		return tlsCfg, nil
	}
	if strings.TrimSpace(cfg.CACert) != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.CACert)) {
			return nil, errors.New("archery CA 证书不是有效的 PEM")
		}
		tlsCfg.RootCAs = pool
	}
	return tlsCfg, nil
}

// Instance 返回配置的实例名（诊断用）。
func (c *Client) Instance() string { return c.cfg.Instance }

// Endpoint 返回配置的基础地址（诊断用）。
func (c *Client) Endpoint() string { return c.endpoint.String() }

// reqSpec 描述一次 HTTP 调用。
type reqSpec struct {
	method    string
	path      string
	query     url.Values
	form      url.Values
	autoLogin bool
}

// do 执行请求；autoLogin 为 true 时，遇 403 或 302→/login 自动重登一次并重试。
// 调用方需持有 c.mu（见 Query/Resource/login）。
func (c *Client) do(rs reqSpec) (status int, body []byte, err error) {
	const maxAttempts = 2
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, rerr := c.buildRequest(rs)
		if rerr != nil {
			return 0, nil, rerr
		}
		resp, herr := c.httpc.Do(req)
		if herr != nil {
			return 0, nil, fmt.Errorf("archery 网络错误: %w", herr)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()

		if rs.autoLogin && attempt == 0 && needsLogin(resp) {
			if err := c.loginLocked(); err != nil {
				return 0, nil, err
			}
			continue
		}
		return resp.StatusCode, body, nil
	}
	return 0, nil, errors.New("archery 请求超过重试次数")
}

func (c *Client) buildRequest(rs reqSpec) (*http.Request, error) {
	u := *c.endpoint
	u.Path = rs.path
	if rs.query != nil {
		u.RawQuery = rs.query.Encode()
	}
	var body io.Reader
	if rs.form != nil {
		body = strings.NewReader(rs.form.Encode())
	}
	req, err := http.NewRequest(rs.method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if rs.form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", c.endpoint.String()+"/sqlquery/")
	req.Header.Set("Origin", c.endpoint.String())
	if csrf := c.csrfFromJar(); csrf != "" {
		req.Header.Set("X-CSRFToken", csrf)
	}
	return req, nil
}

func needsLogin(resp *http.Response) bool {
	if resp.StatusCode == http.StatusForbidden {
		return true
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		loc := resp.Header.Get("Location")
		return strings.Contains(loc, "/login") || strings.Contains(loc, "/accounts/login")
	}
	return false
}

func (c *Client) csrfFromJar() string {
	for _, ck := range c.jar.Cookies(c.endpoint) {
		if ck.Name == "csrftoken" {
			return ck.Value
		}
	}
	return ""
}

func (c *Client) hasSessionCookie() bool {
	for _, ck := range c.jar.Cookies(c.endpoint) {
		if ck.Name == "sessionid" {
			return true
		}
	}
	return false
}

// Login 建立 Archery 会话（Django 登录流程）：
//  1. GET /login/ —— 种下 csrftoken cookie；
//  2. POST /authenticate/（X-CSRFToken + username/password）—— 校验 {status:0} 且有 sessionid。
func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked()
}

func (c *Client) loginLocked() error {
	if _, _, err := c.do(reqSpec{method: "GET", path: "/login/"}); err != nil {
		return fmt.Errorf("login: GET /login/: %w", err)
	}
	if c.csrfFromJar() == "" {
		return errors.New("login: /login/ 未设置 csrftoken cookie，目标可能不是 hhyo/Archery 实例")
	}
	form := url.Values{
		"username": {c.cfg.Username},
		"password": {c.cfg.Password},
	}
	status, body, err := c.do(reqSpec{method: "POST", path: "/authenticate/", form: form})
	if err != nil {
		return fmt.Errorf("login: POST /authenticate/: %w", err)
	}
	if status >= 500 {
		return fmt.Errorf("login: archery 服务端错误 HTTP %d", status)
	}
	if status >= 400 {
		return fmt.Errorf("login: HTTP %d: %s", status, snippet(body))
	}
	var env struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("login: 解析 /authenticate/ 响应: %w (body: %s)", err, snippet(body))
	}
	if env.Status != 0 {
		// 不回显密码；原样返回服务端信息（通常为“用户名或密码错误”）。
		return ErrAuthFailed
	}
	if !c.hasSessionCookie() {
		return errors.New("login: 服务端返回 ok 但未设置 sessionid cookie")
	}
	return nil
}

// ServerError 表示 Archery 返回 status != 0。
type ServerError struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
}

func (e *ServerError) Error() string { return e.Msg }

// QueryResult 镜像 /query/ 响应的 data 块。
type QueryResult struct {
	FullSQL      string   `json:"full_sql"`
	IsExecute    bool     `json:"is_execute"`
	IsMasked     bool     `json:"is_masked"`
	QueryTime    float64  `json:"query_time"`
	MaskRuleHit  bool     `json:"mask_rule_hit"`
	Warning      *string  `json:"warning"`
	Error        *string  `json:"error"`
	IsCritical   bool     `json:"is_critical"`
	Rows         [][]any  `json:"rows"`
	ColumnList   []string `json:"column_list"`
	ColumnType   []string `json:"column_type"`
	AffectedRows int      `json:"affected_rows"`
}

type queryEnvelope struct {
	Status int          `json:"status"`
	Msg    string       `json:"msg"`
	Data   *QueryResult `json:"data"`
}

type listEnvelope struct {
	Status int      `json:"status"`
	Msg    string   `json:"msg"`
	Data   []string `json:"data"`
}

// Query 对 (db, schema) 执行 SELECT。limit 对应 Archery 的 limit_num（服务端内部追加 LIMIT）。
func (c *Client) Query(db, schema, sql string, limit int) (*QueryResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	form := url.Values{
		"instance_name": {c.cfg.Instance},
		"db_name":       {db},
		"schema_name":   {schema},
		"tb_name":       {""},
		"sql_content":   {sql},
		"limit_num":     {strconv.Itoa(limit)},
	}
	status, body, err := c.do(reqSpec{method: "POST", path: "/query/", form: form, autoLogin: true})
	if err != nil {
		return nil, err
	}
	if status >= 500 {
		return nil, fmt.Errorf("archery 服务端错误 HTTP %d", status)
	}
	if status >= 400 {
		return nil, fmt.Errorf("archery HTTP %d: %s", status, snippet(body))
	}
	var env queryEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析 query 响应: %w (body: %s)", err, snippet(body))
	}
	if env.Status != 0 {
		return nil, &ServerError{Status: env.Status, Msg: env.Msg}
	}
	if env.Data == nil {
		return nil, errors.New("archery 返回 ok 但 data 为空")
	}
	return env.Data, nil
}

// ResourceType 是经 /instance/instance_resource/ 列出的元数据类型。
type ResourceType string

const (
	ResDatabase ResourceType = "database"
	ResSchema   ResourceType = "schema"
	ResTable    ResourceType = "table"
	ResColumn   ResourceType = "column"
)

// Resource 列出 库/schema/表/字段。db/schema/table 仅在对该资源类型有意义时传入。
func (c *Client) Resource(rt ResourceType, db, schema, table string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := url.Values{
		"instance_name": {c.cfg.Instance},
		"resource_type": {string(rt)},
	}
	if db != "" {
		q.Set("db_name", db)
	}
	if schema != "" {
		q.Set("schema_name", schema)
	}
	if table != "" {
		q.Set("tb_name", table)
	}
	status, body, err := c.do(reqSpec{method: "GET", path: "/instance/instance_resource/", query: q, autoLogin: true})
	if err != nil {
		return nil, err
	}
	if status >= 500 {
		return nil, fmt.Errorf("archery 服务端错误 HTTP %d", status)
	}
	if status >= 400 {
		return nil, fmt.Errorf("archery HTTP %d: %s", status, snippet(body))
	}
	var env listEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析 resource 响应: %w (body: %s)", err, snippet(body))
	}
	if env.Status != 0 {
		return nil, &ServerError{Status: env.Status, Msg: env.Msg}
	}
	return env.Data, nil
}

func snippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "...(truncated)"
	}
	return string(b)
}
