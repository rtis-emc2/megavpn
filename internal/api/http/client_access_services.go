package http

import (
	nethttp "net/http"

	"github.com/rtis-emc2/megavpn/internal/clientaccess"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

func (s *Server) listClientAccessServices(w nethttp.ResponseWriter, r *nethttp.Request) {
	writeJSON(w, 200, clientAccessServiceCatalog())
}

func clientAccessServiceCatalog() []domain.ClientAccessService {
	return clientaccess.Catalog()
}
