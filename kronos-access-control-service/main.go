/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\main.go
 * @Description: 服务入口
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package main

import (
	"log"

	"github.com/kamalyes/kronos-access-control-service/bootstrap"
)

func main() {
	if err := bootstrap.NewAccessControlServiceGateway().Run(); err != nil {
		log.Fatalf("Gateway startup failed: %v", err)
	}
}
