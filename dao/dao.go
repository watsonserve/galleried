package dao

import (
	"database/sql"
	"errors"
	"path"
	"time"

	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/goengine"
)

type DBI struct {
	goengine.DAO
}

type ResUserImg struct {
	Filename string
	ETag     string
	CTime    int64
}

const selectSQL = "SELECT filename, etag, ctime FROM res_user_img WHERE rtime=0 AND uid=$1 ORDER BY ctime DESC OFFSET $2"

func NewDAO(dbConn *sql.DB) *DBI {
	dao := goengine.InitDAO(dbConn)
	dao.Prepare("real_name", "SELECT raw FROM res_thumb WHERE hash=$1")
	// GET
	dao.Prepare("info", "SELECT etag FROM res_user_img WHERE uid=$1 AND filename=$2 AND rtime=0")
	// LIST
	dao.Prepare("list", selectSQL)
	dao.Prepare("list_limit", selectSQL+" LIMIT $3")
	// DELETE
	dao.Prepare("delt", "UPDATE res_user_img SET rtime=$3 WHERE uid=$1 AND filename=$2 AND rtime=0")
	dao.Prepare("drop", "DELETE FROM res_user_img WHERE uid=$1 AND filename=$2 AND rtime<>0")
	// PUT
	dao.Prepare("inst", "INSERT INTO res_thumb (etag, hash, ext, size) VALUES ($1, $2, $3, $4)")
	dao.Prepare("inst_usr", "INSERT INTO res_user_img (uid, filename, etag, ctime) VALUES ($1, $2, $3, $4)")
	dao.Prepare("updt_usr", "UPDATE res_user_img SET etag=$3 WHERE uid=$1 AND filename=$2 AND rtime=0")

	return &DBI{DAO: *dao}
}

func (dbi *DBI) Info(uid, fileName string) (string, error) {
	row := dbi.StmtMap["info"].QueryRow(uid, fileName)
	eTag := ""
	err := row.Scan(&eTag)
	return eTag, err
}

func (dbi *DBI) selectList(uid string, rangeList []helper.Segment) (rows *sql.Rows, err error) {
	offset := int32(0)
	length := int32(0)

	if nil != rangeList {
		if 1 < len(rangeList) {
			return nil, errors.New("multipart is not be allowed")
		}
		sep := rangeList[0]
		offset = sep.Start
		if -1 != sep.End {
			length = sep.End - sep.Start
		}
	}
	if length < 1 {
		return dbi.StmtMap["list"].Query(uid, offset)
	}
	return dbi.StmtMap["list_limit"].Query(uid, offset, length)
}

func (dbi *DBI) List(uid string, rangeList []helper.Segment) ([]ResUserImg, error) {
	rows, err := dbi.selectList(uid, rangeList)

	if nil != err {
		return nil, err
	}

	list := make([]ResUserImg, 0)
	for rows.Next() {
		var filename, eTag string
		var cTime int64

		err = rows.Scan(&filename, &eTag, &cTime)
		if nil != err {
			return nil, err
		}
		list = append(list, ResUserImg{
			Filename: filename,
			ETag:     eTag,
			CTime:    cTime,
		})
	}
	return list, nil
}

func (dbi *DBI) Insert(uid, eTag, hash, filename string, siz, cTime int64) error {
	extName := path.Ext(filename)
	_, err := dbi.StmtMap["inst"].Exec(eTag, hash, extName, siz)
	if nil == err {
		_, err = dbi.StmtMap["inst_usr"].Exec(uid, filename, eTag, cTime)
	}
	return err
}

func (dbi *DBI) Update(uid, eTag, hash, filename string, siz int64) error {
	extName := path.Ext(filename)
	_, err := dbi.StmtMap["inst"].Exec(eTag, hash, extName, siz)
	if nil == err {
		_, err = dbi.StmtMap["updt_usr"].Exec(uid, filename, eTag)
	}
	return err
}

func (dbi *DBI) Del(uid, filename string) error {
	_, err := dbi.StmtMap["delt"].Exec(uid, filename, time.Now().Unix())
	return err
}

func (dbi *DBI) Drop(uid, filename string) error {
	_, err := dbi.StmtMap["drop"].Exec(uid, filename)
	return err
}
