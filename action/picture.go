package action

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/galleried/services"
	"github.com/watsonserve/goengine"
)

type PictureAction struct {
	prefixLen int
	sgr       goengine.SessionManager
	listSrv   *services.ListService
	dav       *services.FileService
}

type UploadParams struct {
	Uid      string
	Digest   string
	IfMatch  string
	FileName string
	Origin   *url.URL
}

var imgCache = map[string]bool{"thumb": true, "preview": true, "raw": true}

func getIfMatch(reqHeader *http.Header) string {
	matchETag := helper.GetMatch(reqHeader)
	ifMatch := ""
	if nil != matchETag {
		if matchETag.W {
			return ""
		} else {
			ifMatch = matchETag.Value
		}
	}
	return ifMatch
}

func getInStream(req *http.Request) (io.ReadCloser, error) {
	encoding := helper.GetEncodeType(&req.Header)
	switch encoding {
	case "gzip":
		return gzip.NewReader(req.Body)
	case "":
		return req.Body, nil
	default:
		return nil, errors.New("unsurpported Content-Encoding " + encoding)
	}
}

func NewPictureAction(prefixLen int, sgr goengine.SessionManager, listSrv *services.ListService, fileSrv *services.FileService) *PictureAction {
	return &PictureAction{
		prefixLen: prefixLen,
		sgr:       sgr,
		listSrv:   listSrv,
		dav:       fileSrv,
	}
}

func (d *PictureAction) getPrams(rsp http.ResponseWriter, req *http.Request) (*UploadParams, int, string) {
	reqHeader := &req.Header
	uid := helper.GetUid(d.sgr, rsp, req)
	cType := strings.Split(reqHeader.Get("Content-Type"), ";")[0]
	origin := helper.GetOrigin(reqHeader)
	digest := helper.GetDigest(reqHeader, "sha-256")
	ifMatch := getIfMatch(reqHeader)
	fileName := helper.GetFileName(req.URL.Path)

	if "" == uid {
		return nil, http.StatusUnauthorized, ""
	}
	if !strings.HasPrefix(cType, "image/") {
		return nil, http.StatusUnsupportedMediaType, "Accept Image Only"
	}
	if nil == origin {
		return nil, http.StatusBadRequest, "Header Origin Not Found"
	}
	if "" == digest {
		return nil, http.StatusBadRequest, "Content-Digest sha-256 Required"
	}
	if "" == ifMatch {
		return nil, http.StatusPreconditionFailed, ""
	}

	return &UploadParams{
		Uid:      uid,
		Origin:   origin,
		Digest:   digest,
		IfMatch:  ifMatch,
		FileName: fileName,
	}, 0, ""
}

func (d *PictureAction) read(rsp http.ResponseWriter, req *http.Request) {
	uid := helper.GetUid(d.sgr, rsp, req)
	cachedETag := helper.GetNoneMatch(&req.Header)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}

	meta, stat, msg := d.dav.SendFile(uid, req.URL.Path, http.MethodHead == req.Method, cachedETag)
	if nil == meta {
		if http.StatusNotModified == stat {
			rsp.WriteHeader(http.StatusNotModified)
			rsp.Write(nil)
		} else {
			StdJSONResp(rsp, nil, stat, msg)
		}
		return
	}
	respHeader := rsp.Header()
	respHeader.Set("Vary", "Cookie")
	respHeader.Set("Content-Type", meta.ContentType)
	respHeader.Set("Content-Length", fmt.Sprintf("%d", meta.Size))
	respHeader.Set("Content-Digest", fmt.Sprintf("sha-256=:%s:", meta.Sha256Hash))
	// respHeader.Set("Last-Modified", meta.ModTime.String())
	respHeader.Set("ETag", "\""+meta.ETag+"\"")
	outStream := meta.OutStream
	if nil == outStream {
		rsp.Write(nil)
		return
	}
	defer outStream.Close()
	io.Copy(rsp, outStream)
}

func (d *PictureAction) write(rsp http.ResponseWriter, req *http.Request) {
	params, stat, msg := d.getPrams(rsp, req)
	if 0 != stat {
		StdJSONResp(rsp, nil, stat, msg)
		return
	}

	uid := params.Uid
	fileName := params.FileName
	opt := d.dav.CheckOption(uid, fileName, params.IfMatch)
	switch opt {
	case services.Removed:
		StdJSONResp(rsp, nil, http.StatusGone, "")
		return
	case services.Existed:
		StdJSONResp(rsp, nil, http.StatusForbidden, "Existed")
		return
	case services.NotMatch:
		StdJSONResp(rsp, nil, http.StatusPreconditionFailed, "")
		return
	default:
	}

	body, err := getInStream(req)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
		return
	}
	defer body.Close()

	digest := params.Digest
	eTagVal, siz, cTime, err := d.dav.WriteFile(fileName, digest, body)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusServiceUnavailable, err.Error())
		return
	}
	err = d.dav.WriteIndex(opt, uid, eTagVal, digest, fileName, siz, cTime)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
		return
	}

	origin := params.Origin
	origin.Path = req.URL.Path[4:]
	respHeader := rsp.Header()
	respHeader.Set("Location", origin.String())
	respHeader.Set("ETag", "\""+eTagVal+"\"")
	StdJSONResp(rsp, nil, http.StatusCreated, "")
}

func (d *PictureAction) preview(rsp http.ResponseWriter, req *http.Request) {
	fileName := helper.GetFileName(req.URL.Path)
	uid := helper.GetUid(d.sgr, rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}
	err := d.dav.GenPreview(uid, fileName)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusNotFound, err.Error())
		return
	}
	StdJSONResp(rsp, nil, http.StatusCreated, "")
}

func (d *PictureAction) list(rsp http.ResponseWriter, req *http.Request) {
	if http.MethodGet != req.Method {
		StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
		return
	}
	uid := helper.GetUid(d.sgr, rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}
	rangeList := helper.GetRange(&req.Header)
	list, err := d.listSrv.List(uid, rangeList)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusServiceUnavailable, err.Error())
		return
	}
	StdJSONResp(rsp, list, 0, "")
}

func (d *PictureAction) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
	subPath := req.URL.Path[d.prefixLen:]

	if "/" == subPath {
		d.list(rsp, req)
		return
	}

	lev := req.URL.Query().Get("lev")
	if "" == lev {
		lev = "raw"
	}
	if !imgCache[lev] {
		StdJSONResp(rsp, nil, http.StatusNotFound, "")
		return
	}
	if "raw" != lev && http.MethodGet != req.Method && http.MethodHead != req.Method {
		StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
		return
	}
	req.URL.Path = fmt.Sprintf("/%s%s", lev, subPath)

	switch req.Method {
	case http.MethodHead:
		fallthrough
	case http.MethodGet:
		d.read(rsp, req)
		return
	case http.MethodPut:
		d.write(rsp, req)
		return
	case http.MethodPost:
		d.preview(rsp, req)
		return
	default:
	}
	StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
}
