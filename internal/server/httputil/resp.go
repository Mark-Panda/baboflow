package httputil

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 统一响应信封 {code,message,data}
const (
	CodeOK          = 0
	CodeBadRequest  = 400
	CodeUnauthorized = 401
	CodeForbidden   = 403
	CodeNotFound    = 404
	CodeConflict    = 409
	CodeInternal    = 500
)

type envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, envelope{Code: CodeOK, Message: "ok", Data: data})
}

func Fail(c *gin.Context, code int, message string) {
	// HTTP 状态码与业务 code 对齐（限流 429 等非标准码回退 200，由信封 code 表达）。
	status := code
	if status < 400 || status > 599 {
		status = http.StatusOK
	}
	c.JSON(status, envelope{Code: code, Message: message})
}

func BadRequest(c *gin.Context, message string)  { Fail(c, CodeBadRequest, message) }
func Unauthorized(c *gin.Context, message string) { Fail(c, CodeUnauthorized, message) }
func NotFound(c *gin.Context, message string)     { Fail(c, CodeNotFound, message) }
func Conflict(c *gin.Context, message string)     { Fail(c, CodeConflict, message) }
func Internal(c *gin.Context, message string)     { Fail(c, CodeInternal, message) }

// Page 分页返回
type Page struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

func OKPage(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	OK(c, Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// PageParams 解析 page/pageSize，默认 1/20。
func PageParams(c *gin.Context) (page, pageSize int) {
	page = queryInt(c, "page", 1)
	pageSize = queryInt(c, "pageSize", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return
}

func queryInt(c *gin.Context, key string, def int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
