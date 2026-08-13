package action

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/galleried/services"
)

type PictureAction struct {
	prefixLen int
	sgr       *helper.SessMgr
	listSrv   *services.ListService
	dav       *services.FileService
}

type UploadParams struct {
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

func getPrams(req *http.Request) (*UploadParams, int, string) {
	reqHeader := &req.Header
	cType := strings.Split(reqHeader.Get("Content-Type"), ";")[0]
	origin := helper.GetOrigin(reqHeader)
	digest := helper.GetDigest(reqHeader, "sha-256")
	ifMatch := getIfMatch(reqHeader)
	fileName := helper.GetFileName(req.URL.Path)

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
		Origin:   origin,
		Digest:   digest,
		IfMatch:  ifMatch,
		FileName: fileName,
	}, 0, ""
}

func NewPictureAction(prefixLen int, sgr *helper.SessMgr, listSrv *services.ListService, fileSrv *services.FileService) *PictureAction {
	return &PictureAction{
		prefixLen: prefixLen,
		sgr:       sgr,
		listSrv:   listSrv,
		dav:       fileSrv,
	}
}

func (d *PictureAction) list(rsp http.ResponseWriter, req *http.Request, isRecycled bool) {
	if http.MethodGet != req.Method {
		StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
		return
	}
	uid := d.sgr.GetUid(rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}
	rangeList := helper.GetRange(&req.Header)
	list, err := d.listSrv.List(uid, isRecycled, rangeList)
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusServiceUnavailable, err.Error())
		return
	}
	StdJSONResp(rsp, list, 0, "")
}

func (d *PictureAction) read(rsp http.ResponseWriter, req *http.Request, isRecycled bool) {
	uid := d.sgr.GetUid(rsp, req)
	cachedETag := helper.GetNoneMatch(&req.Header)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}

	meta, stat, msg := d.dav.SendFile(uid, req.URL.Path, http.MethodHead == req.Method, isRecycled, cachedETag)
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

func (d *PictureAction) move(rsp http.ResponseWriter, req *http.Request, isRecycled bool) {
	uid := d.sgr.GetUid(rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}

	etag := getIfMatch(&req.Header)
	dst := helper.GetDestination(&req.Header)
	src := path.Clean(req.URL.Path)

	if (!isRecycled || "/" != dst) && (isRecycled || "/recycle/" != dst) {
		StdJSONResp(rsp, nil, http.StatusConflict, "")
		return
	}

	err := d.dav.Recycle(uid, src, etag, "/recycle/" == dst)
	if nil != err {
		StdNilJSONResp(rsp, err.Error())
		return
	}
	rsp.WriteHeader(http.StatusNoContent)
	rsp.Write(nil)
}

func (d *PictureAction) write(rsp http.ResponseWriter, req *http.Request) {
	uid := d.sgr.GetUid(rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}
	params, stat, msg := getPrams(req)
	if 0 != stat {
		StdJSONResp(rsp, nil, stat, msg)
		return
	}

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
	uid := d.sgr.GetUid(rsp, req)
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

func (d *PictureAction) remove(rsp http.ResponseWriter, req *http.Request) {
	uid := d.sgr.GetUid(rsp, req)
	if "" == uid {
		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
		return
	}

	etag := getIfMatch(&req.Header)
	if "" == etag {
		StdJSONResp(rsp, nil, http.StatusPreconditionFailed, "")
		return
	}

	fileName := helper.GetFileName(req.URL.Path)
	stat, msg, err := d.dav.DeleteFile(uid, fileName, etag)
	if nil != err || 0 != stat {
		StdJSONResp(rsp, nil, stat, msg)
		return
	}
	rsp.WriteHeader(http.StatusNoContent)
	rsp.Write(nil)
}

func (d *PictureAction) levPath(req *http.Request) int {
	uri := req.URL
	subPath := uri.Path[d.prefixLen:]
	isRecycle := strings.HasPrefix(subPath, "/recycle/")
	if isRecycle {
		subPath = subPath[len("/recycle"):]
	}

	lev := uri.Query().Get("lev")
	if "" == lev {
		lev = "raw"
	}
	if !imgCache[lev] {
		return http.StatusNotFound
	}
	if "raw" != lev && http.MethodGet != req.Method && http.MethodHead != req.Method {
		return http.StatusMethodNotAllowed
	}
	uri.Path = fmt.Sprintf("/%s%s", lev, subPath)
	uri.RawQuery = fmt.Sprintf("recycle=%d", isRecycle)
	return 0
}

func (d *PictureAction) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
	code := d.levPath(req)
	if 0 != code {
		StdJSONResp(rsp, nil, code, "")
		return
	}

	isRecycled := "1" == req.URL.Query().Get("recycle")

	if "/" == req.URL.Path {
		d.list(rsp, req, isRecycled)
		return
	}

	switch req.Method {
	case http.MethodHead:
		fallthrough
	case http.MethodGet:
		d.read(rsp, req, isRecycled)
		return
	case "MOVE":
		d.move(rsp, req, isRecycled)
		return
	case http.MethodPut:
		if !isRecycled {
			d.write(rsp, req)
			return
		}
	case http.MethodPost:
		if !isRecycled {
			d.preview(rsp, req)
			return
		}
	case http.MethodDelete:
		if isRecycled {
			d.remove(rsp, req)
			return
		}
	default:
	}
	StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
}
