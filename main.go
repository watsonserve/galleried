package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/watsonserve/galleried/action"
	"github.com/watsonserve/galleried/dao"
	"github.com/watsonserve/galleried/services"
	"github.com/watsonserve/goengine"
	"github.com/watsonserve/goutils"
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
	rootDir := conf["root"][0]
	fmt.Printf("root: %s\n", rootDir)

	sessMgr := goengine.InitSessionManager(
		goengine.NewRedisStore(conf.GetVal("redis_address"), conf.GetVal("redis_password"), 1),
		conf.GetVal("sess_name"),
		conf.GetVal("cookie_prefix"),
		conf.GetVal("session_prefix"),
		conf.GetVal("domain"),
	)

	dbi := dao.NewDAO(dbConn)

	prefix := conf.GetVal("path_prefix")
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	listSrv := services.NewListService(dbi, rootDir)
	fileSrv := services.NewFileService(dbi, rootDir)

	p := action.NewPictureAction(len(prefix)-1, sessMgr, listSrv, fileSrv)
	u := action.NewUserAction([]string{conf.GetVal("app_id"), conf.GetVal("app_secret")}, sessMgr)

	router := goengine.InitHttpRoute()
	router.Set("/login", u.ServeHTTP)
	router.StartWith(prefix, p.ServeHTTP)

	engine := goengine.New(router, nil)

	listen := conf.GetVal("listen")
	if 0 != len(addr) {
		listen = addr[0]
	}

	if err = http.ListenAndServe(listen, engine); nil != err {
		panic(err)
	}
}
