package services

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/watsonserve/galleried/dao"
	"github.com/watsonserve/galleried/helper"
)

type FileService struct {
	rootPath string
	dbi      *dao.DBI
}

const (
	Removed  = 1 // 001
	Existed  = 3 // 011
	NotMatch = 5 // 101
	ToCreate = 0 // 000
	ToUpdate = 2 // 010
)

func NewFileService(dbi *dao.DBI, root string) *FileService {
	return &FileService{
		rootPath: path.Clean(root),
		dbi:      dbi,
	}
}

func (d *FileService) checkOption(uid, fileName, ifMatch string) int {
	eTagVal, err := d.dbi.Info(uid, fileName)

	// not found
	if nil != err {
		if "" == ifMatch {
			return ToCreate
		}
		return Removed
	}

	if "" == ifMatch {
		return Existed
	}

	// not matched
	if ifMatch != eTagVal {
		return NotMatch
	}
	return ToUpdate
}

func (d *FileService) getLocalFilename(reqPath, baseName, extName string) string {
	dirPath := path.Base(path.Dir(reqPath))
	return path.Clean(path.Join(d.rootPath, dirPath, baseName+extName))
}

func (d *FileService) SendFile(resp http.ResponseWriter, req *http.Request) {
	uid := helper.GetUid(req)
	fileName := helper.GetFileName(req.URL.Path)
	cachedETag := helper.GetNoneMatch(&req.Header)

	if "" == uid {
		StdJSONResp(resp, nil, http.StatusUnauthorized, "")
		return
	}
	eTagVal, err := d.dbi.Info(uid, fileName)
	if nil != err {
		StdJSONResp(resp, nil, http.StatusNotFound, "")
		return
	}
	if nil != cachedETag && !cachedETag.W && cachedETag.Value == eTagVal {
		resp.WriteHeader(http.StatusNotModified)
		resp.Write(nil)
		return
	}

	absPath := d.getLocalFilename(req.URL.Path, eTagVal, path.Ext(fileName))
	fp, err := os.Open(absPath)
	if nil != err {
		StdJSONResp(resp, nil, http.StatusNotFound, err.Error())
		return
	}
	defer fp.Close()

	meta, err := helper.GetMeta(fp)
	if nil != err {
		StdJSONResp(resp, nil, http.StatusBadRequest, err.Error())
		return
	}

	respHeader := resp.Header()
	respHeader.Set("Vary", "Cookie")
	respHeader.Set("Content-Type", meta.ContentType)
	respHeader.Set("Content-Length", fmt.Sprintf("%d", meta.Size))
	respHeader.Set("Content-Digest", fmt.Sprintf("sha-256=:%s:", meta.Sha256Hash))
	// respHeader.Set("Last-Modified", meta.ModTime.String())
	respHeader.Set("ETag", "\""+eTagVal+"\"")
	if http.MethodHead == req.Method {
		resp.Write(nil)
		return
	}
	io.Copy(resp, fp)
}

func (d *FileService) Upload(resp http.ResponseWriter, req *http.Request) {
	reqHeader := &req.Header
	cType := strings.Split(reqHeader.Get("Content-Type"), ";")[0]
	encoding := helper.GetEncodeType(reqHeader)
	origin := helper.GetOrigin(reqHeader)
	digest := helper.GetDigest(reqHeader, "sha-256")
	matchETag := helper.GetMatch(reqHeader)
	siz := helper.GetContentLength(reqHeader)
	uid := helper.GetUid(req)
	fileName := helper.GetFileName(req.URL.Path)

	if "" == uid {
		StdJSONResp(resp, nil, http.StatusUnauthorized, "")
		return
	}
	if !strings.HasPrefix(cType, "image/") {
		StdJSONResp(resp, nil, http.StatusUnsupportedMediaType, "Accept Image Only")
		return
	}
	if nil == origin {
		StdJSONResp(resp, nil, http.StatusBadRequest, "Header Origin Not Found")
		return
	}
	if "" == digest {
		StdJSONResp(resp, nil, http.StatusBadRequest, "Content-Digest sha-256 Required")
		return
	}

	ifMatch := ""
	if nil != matchETag {
		if matchETag.W {
			StdJSONResp(resp, nil, http.StatusPreconditionFailed, "")
			return
		} else {
			ifMatch = matchETag.Value
		}
	}

	opt := d.checkOption(uid, fileName, ifMatch)
	switch opt {
	case Removed:
		StdJSONResp(resp, nil, http.StatusGone, "")
		return
	case Existed:
		StdJSONResp(resp, nil, http.StatusForbidden, "Existed")
		return
	case NotMatch:
		StdJSONResp(resp, nil, http.StatusPreconditionFailed, "")
		return
	default:
	}

	body := req.Body
	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(req.Body)
		if nil != err {
			StdJSONResp(resp, nil, http.StatusBadRequest, "")
			return
		}
		body = reader
	case "":
	default:
		StdJSONResp(resp, nil, http.StatusBadRequest, "unsurpported Content-Encoding "+encoding)
		return
	}
	defer body.Close()
	eTagVal, siz, cTime, err := helper.CreateNewFile(path.Join(d.rootPath, "raw"), path.Ext(fileName), digest, body)
	if nil != err {
		StdJSONResp(resp, nil, http.StatusServiceUnavailable, err.Error())
		return
	}

	if ToCreate == opt {
		err = d.dbi.Insert(uid, eTagVal, digest, fileName, siz, cTime)
	} else {
		err = d.dbi.Update(uid, eTagVal, digest, fileName, siz)
	}
	if nil != err {
		StdJSONResp(resp, nil, http.StatusBadRequest, err.Error())
		return
	}
	origin.Path = req.URL.Path[4:]
	respHeader := resp.Header()
	respHeader.Set("Location", origin.String())
	respHeader.Set("ETag", "\""+eTagVal+"\"")
	StdJSONResp(resp, nil, http.StatusCreated, "")
}

func (d *FileService) GenPreview(resp http.ResponseWriter, req *http.Request) {
	fileName := helper.GetFileName(req.URL.Path)
	uid := helper.GetUid(req)
	if "" == uid {
		StdJSONResp(resp, nil, http.StatusUnauthorized, "")
		return
	}

	eTagVal, err := d.dbi.Info(uid, fileName)
	if nil != err {
		StdJSONResp(resp, nil, http.StatusNotFound, "")
		return
	}

	err = helper.GenPreview(d.rootPath, eTagVal, path.Ext(fileName))
	if nil != err {
		StdJSONResp(resp, nil, http.StatusNotFound, err.Error())
		return
	}
	StdJSONResp(resp, nil, http.StatusCreated, "")
}

func (d *FileService) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodHead:
		fallthrough
	case http.MethodGet:
		d.SendFile(resp, req)
		return
	case http.MethodPut:
		d.Upload(resp, req)
		return
	case http.MethodPost:
		d.GenPreview(resp, req)
		return
	default:
	}
	StdJSONResp(resp, nil, http.StatusMethodNotAllowed, "")
}
