package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// feishuBaseURL 飞书开放平台基础地址。做成包级变量便于单测用 httptest 替换。
var feishuBaseURL = "https://open.feishu.cn"

// feishuAuthorizePath 飞书网页授权页路径（旧版 authorize 端点，相对 feishuBaseURL）。
const feishuAuthorizePath = "/open-apis/authen/v1/authorize"

// 飞书登录相关错误。
var (
	ErrFeishuNotConfigured = errors.New("飞书登录未配置（缺少 FEISHU_APP_ID/FEISHU_APP_SECRET/FEISHU_REDIRECT_URI）")
	ErrFeishuAuth          = errors.New("飞书授权失败")
)

// feishuClient 直连飞书开放平台的极简客户端（仅登录所需的 3 个接口）。
type feishuClient struct {
	appID     string
	appSecret string
	httpc     *http.Client
}

func newFeishuClient(appID, appSecret string) *feishuClient {
	return &feishuClient{
		appID:     appID,
		appSecret: appSecret,
		httpc:     &http.Client{Timeout: 15 * time.Second},
	}
}

// feishuErr 飞书返回 code!=0 时的错误（透出 msg，不含敏感信息）。
type feishuErr struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *feishuErr) Error() string { return fmt.Sprintf("飞书接口错误(code=%d): %s", e.Code, e.Msg) }

// post 以 JSON 调用飞书接口并解析公共 code/msg 包裹；data 反序列化进 out。
func (c *feishuClient) post(ctx context.Context, path string, body any, bearer string, out any) error {
	return c.do(ctx, http.MethodPost, path, body, bearer, out)
}

func (c *feishuClient) get(ctx context.Context, path, bearer string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, bearer, out)
}

func (c *feishuClient) do(ctx context.Context, method, path string, body any, bearer string, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, feishuBaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("请求飞书失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("飞书 HTTP %d: %s", resp.StatusCode, feishuSnippet(raw))
	}
	// 公共包裹：{code, msg, data/app_access_token/...}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("解析飞书响应失败: %w (body: %s)", err, feishuSnippet(raw))
	}
	if envelope.Code != 0 {
		return &feishuErr{Code: envelope.Code, Msg: envelope.Msg}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("解析飞书响应数据失败: %w", err)
		}
	}
	return nil
}

// appAccessToken 用 app_id/app_secret 换 app_access_token（自建应用内部获取）。
func (c *feishuClient) appAccessToken(ctx context.Context) (string, error) {
	var out struct {
		AppAccessToken string `json:"app_access_token"`
		Expire         int    `json:"expire"`
	}
	err := c.post(ctx, "/open-apis/auth/v3/app_access_token/internal", map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}, "", &out)
	if err != nil {
		return "", err
	}
	if out.AppAccessToken == "" {
		return "", errors.New("飞书返回空 app_access_token")
	}
	return out.AppAccessToken, nil
}

// userTokenResult code 换 user_access_token 的结果（OIDC 端点直接在 data 里带用户标识）。
type userTokenResult struct {
	UserAccessToken string `json:"access_token"`
	OpenID          string `json:"open_id"`
	UnionID         string `json:"union_id"`
	Name            string `json:"name"`
	EnName          string `json:"en_name"`
	Email           string `json:"email"`
	AvatarURL       string `json:"avatar_url"`
}

// exchangeCode 用授权 code 换 user_access_token 与用户标识（OIDC）。
func (c *feishuClient) exchangeCode(ctx context.Context, appToken, code string) (*userTokenResult, error) {
	var out struct {
		Data userTokenResult `json:"data"`
	}
	err := c.post(ctx, "/open-apis/authen/v1/oidc/access_token", map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}, appToken, &out)
	if err != nil {
		return nil, err
	}
	if out.Data.UserAccessToken == "" {
		return nil, errors.New("飞书返回空 user_access_token")
	}
	return &out.Data, nil
}

// userInfoResult user_info 返回的用户资料。
type userInfoResult struct {
	OpenID    string `json:"open_id"`
	UnionID   string `json:"union_id"`
	Name      string `json:"name"`
	EnName    string `json:"en_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// userInfo 用 user_access_token 拉取用户资料（拿权威 openid/昵称/头像/邮箱）。
func (c *feishuClient) userInfo(ctx context.Context, userToken string) (*userInfoResult, error) {
	var out struct {
		Data userInfoResult `json:"data"`
	}
	if err := c.get(ctx, "/open-apis/authen/v1/user_info", userToken, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// FeishuUsecase 处理飞书 OAuth 登录：构造授权 URL、用 code 换用户并按 openid 找/建账号发证。
type FeishuUsecase struct {
	repo AuthRepo
	cfg  *conf.Config
}

func NewFeishuUsecase(repo AuthRepo, c *conf.Config) *FeishuUsecase {
	return &FeishuUsecase{repo: repo, cfg: c}
}

// Configured 报告飞书登录是否已配置完整（缺一项则不可用）。
func (uc *FeishuUsecase) Configured() bool {
	return strings.TrimSpace(uc.cfg.FeishuAppID) != "" &&
		strings.TrimSpace(uc.cfg.FeishuAppSecret) != "" &&
		strings.TrimSpace(uc.cfg.FeishuRedirectURI) != ""
}

// BuildAuthURL 拼飞书授权页 URL（redirect_uri urlencode，附 CSRF state）。
func (uc *FeishuUsecase) BuildAuthURL(state string) string {
	q := url.Values{}
	q.Set("app_id", uc.cfg.FeishuAppID)
	q.Set("redirect_uri", uc.cfg.FeishuRedirectURI)
	q.Set("state", state)
	return feishuBaseURL + feishuAuthorizePath + "?" + q.Encode()
}

// LoginByCode 用授权 code 完成飞书登录：换 user token → 拉 user_info 拿 openid →
// 按 openid 找/建用户 → 复用共享 issueSession 发证（与密码登录同一套 session）。
func (uc *FeishuUsecase) LoginByCode(ctx context.Context, code, ip, ua string) (*LoginResult, error) {
	if !uc.Configured() {
		return nil, ErrFeishuNotConfigured
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrFeishuAuth
	}
	cli := newFeishuClient(uc.cfg.FeishuAppID, uc.cfg.FeishuAppSecret)

	appToken, err := cli.appAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	tok, err := cli.exchangeCode(ctx, appToken, code)
	if err != nil {
		return nil, err
	}
	// 以 user_info 为权威资料（exchangeCode 的 OIDC data 也带 openid，这里统一取 user_info）。
	profile, err := cli.userInfo(ctx, tok.UserAccessToken)
	if err != nil {
		return nil, err
	}
	openid := strings.TrimSpace(profile.OpenID)
	if openid == "" {
		openid = strings.TrimSpace(tok.OpenID) // user_info 异常缺字段时回退 OIDC data
	}
	if openid == "" {
		return nil, errors.New("飞书未返回 open_id")
	}

	displayName := firstNonEmpty(profile.Name, profile.EnName, "飞书用户")
	user, err := uc.repo.FindUserByFeishuOpenID(ctx, openid)
	switch {
	case err == nil:
		// 已存在：回写最新资料（昵称/头像/邮箱/union_id）。
		_ = uc.repo.UpdateFeishuProfile(ctx, user.ID, displayName, profile.AvatarURL, profile.Email, profile.UnionID)
		user.DisplayName = displayName
		user.Avatar = profile.AvatarURL
		user.Email = profile.Email
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 首次飞书登录：自动建号。username=feishu_<openid>，无可用密码。
		user = &po.AdminUser{
			Username:      "feishu_" + openid,
			PasswordHash:  "", // 空串占位，无法通过密码登录
			DisplayName:   displayName,
			MustChangePwd: false,
			FeishuOpenID:  strPtr(openid),
			FeishuUnionID: profile.UnionID,
			Avatar:        profile.AvatarURL,
			Email:         profile.Email,
		}
		if cerr := uc.repo.CreateUser(ctx, user); cerr != nil {
			return nil, cerr
		}
	default:
		return nil, err
	}

	sid, err := issueSession(ctx, uc.repo, user, ip, ua)
	if err != nil {
		return nil, err
	}
	return &LoginResult{
		UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		MustChangePwd: user.MustChangePwd, SessionID: sid,
	}, nil
}

func strPtr(s string) *string { return &s }

// feishuSnippet 截断响应体用于错误信息（避免日志过长）。
func feishuSnippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "...(truncated)"
	}
	return string(b)
}
