package helper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/watsonserve/goengine"
)

func GenUUIDStr() (string, error) {
	var buf [32]byte
	__uuid, err := uuid.NewV7()
	if nil != err {
		return "", err
	}
	hex.Encode(buf[:], __uuid[:])
	return string(buf[:]), nil
}

/**
 * @return baseName
 */
func createNewFile(dir, ext string, perm os.FileMode) (string, *os.File, error) {
	if 0 < len(ext) && '.' != ext[0] {
		ext = "." + ext
	}
	if "" == dir {
		dir = "/"
	}

	for i := 0; i < 16; i++ {
		baseName, err := GenUUIDStr()
		if nil != err {
			return "", nil, err
		}
		fileName := path.Join(dir, baseName+ext)
		_, err = os.Stat(fileName)
		if os.IsNotExist(err) {
			dst, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
			return baseName, dst, err
		}
	}
	return "", nil, errors.New("retry timeout")
}

func CreateNewFile(dir, ext, digest string, src io.Reader) (string, int64, int64, error) {
	siz := int64(0)
	cTime := int64(0)
	eTag, fp, err := createNewFile(dir, ext, 0660)

	for nil == err {
		defer fp.Close()

		siz, err = io.Copy(fp, src)
		if nil != err {
			break
		}
		_, err = fp.Seek(0, 0)
		if nil != err {
			break
		}
		var hash string
		hash, err = Sha256ByFile(fp)
		if nil != err {
			break
		}
		if hash != digest {
			err = errors.New("Digest Not Match")
		}
		break
	}

	return eTag, siz, cTime, err
}

func GetOrigin(header *http.Header) *url.URL {
	strURL := strings.TrimSpace(header.Get("Origin"))
	if "" != strURL {
		uri, err := url.Parse(strURL)
		if nil == err {
			return uri
		}
	}

	strURL = strings.TrimSpace(header.Get("Referer"))
	if "" != strURL {
		uri, err := url.Parse(strURL)
		if nil == err {
			return &url.URL{
				Scheme: uri.Scheme,
				Host:   uri.Host,
			}
		}
	}

	strURL = strings.TrimSpace(header.Get("Host"))
	if "" == strURL {
		return nil
	}
	return &url.URL{
		Scheme: "https:",
		Host:   strURL,
	}
}

func GetEncodeType(header *http.Header) string {
	return header.Get("Content-Encoding")
}

func GetExtNameByReq(header *http.Header) ([]string, error) {
	contentType := strings.Split(header.Get("Content-Type"), ";")[0]
	return mime.ExtensionsByType(contentType)
}

func GetContentLength(header *http.Header) int64 {
	contentLength, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	if nil != err {
		return -1
	}
	return contentLength
}

type Segment struct {
	Start int32
	End   int32
}

func GetRange(header *http.Header) []Segment {
	// bytes=200-1000, 2000-6576, 19000-
	rangeVal := strings.Split(header.Get("Range"), "=")
	if len(rangeVal) < 2 {
		return nil
	}
	rangeList := strings.Split(rangeVal[1], ",")
	results := make([]Segment, len(rangeList))

	for i, seg := range rangeList {
		sep := strings.Split(strings.TrimSpace(seg), "-")
		offset, err := strconv.ParseInt(sep[0], 10, 32)
		if nil != err || offset < 0 {
			offset = 0
		}
		end, err := strconv.ParseInt(sep[1], 10, 32)
		if nil != err || end < offset {
			end = -1 // infinity
		}
		results[i] = Segment{Start: int32(offset), End: int32(end)}
	}

	return results
}

func Sha256ByFile(fp *os.File) (string, error) {
	hasher := sha256.New()
	_, err := io.Copy(hasher, fp)
	if nil != err {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type Meta struct {
	Size        int32
	ModTime     time.Time
	ContentType string
	Sha256Hash  string
}

func GetMeta(fp *os.File) (*Meta, error) {
	stat, err := fp.Stat()
	if nil != err {
		return nil, err
	}

	hash, err := Sha256ByFile(fp)
	if nil != err {
		return nil, err
	}
	return &Meta{
		Size:        int32(stat.Size()),
		ModTime:     stat.ModTime(),
		ContentType: mime.TypeByExtension(path.Ext(stat.Name())),
		Sha256Hash:  hash,
	}, nil
}

/**
 * digestKey: sha-1, sha-256, ...
 */
func GetDigest(header *http.Header, digestKey string) string {
	contentDigest := header.Get("Content-Digest")
	if "" == contentDigest {
		return ""
	}

	reprDigest := strings.Split(strings.ToLower(contentDigest), ",")
	for _, item := range reprDigest {
		kv := strings.Split(strings.TrimSpace(item), "=")
		if len(kv) < 2 {
			continue
		}
		key := kv[0]
		if digestKey == key {
			val := kv[1]
			return val[1 : len(val)-1]
		}
	}
	return ""
}

func Write(fp *os.File, offset int64, src io.Reader) error {
	var err error = nil
	if 0 < offset {
		_, err = fp.Seek(offset, 0)
	}
	if nil == err {
		_, err = io.Copy(fp, src)
	}
	return err
}

type UsrSess struct {
	OpenId string `json:"open_id"`
}

func SetUid(sgr goengine.SessionManager, resp http.ResponseWriter, req *http.Request, uid string) error {
	sess := sgr.LoadSession(req)
	usr := &UsrSess{OpenId: uid}
	var err error
	if err := sess.Set("user", usr); nil == err {
		err = sgr.Save(resp, sess, -1)
	}
	return err
}

func GetUid(sgr goengine.SessionManager, req *http.Request) string {
	sess := sgr.LoadSession(req)
	usr := &UsrSess{}
	err := sess.Load("user", usr)
	if nil != err {
		return ""
	}
	return usr.OpenId
}

type ETag struct {
	Value string
	W     bool
}

func getMatchVal(eTag string) *ETag {
	length := len(eTag)
	if 0 == length {
		return nil
	}
	ret := &ETag{Value: "", W: strings.HasPrefix(eTag, "W/\"")}
	offset := 1
	if ret.W {
		offset |= 2
	}
	ret.Value = eTag[offset : length-1]
	if "" == ret.Value {
		return nil
	}
	return ret
}

func GetMatch(header *http.Header) *ETag {
	return getMatchVal(header.Get("If-Match"))
}

func GetNoneMatch(header *http.Header) *ETag {
	return getMatchVal(header.Get("If-None-Match"))
}

func GetFileName(pathName string) string {
	length := len(pathName)
	i := length - 1
	if i < 0 {
		return ""
	}
	for ; 0 < i && '/' != pathName[i]; i-- {

	}
	if '/' == pathName[i] {
		i++
	}
	if length <= i {
		return ""
	}
	return pathName[i:]
}

func GenPreview(rootPath, baseName, extName string) error {
	absPath := path.Join(rootPath, "raw", baseName+extName)
	genFile := baseName + ".webp"
	preview := path.Join(rootPath, "preview", genFile)
	thumb := path.Join(rootPath, "thumb", genFile)

	fmt.Printf("gen-preview --thumb=%s --preview=%s %s\n", thumb, preview, absPath)
	// mat, err := imghelper.IMRead(absPath)
	// if nil != err {
	// 	return err
	// }

	// err = imghelper.IMWrite(mat, preview, 64, 960)
	// if nil == err {
	// 	err = imghelper.IMWrite(mat, thumb, 50, 320)
	// }
	return nil
}
