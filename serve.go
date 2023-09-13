package main

import (
	"fmt"

	"github.com/kataras/iris/v12"
)

func Serve(root string, port uint) error {
	app := iris.New()
	app.HandleDir("/", iris.Dir(root), iris.DirOptions{
		IndexName:  "/index.html",
		Compress:   false,
		ShowList:   true,
		ShowHidden: true,
		Cache: iris.DirCacheOptions{
			Enable: false,
		},
	})

	return app.Listen(fmt.Sprintf(":%d", port))
}
