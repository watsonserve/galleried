package helper

import (
	"net/http"

	"github.com/watsonserve/goengine"
)

type SessMgr struct {
	goengine.SessionManager
}

type openUser struct {
	OpenId string `json:"open_id"`
}

func InitSessMgr(storer goengine.SessionStore, sessName string, cookiePrefix string, sessionPrefix string, domain string) *SessMgr {
	return &SessMgr{goengine.InitSessionManager(storer, sessName, cookiePrefix, sessionPrefix, domain)}
}

func (sgr *SessMgr) SetUid(rsp http.ResponseWriter, req *http.Request, uid string) error {
	sess := sgr.LoadSession(rsp, req)
	usr := &openUser{OpenId: uid}
	var err error
	if err := sess.Set("user", usr); nil == err {
		err = sgr.Save(rsp, sess, -1)
	}
	return err
}

func (sgr *SessMgr) GetUid(rsp http.ResponseWriter, req *http.Request) string {
	sess := sgr.LoadSession(rsp, req)
	usr := &openUser{}
	err := sess.Load("user", usr)
	if nil != err {
		return ""
	}
	return usr.OpenId
}
