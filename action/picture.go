package action

import (
	"fmt"
	"net/http"

	"github.com/watsonserve/galleried/services"
)

type PictureAction struct {
	prefixLen int
	listSrv   http.Handler
	dav       http.Handler
}

var imgCache = map[string]bool{"thumb": true, "preview": true, "raw": true}

func NewPictureAction(prefixLen int, listSrv http.Handler, fileSrv http.Handler) *PictureAction {
	return &PictureAction{
		prefixLen: prefixLen,
		listSrv:   listSrv,
		dav:       fileSrv,
	}
}

func (d *PictureAction) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	subPath := req.URL.Path[d.prefixLen:]

	if "/" == subPath {
		d.listSrv.ServeHTTP(resp, req)
		return
	}

	query := req.URL.Query()
	lev := query.Get("lev")
	if "" == lev {
		lev = "raw"
	}
	if !imgCache[lev] {
		services.StdJSONResp(resp, nil, http.StatusNotFound, "")
		return
	}
	if "raw" != lev && http.MethodGet != req.Method && http.MethodHead != req.Method {
		services.StdJSONResp(resp, nil, http.StatusMethodNotAllowed, "")
		return
	}
	req.URL.Path = fmt.Sprintf("/%s%s", lev, subPath)
	d.dav.ServeHTTP(resp, req)
}
