package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/watsonserve/galleried/action"
	"github.com/watsonserve/galleried/dao"
	"github.com/watsonserve/galleried/helper"
	"github.com/watsonserve/galleried/services"
	"github.com/watsonserve/goengine"
	"github.com/watsonserve/goutils"
	"github.com/watsonserve/pass_sdk"
)

func main() {
	optionsInfo := []goutils.Option{
		{
			Name:      "help",
			Opt:       'h',
			Option:    "help",
			HasParams: false,
			Desc:      "display help info",
		},
		{
			Name:      "conf",
			Opt:       'c',
			Option:    "conf",
			HasParams: true,
			Desc:      "configure filename",
		},
	}
	helpInfo := goutils.GenHelp(optionsInfo, "")
	opts, addr := goutils.GetOptions(optionsInfo)
	confFile, hasConf := opts["conf"]
	if _, hasHelp := opts["help"]; hasHelp {
		fmt.Println(helpInfo)
		return
	}
	if !hasConf {
		confFile = "/etc/galleried/galleried.conf"
	}
	conf, err := goutils.GetConf(confFile)
	if nil != err {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}

	dbConn := goengine.ConnPg(&goengine.DbConf{
		User:   conf.GetVal("db_user"),
		Passwd: conf.GetVal("db_passwd"),
		Host:   conf.GetVal("db_host"),
		Name:   conf.GetVal("db_name"),
		Port:   conf.GetVal("db_port"),
	})
	rootDir := conf.GetVal("root")
	fmt.Printf("root: %s\n", rootDir)

	sessMgr := helper.InitSessMgr(
		goengine.NewRedisStore(conf.GetVal("redis_address"), conf.GetVal("redis_password"), 1),
		conf.GetVal("sess_name"),
		conf.GetVal("cookie_prefix"),
		conf.GetVal("session_prefix"),
		conf.GetVal("domain"),
	)

	srvInfo := &pass_sdk.SrvInfo{
		AuthPathname: conf.GetVal("auth_pathname"),
		AppId: conf.GetVal("app_id"),
		Scheme: conf.GetVal("scheme"),
		Host: conf.GetVal("host"),
		Secret: conf.GetVal("secret"),
	}
	pass_sdk.BizAO
	pass_sdk.BindAuthMgr(srvInfo)

	// var dbi *dao.DBI = nil
	dbi := dao.NewDAO(dbConn)

	prefix := conf.GetVal("path_prefix")
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	listSrv := services.NewListService(dbi, rootDir)
	fileSrv := services.NewFileService(dbi, rootDir)

	p := action.NewPictureAction(len(prefix)-1, sessMgr, listSrv, fileSrv)
	u := action.NewUserAction([]string{conf.GetVal("app_id"), conf.GetVal("app_secret")}, sessMgr)

	// sess := map[string]string{
	// 	"d7c1fb907d6d47bda1f1b0f45baeb878": "bacac18aae9e4cd6aad02bf9ca389664",
	// }
	router := goengine.InitHttpRoute()
	router.Set("/login", u.ServeHTTP)
	router.StartWith(prefix, p.ServeHTTP)

	engine := goengine.New(router)
	listen := conf.GetVal("listen")
	if 0 < len(addr) {
		listen = addr[0]
	}
	if err = http.ListenAndServe(listen, engine); nil != err {
		panic(err)
	}
}
