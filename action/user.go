package action

import (
	"crypto/sha256"
	"io"
	"net/http"

	"github.com/watsonserve/goengine"
	"github.com/watsonserve/otp"
)

type UserAction struct {
	sgr         goengine.SessionManager
	appIdSecret []string // [appId, appSecret]
}

func NewUserAction(appIdSecret []string, sgr goengine.SessionManager) *UserAction {
	return &UserAction{
		sgr:         sgr,
		appIdSecret: appIdSecret,
	}
}

func (d *UserAction) loadOpenId(cookies []*http.Cookie) (string, error) {
	r, err := http.NewRequest(http.MethodGet, "https://passport.watsonserve.com/api/open-id.json", nil)
	if nil != err {
		return "", err
	}
	for _, ck := range cookies {
		r.AddCookie(ck)
	}
	appId := d.appIdSecret[0]
	appSecret := d.appIdSecret[1]
	code, err := otp.GenTotp(sha256.New, appSecret)
	if nil != err {
		return "", err
	}
	r.SetBasicAuth(appId, code)
	cli := http.Client{}
	passResp, err := cli.Do(r)
	if nil != err {
		return "", err
	}
	defer passResp.Body.Close()
	buf, err := io.ReadAll(passResp.Body)
	if nil != err {
		return "", err
	}
	return string(buf), nil
}

func (d *UserAction) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
	if http.MethodGet != req.Method {
		StdJSONResp(rsp, nil, http.StatusMethodNotAllowed, "")
		return
	}
	openId, err := d.loadOpenId(req.Cookies())
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
		return
	}
	sess := d.sgr.LoadSession(rsp, req)
	if err = sess.Set("user", map[string]string{"open_id": openId}); nil == err {
		err = d.sgr.Save(rsp, sess, -1)
	}
	if nil != err {
		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
		return
	}
	StdJSONResp(rsp, nil, 0, "")
}
