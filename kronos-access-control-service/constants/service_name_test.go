/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 00:00:00
 * @LastEditors: kamalyes example@qq.com
 * @LastEditTime: 2026-11-01 00:00:00
 * @FilePath: \kronos-access-control-service\constants\service_name_test.go
 * @Description: 免鉴权白名单行为
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package constants

import "testing"

// 白名单是绕过鉴权的后门，所以它的边界行为必须被测到
//
// 这组测试不是为了覆盖率。白名单匹配写错的后果分两种，都不会报错：
//   匹配过宽 → 业务接口免鉴权，而一切"正常"
//   匹配过窄 → 健康检查被拦，k8s 探针失败，Pod 反复重启
// 前者是安全事故，后者至少看得见。所以重点测前者。
//
// [EN] The allow list is an authorization bypass, so its edge behaviour must be tested. Not for
// coverage: a matching mistake fails silently in two ways. Too wide leaves business endpoints
// credential-free with everything apparently fine; too narrow blocks the health check and the
// pod restarts in a loop. The first is a security incident and the second is at least visible,
// so the first gets the attention.
func TestIsPublicPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
		why  string
	}{
		{"/health", true, "健康检查必须免鉴权，否则探针拿不到 token 会让 Pod 反复重启"},
		{"/healthz", true, "k8s 常用的另一种写法"},
		{"/swagger/index.html", true, "文档站是一组路径，前缀匹配要覆盖子路径"},
		{"/metrics", true, "指标抓取方不带业务 token"},

		{"/v1/products", false, "业务接口必须鉴权"},
		{"/v1/temp-keys/decrypt", false, "密钥接口尤其必须鉴权"},
		{"", false, "空路径不该被当成免鉴权"},
		{"/", false, "根路径不在白名单里；若被放行等于整站免鉴权"},
		{"/healthcheck-admin", true, "已知的前缀匹配副作用：/health 前缀会命中它"},
	}
	for _, c := range cases {
		if got := IsPublicPath(c.path); got != c.want {
			t.Errorf("IsPublicPath(%q) = %v，期望 %v —— %s", c.path, got, c.want, c.why)
		}
	}
}

// TestIsPublicPathRejectsEmptyPrefix 列表里混进空串时不能放行一切
//
// 这是白名单最危险的失效模式：一个空串让 strings.HasPrefix 对任何路径都返回 true，
// 于是整个服务免鉴权，而所有接口都"正常工作"，没有任何报错。
// IsPublicPath 里显式跳过空前缀，这个测试守住那一行不被"优化"掉。
//
// [EN] The most dangerous failure mode: one empty string makes HasPrefix true for every path,
// leaving the whole service credential-free while every endpoint works and nothing is logged.
// IsPublicPath skips empty prefixes explicitly and this test keeps that line from being
// optimized away.
func TestIsPublicPathRejectsEmptyPrefix(t *testing.T) {
	original := publicPaths
	t.Cleanup(func() { publicPaths = original })

	publicPaths = []string{""}
	if IsPublicPath("/v1/products") {
		t.Fatal("白名单里的空串让业务接口免鉴权了 —— 整个服务等于没有鉴权")
	}
}
