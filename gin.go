package main

import (
	"github.com/gin-gonic/gin"
	//"github.com/gin-contrib/static"
	//"net/http"
)

func main() {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	//r.Use(gin.Recovery())

	//r.LoadHTMLGlob("templates/*")

	//r.Use(static.Serve("/", static.LocalFile("./public", true)))
	r.GET("/path", myHandler)
	//r.GET("/test/:number", testHandler)


	r.Run(":80")
}

func myHandler(ctx *gin.Context) {
	ctx.Data(200, "text/html; charset=utf-8", []byte("<h1>Hello World</h1>") )
}


//func testHandler(ctx *gin.Context) {
//	number := ctx.Param("number")
//
//	content := make(map[string]string)
//
//	content["host"] = ctx.Request.Host
//	for i, param := range ctx.Params {
//		content["param" + string(i)] = param.Value
//	}
//	content["ip"] = ctx.ClientIP()
//
//	ctx.HTML(http.StatusOK, "no-sidebar.tmpl", gin.H{
//		"title": "Website Title",
//		"number": number,
//		"content": content,
//	})
//}

