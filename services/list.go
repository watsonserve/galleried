package services

import (
	"path"

	"github.com/watsonserve/galleried/dao"
	"github.com/watsonserve/galleried/helper"
)

type ListService struct {
	raw string
	dbi *dao.DBI
}

func NewListService(dbi *dao.DBI, root string) *ListService {
	return &ListService{
		raw: path.Clean(path.Join(root, "raw")),
		dbi: dbi,
	}
}

func (d *ListService) List(uid string, rangeList []helper.Segment) ([]dao.ResUserImg, error) {
	return d.dbi.List(uid, rangeList)

}

// func (d *ListService) delt(uid string, rsp http.ResponseWriter, req *http.Request) {
// 	fileName := helper.GetFileName(req.URL.Path)
// 	if "" == uid {
// 		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
// 		return
// 	}
// 	err := d.dbi.Del(uid, fileName)
// 	if nil != err {
// 		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
// 		return
// 	}

// 	StdJSONResp(rsp, nil, 0, "")
// }

// func (d *ListService) drop(uid string, rsp http.ResponseWriter, req *http.Request) {
// 	fileName := helper.GetFileName(req.URL.Path)
// 	if "" == uid {
// 		StdJSONResp(rsp, nil, http.StatusUnauthorized, "")
// 		return
// 	}
// 	err := d.dbi.Drop(uid, fileName)
// 	if nil != err {
// 		StdJSONResp(rsp, nil, http.StatusBadRequest, err.Error())
// 		return
// 	}

// 	StdJSONResp(rsp, nil, 0, "")
// }
