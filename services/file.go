package services

import (
	"io"
	"net/http"
	"os"
	"path"

	"github.com/watsonserve/galleried/dao"
	"github.com/watsonserve/galleried/helper"
)

type FileService struct {
	rootPath string
	dbi      *dao.DBI
}

type FileMeta struct {
	Size        int32
	ContentType string
	Sha256Hash  string
	ETag        string
	OutStream   io.ReadCloser
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

func (d *FileService) CheckOption(uid, fileName, ifMatch string) int {
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

func (d *FileService) SendFile(uid, urlPath string, infoOnly bool, cachedETag *helper.ETag) (*FileMeta, int, string) {
	fileName := helper.GetFileName(urlPath)
	eTagVal, err := d.dbi.Info(uid, fileName)
	if nil != err {
		return nil, http.StatusNotFound, ""
	}
	if nil != cachedETag && !cachedETag.W && cachedETag.Value == eTagVal {
		return nil, http.StatusNotModified, ""
	}

	absPath := d.getLocalFilename(urlPath, eTagVal, path.Ext(fileName))
	fp, err := os.Open(absPath)
	if nil != err {
		return nil, http.StatusNotFound, err.Error()
	}

	meta, err := helper.GetMeta(fp)
	if nil != err {
		fp.Close()
		return nil, http.StatusBadRequest, err.Error()
	}

	out := &FileMeta{
		ContentType: meta.ContentType,
		Size:        meta.Size,
		Sha256Hash:  meta.Sha256Hash,
		ETag:        eTagVal,
		OutStream:   nil,
	}
	if infoOnly {
		fp.Close()
	} else {
		out.OutStream = fp
	}
	return out, 0, ""
}

func (d *FileService) WriteFile(fileName, digest string, body io.ReadCloser) (string, int64, int64, error) {
	return helper.CreateNewFile(path.Join(d.rootPath, "raw"), path.Ext(fileName), digest, body)
}

func (d *FileService) WriteIndex(opt int, uid, eTagVal, digest, fileName string, siz, cTime int64) error {
	if ToCreate == opt {
		return d.dbi.Insert(uid, eTagVal, digest, fileName, siz, cTime)
	}
	return d.dbi.Update(uid, eTagVal, digest, fileName, siz)
}

func (d *FileService) GenPreview(uid, fileName string) error {
	eTagVal, err := d.dbi.Info(uid, fileName)
	if nil == err {
		err = helper.GenPreview(d.rootPath, eTagVal, path.Ext(fileName))
	}

	return err
}
