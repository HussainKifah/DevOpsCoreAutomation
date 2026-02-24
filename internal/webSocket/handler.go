package websocket

import (
	"net/http"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ServerWs(hub *Hub, jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token, err := ctx.Cookie("access_token")
		if err != nil || token == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "not authenticatd",
			})
			return
		}
		_, err = jwtManager.ValidateToken(token)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			return
		}
		conn, err := upgrader.Upgrade(
			ctx.Writer, ctx.Request, nil,
		)
		if err != nil {
			return
		}
		client := NewClient(hub, conn)
		hub.register <- client

		go client.WritePump()
		go client.ReadPump()
	}
}
