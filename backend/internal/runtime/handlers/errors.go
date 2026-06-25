package handlers

import (
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

func errorBody(err error) gin.H {
	body := gin.H{"error": err.Error()}
	if code, ok := services.ErrorCodeForError(err); ok {
		body["code"] = code
	}
	return body
}
