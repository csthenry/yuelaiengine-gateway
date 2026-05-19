package httperr

import (
	"encoding/json"
	"net/http"
)

// Body 定义统一错误响应格式。
type Body struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Write 输出统一 JSON 错误。
func Write(w http.ResponseWriter, status int, code, message string) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if code == "" {
		code = "INTERNAL_ERROR"
	}
	if message == "" {
		message = http.StatusText(status)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Body{
		Code:    code,
		Message: message,
	})
}
