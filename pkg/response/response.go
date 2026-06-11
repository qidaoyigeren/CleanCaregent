package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:      "OK",
		Message:   "success",
		RequestID: RequestID(c),
		Data:      data,
	})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{
		Code:      "OK",
		Message:   "success",
		RequestID: RequestID(c),
		Data:      data,
	})
}

func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Envelope{
		Code:      "OK",
		Message:   "accepted",
		RequestID: RequestID(c),
		Data:      data,
	})
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Envelope{
		Code:      code,
		Message:   message,
		RequestID: RequestID(c),
	})
}

func RequestID(c *gin.Context) string {
	value, _ := c.Get("request_id")
	requestID, _ := value.(string)
	return requestID
}
