package action

import (
	"encoding/json"
	"net/http"
)

type stdResp struct {
	Status bool        `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
}

func Send(res http.ResponseWriter, code int, ct string, data []byte) {
	header := res.Header()
	header.Set("Content-Type", ct+"; charset=utf-8")
	res.WriteHeader(code)
	res.Write(data)
}

func StdJSONResp(res http.ResponseWriter, data interface{}, code int, msg string) {
	if 0 == code {
		code = 200
	}

	if "" == msg {
		msg = http.StatusText(code)
	}

	buf, _ := json.Marshal(&stdResp{
		Status: 2 == code/100,
		Msg:    msg,
		Data:   data,
	})

	Send(res, code, "application/json", buf)
}
