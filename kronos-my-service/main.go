/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\main.go
 * @Description: MyService 服务启动器
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package main

import (
	goconfig "github.com/kamalyes/go-config"
	"github.com/kamalyes/kronos-my-service/bootstrap"
	"log"
	"os"
	"time"
)

func init() {
	goconfig.RegisterEnvPrefixes("laos", "laos")
	goconfig.RegisterEnvPrefixes("pakistan", "pakistan")
	goconfig.RegisterEnvPrefixes("myanmar", "myanmar")
}

func main() {
	time.Local = time.UTC

	server := bootstrap.NewMyServiceGateway()

	if err := server.Run(); err != nil {
		log.Fatalf("Gateway startup failed: %v", err)
		os.Exit(1)
	}
}
