package service

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

type sidecarAgentRepo struct {
	stubAgentRepo
	userID int64
}

func (r sidecarAgentRepo) GetSession(context.Context, string) (*po.AgentSession, error) {
	return &po.AgentSession{ID: "session-1", UserID: &r.userID}, nil
}

func (sidecarAgentRepo) GetAsset(context.Context, int64) (*po.Asset, error) {
	return &po.Asset{
		ID:        9,
		Name:      "note.txt",
		Mime:      "text/plain",
		Path:      "assets/note.txt",
		SessionID: "session-1",
	}, nil
}

func (sidecarAgentRepo) CreateAsset(_ context.Context, asset *po.Asset) error {
	asset.ID = 9007199254740993
	return nil
}

type sidecarAssetStore struct {
	data []byte
}

func (sidecarAssetStore) Save(string, string, []byte) (string, error) { return "", nil }
func (s sidecarAssetStore) Read(string) ([]byte, error)               { return s.data, nil }
func (sidecarAssetStore) DeleteBySession(string) error                { return nil }

func TestAgentAssetSidecarUploadReturnsProtoInt64Strings(t *testing.T) {
	repo := sidecarAgentRepo{userID: 7}
	uc := biz.NewAgentUsecase(repo, nil, nil, sidecarAssetStore{}, nil)
	handler := NewAgentHandler(uc)
	router := gin.New()
	router.POST("/api/v1/agent-assets", GinAuthMiddleware(testAuthUsecase()), handler.UploadAsset)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writer.WriteField("sessionId", "session-1"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-assets", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "valid-sid"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		ID   string `json:"id"`
		Size string `json:"size"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("asset response must encode id and size as strings: %v; body=%s", err, response.Body.String())
	}
	if body.ID != "9007199254740993" || body.Size != "5" {
		t.Fatalf("unexpected asset response: %#v", body)
	}
}

func TestAgentAssetSidecarReturnsRawContentAfterAuthentication(t *testing.T) {
	repo := sidecarAgentRepo{userID: 7}
	uc := biz.NewAgentUsecase(repo, nil, nil, sidecarAssetStore{data: []byte("asset-body")}, nil)
	handler := NewAgentHandler(uc)
	router := gin.New()
	router.GET("/api/v1/agent-assets/:assetId", GinAuthMiddleware(testAuthUsecase()), handler.GetAsset)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-assets/9", nil)
	request.AddCookie(&http.Cookie{Name: biz.SessionCookieName, Value: "valid-sid"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain")
	}
	if got := response.Body.String(); got != "asset-body" {
		t.Fatalf("body = %q, want %q", got, "asset-body")
	}
}
