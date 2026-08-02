package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"baboflow/internal/data/po"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrBadCredential = errors.New("用户名或密码错误")
	ErrSessionExpired = errors.New("会话已过期")
)

const (
	SessionCookieName = "baboflow_sid"
	sessionTTL        = 7 * 24 * time.Hour // 7 天滑动过期
)

type AuthRepo interface {
	FindUserByUsername(ctx context.Context, username string) (*po.AdminUser, error)
	FindUserByID(ctx context.Context, id int64) (*po.AdminUser, error)
	UpdateUserPassword(ctx context.Context, id int64, hash string, mustChange bool) error
	TouchLastLogin(ctx context.Context, id int64) error

	CreateSession(ctx context.Context, s *po.Session) error
	FindSession(ctx context.Context, id string) (*po.Session, error)
	TouchSession(ctx context.Context, id string, expiresAt time.Time) error
	DeleteSession(ctx context.Context, id string) error
}

type AuthUsecase struct {
	repo AuthRepo
}

func NewAuthUsecase(repo AuthRepo) *AuthUsecase { return &AuthUsecase{repo: repo} }

type LoginResult struct {
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	MustChangePwd bool `json:"mustChangePwd"`
	SessionID   string `json:"-"`
}

func (uc *AuthUsecase) Login(ctx context.Context, username, password, ip, ua string) (*LoginResult, error) {
	user, err := uc.repo.FindUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBadCredential
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrBadCredential
	}
	sid := newSessionID()
	sess := &po.Session{
		ID:        sid,
		UserID:    user.ID,
		IP:        ip,
		UserAgent: ua,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	if err := uc.repo.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	_ = uc.repo.TouchLastLogin(ctx, user.ID)
	return &LoginResult{
		UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		MustChangePwd: user.MustChangePwd, SessionID: sid,
	}, nil
}

func (uc *AuthUsecase) Logout(ctx context.Context, sid string) error {
	if sid == "" {
		return nil
	}
	return uc.repo.DeleteSession(ctx, sid)
}

// Validate 校验会话并滑动续期，返回当前用户。
func (uc *AuthUsecase) Validate(ctx context.Context, sid string) (*po.AdminUser, error) {
	if sid == "" {
		return nil, ErrSessionExpired
	}
	sess, err := uc.repo.FindSession(ctx, sid)
	if err != nil {
		return nil, ErrSessionExpired
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = uc.repo.DeleteSession(ctx, sid)
		return nil, ErrSessionExpired
	}
	// 滑动续期
	_ = uc.repo.TouchSession(ctx, sid, time.Now().Add(sessionTTL))
	user, err := uc.repo.FindUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, ErrSessionExpired
	}
	return user, nil
}

func (uc *AuthUsecase) ChangePassword(ctx context.Context, userID int64, oldPwd, newPwd string) error {
	user, err := uc.repo.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPwd)) != nil {
		return errors.New("原密码错误")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return uc.repo.UpdateUserPassword(ctx, userID, string(hash), false)
}

func newSessionID() string {
	b := make([]byte, 16) // 128-bit
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
