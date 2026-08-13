package action

import (
	"net/http"

	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/pass_sdk"
)

type UserAction struct {
	sgr *helper.SessMgr
}

func NewUserAction(sgr *helper.SessMgr) *UserAction {
	return &UserAction{sgr}
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
