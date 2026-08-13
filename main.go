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

type Conf struct {
	Db      *goengine.DbConf
	Redis   *helper.RedisConf
	Pass    *pass_sdk.SrvInfo
	Prefix  string
	RootDir string
	Listen  string
}

func getConfig() *Conf {
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
		return nil
	}
	if !hasConf {
		confFile = "/etc/galleried/galleried.conf"
	}
	conf, err := goutils.GetConf(confFile)
	if nil != err {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil
	}

	prefix := conf.GetVal("path_prefix")
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	listen := conf.GetVal("listen")
	if 0 < len(addr) {
		listen = addr[0]
	}

	return &Conf{
		Db: &goengine.DbConf{
			User:   conf.GetVal("db_user"),
			Passwd: conf.GetVal("db_passwd"),
			Host:   conf.GetVal("db_host"),
			Name:   conf.GetVal("db_name"),
			Port:   conf.GetVal("db_port"),
		},
		Redis: &helper.RedisConf{
			RedisAddress:  conf.GetVal("redis_address"),
			RedisPassword: conf.GetVal("redis_password"),
			SessName:      conf.GetVal("sess_name"),
			CookiePrefix:  conf.GetVal("cookie_prefix"),
			SessionPrefix: conf.GetVal("session_prefix"),
			Domain:        conf.GetVal("domain"),
		},
		Pass: &pass_sdk.SrvInfo{
			CliAuthPathname: conf.GetVal("auth_pathname"),
			AppId:           conf.GetVal("app_id"),
			Scheme:          conf.GetVal("scheme"),
			Host:            conf.GetVal("host"),
			Secret:          conf.GetVal("secret"),
		},
		Prefix:  prefix,
		RootDir: conf.GetVal("root"),
		Listen:  listen,
	}
}

func start(conf *Conf, p *action.PictureAction, sessMgr *helper.SessMgr) {
	u := action.NewUserAction(sessMgr)
	router := goengine.InitHttpRoute()
	router.StartWith(conf.Prefix, p.ServeHTTP)
	err := pass_sdk.BindAuthMgr(conf.Pass, u, router)
	if nil != err {
		panic(err)
	}

	engine := goengine.New(router)
	if err = http.ListenAndServe(conf.Listen, engine); nil != err {
		panic(err)
	}
}

func main() {
	conf := getConfig()

	// var dbi *dao.DBI = nil
	dbi := dao.NewDAO(goengine.ConnPg(conf.Db))

	sessMgr := helper.InitSessMgr(conf.Redis)

	rootDir := conf.RootDir

	listSrv := services.NewListService(dbi, rootDir)
	fileSrv := services.NewFileService(dbi, rootDir)

	p := action.NewPictureAction(len(conf.Prefix)-1, sessMgr, listSrv, fileSrv)

	fmt.Printf("root: %s\n", rootDir)
	start(conf, p, sessMgr)
}
