package service

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"baboflow/internal/biz"
)

func TestSkillErrMapsInternalErrorsToServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &SkillHandler{}
	h.skillErr(c, &biz.SkillInternalError{Err: errors.New("workspace unavailable")})

	if w.Code != 500 {
		t.Fatalf("expected HTTP 500, got %d", w.Code)
	}
}
