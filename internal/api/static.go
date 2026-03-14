package api

import (
	"net/http"

	"github.com/ms/agent-daemon/web"
)

func staticHandler() http.Handler {
	return http.FileServer(http.FS(web.FS))
}
