package action

import (
	"crypto/sha256"
	"io"
	"net/http"

	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/otp"
	"github.com/watsonserve/pass_sdk"
)

type UserAction struct {
	sgr         *helper.SessMgr
	appIdSecret []string // [appId, appSecret]
}

func NewUserAction(appIdSecret []string, sgr *helper.SessMgr) *UserAction {
	return &UserAction{
		sgr:         sgr,
		appIdSecret: appIdSecret,
	}
}

func (d *UserAction) IsCheckedIn(rsp http.ResponseWriter, req *http.Request) bool {
	uid := d.sgr.GetUid(rsp, req)
	return "" != uid
}

func (d *UserAction) Error(rsp http.ResponseWriter, req *http.Request, code int, explain string) {
	StdJSONResp(rsp, nil, code, "")
}

func (d *UserAction) User(rsp http.ResponseWriter, req *http.Request, usr *pass_sdk.UserData, rd string) {
	d.sgr.SetUid(rsp, req, usr.OpenId)
	StdJSONResp(rsp, nil, http.StatusOK, "")
}

func (d *UserAction) loadOpenId(cookies []*http.Cookie) (string, error) {
	r, err := http.NewRequest(http.MethodGet, "https://passport.watsonserve.com/api/open-user.json", nil)
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
