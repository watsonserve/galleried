package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"path"

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

func sqlList(equit bool, orderBy string, paged bool) string {
	strEquit := "<>"
	if equit {
		strEquit = "="
	}
	strPaged := ""
	if paged {
		strPaged = " OFFSET $2 LIMIT $3"
	}
	return fmt.Sprintf("SELECT filename, etag, ctime FROM res_user_img WHERE uid=$1 AND rtime%s0 ORDER BY %s DESC%s", strEquit, orderBy, strPaged)
}

func NewDAO(dbConn *sql.DB) *DBI {
	dao := goengine.InitDAO(dbConn)
	dao.Prepare("real_name", "SELECT raw FROM res_thumb WHERE hash=$1")

	// GET
	dao.Prepare("info", "SELECT etag FROM res_user_img WHERE uid=$1 AND filename=$2 AND rtime=0")
	dao.Prepare("recycle_info", "SELECT etag FROM res_user_img WHERE uid=$1 AND filename=$2 AND rtime>0")

	// LIST
	dao.Prepare("list", sqlList(true, "ctime", false))
	dao.Prepare("list_limit", sqlList(true, "ctime", true))
	dao.Prepare("recycle_list", sqlList(false, "rtime", false))

	// PUT
	dao.Prepare("inst", "INSERT INTO res_thumb (etag, hash, ext, size) VALUES ($1, $2, $3, $4)")
	dao.Prepare("inst_usr", "INSERT INTO res_user_img (uid, filename, etag, ctime) VALUES ($1, $2, $3, $4)")
	dao.Prepare("updt_usr", "UPDATE res_user_img SET etag=$3 WHERE uid=$1 AND filename=$2 AND rtime=0")
	dao.Prepare("move_to_recycle", `
		UPDATE res_user_img SET rtime=EXTRACT(EPOCH FROM clock_timestamp())::int WHERE rtime=0 AND uid=$1 AND filename=$2 AND etag=$3
	`)
	dao.Prepare("restore_recycle", `
		UPDATE res_user_img SET rtime=0 WHERE rtime>0 AND uid=$1 AND filename=$2 AND etag=$3
	`)

	// DELETE
	dao.Prepare("delete", `
		WITH deleted AS (
			DELETE FROM res_user_img
			WHERE uid=$1 AND filename=$2 AND rtime>0 AND etag=$3
		),
		thumb_deleted AS (
			DELETE FROM res_thumb
			WHERE etag=$3 AND NOT EXISTS (
				SELECT 1 FROM res_user_img WHERE etag=$3
			)
		)
		SELECT 1 FROM res_thumb WHERE etag=$3
	`)

	return &DBI{DAO: *dao}
}

func (dbi *DBI) Info(uid, fileName string, isRecycled bool) (string, error) {
	cmd := "info"
	if isRecycled {
		cmd = "recycle_info"
	}

	eTag := ""
	err := dbi.StmtMap[cmd].QueryRow(uid, fileName).Scan(&eTag)
	return eTag, err
}

func (dbi *DBI) selectList(uid string, isRecycle bool, rangeList []helper.Segment) (rows *sql.Rows, err error) {
	if isRecycle {
		return dbi.StmtMap["recycle_list"].Query(uid)
	}

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
		return dbi.StmtMap["list"].Query(uid)
	}
	return dbi.StmtMap["list_limit"].Query(uid, offset, length)
}

func (dbi *DBI) List(uid string, isRecycle bool, rangeList []helper.Segment) ([]ResUserImg, error) {
	rows, err := dbi.selectList(uid, isRecycle, rangeList)

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

func (dbi *DBI) DeleteRecycle(uid, filename, eTag string) (bool, error) {
	var shouldNotCleanup bool
	err := dbi.StmtMap["delete"].QueryRow(uid, filename, eTag).Scan(&shouldNotCleanup)
	return !shouldNotCleanup, err
}

func (dbi *DBI) SetRecycle(uid, filename, etag string, rec bool) error {
	cmd := "move_to_recycle"
	if rec {
		cmd = "restore_recycle"
	}
	_, err := dbi.StmtMap[cmd].Exec(uid, filename, etag)
	return err
}
