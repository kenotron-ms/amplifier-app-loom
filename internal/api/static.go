package api

import (
	"net/http"

	"github.com/ms/amplifier-app-loom/web"
)

func staticHandler() http.Handler {
	return http.FileServer(http.FS(web.FS))
}
