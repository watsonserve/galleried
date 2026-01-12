package main

import (
	"fmt"
	"net/http"
	"os"

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
		confFile = "/etc/galleried.conf"
	}
	conf, err := goutils.GetConf(confFile)
	if nil != err {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}

	dbConn := goengine.ConnPg(&goengine.DbConf{
		User:   conf["db_user"][0],
		Passwd: conf["db_passwd"][0],
		Host:   conf["db_host"][0],
		Name:   conf["db_name"][0],
		Port:   conf["db_port"][0],
	})
	rootDir := conf["root"][0]
	fmt.Printf("root: %s\n", rootDir)

	sessMgr := goengine.InitSessionManager(
		goengine.NewRedisStore(conf["redis_address"][0], conf["redis_password"][0], 1),
		conf["sess_name"][0],
		conf["cookie_prefix"][0],
		conf["session_prefix"][0],
		conf["domain"][0],
	)

	dbi := dao.NewDAO(dbConn)

	listSrv := services.NewListService(dbi, rootDir)
	fileSrv := services.NewFileService(dbi, rootDir)

	p := action.NewPictureAction(sessMgr, listSrv, fileSrv)

	router := goengine.InitHttpRoute()
	router.StartWith(conf["path_prefix"][0]+"/", p.ServeHTTP)
	engine := goengine.New(router, nil)

	listen := conf["listen"][0]
	listen_1 := addr[0]
	if "" != listen_1 {
		listen = listen_1
	}
	http.ListenAndServe(listen, engine)
}
