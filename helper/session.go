package helper

import (
	"net/http"

	"github.com/watsonserve/goengine"
)

type RedisConf struct {
	RedisAddress  string
	RedisPassword string
	SessName      string
	CookiePrefix  string
	SessionPrefix string
	Domain        string
}

type SessMgr struct {
	goengine.SessionManager
}

type openUser struct {
	OpenId string `json:"open_id"`
}

func InitSessMgr(c *RedisConf) *SessMgr {
	return &SessMgr{goengine.InitSessionManager(
		goengine.NewRedisStore(c.RedisAddress, c.RedisPassword, 1),
		c.SessName,
		c.CookiePrefix,
		c.SessionPrefix,
		c.Domain,
	)}
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
