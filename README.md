# kronos-demo

四个 kopy 模板按**全默认变量**渲染出来的一套可跑服务,用来看清「模板生成出来的东西长什么样、怎么串起来」

这个目录不是 git 仓库,四个子项目也都没有 `git init` —— 它是工作区,随时可以 `bash reset.sh` 重建
模板仓库本身一行没改

## 它是怎么来的

```bash
kopy new ./kronos-access-control-template -o kronos-demo/kronos-access-control-service -y --hooks never
kopy new ./kronos-key-service-template    -o kronos-demo/kronos-key-service          -y --hooks never
kopy new ./kronos-service-template        -o kronos-demo/kronos-my-service           -y --hooks never
kopy new ./kronos-gateway-template        -o kronos-demo/kronos-gateway-service      -y --hooks never
```

每个产物再跑一次它自带的 `bash scripts/init.sh`:解析协议中心依赖 → tidy → fmt → build → test
协议中心取的是已发布的 `github.com/kamalyes/kronos-share-proto-template@build`,解析到
`v0.0.0-20261030105250-87b817d55412`,不需要本地有协议仓库

## 四个服务与端口

| 服务 | 来自 | HTTP | gRPC | 启动需要 |
|---|---|---:|---:|---|
| `kronos-access-control-service` | kronos-access-control-template | 8390 | 9390 | 无 |
| `kronos-gateway-service` | kronos-gateway-template | 8490 | 9490 | 无 |
| `kronos-key-service` | kronos-key-service-template | 8290 | 9290 | Redis |
| `kronos-my-service` | kronos-service-template | 8190 | 9190 | Redis + MySQL |

端口不冲突是模板默认值本来就排开的,不是我在这里改的

网关侧不需要配任何路由:`resources/kronos-gateway-service-dev.yaml` 的 `grpc.clients` 已经把上面三个
下游都声明好了(9390 / 9190 / 9290),网关启动时靠 gRPC server reflection 读对方的 service 与
`google.api.http` 注解,在**网关进程里**建出 HTTP 路由 加一个下游只改这段配置,不写 Go 代码

鉴权服务是特殊的一个:它是「代理的前置条件」而不是「被代理的对象」,不可用时中间件返回 500 而不是放行

## 现在能跑到哪一步

本机没有 Redis / MySQL / Docker,所以实测结果是:

- **access-control + gateway 这一对完全跑通**,`bash smoke.sh` 可复现
  网关连上 3 个下游、对 access-control 的 reflection 成功,发现
  `kronos.api.access_control.AuthService`;另两个未启动的下游如实报 `Unavailable`,网关不硬失败
- `kronos-key-service` 缺 Redis **刻意不降级**:`Redis 未配置:本服务的临时密钥与 ID 序列都依赖
  Redis,无法降级启动`
- `kronos-my-service` 缺库同样硬失败:`database not initialized`

要把后两个也跑起来,补基础设施即可(配置里是 `localhost:6379` 与 `localhost:3306` root/`mysql_password`,
库名 `my_service`),代码侧不用动

## 一处必须手工补的东西

`kronos-access-control-template` 与 `kronos-key-service-template` 的配置里写着
`i18n.enabled: true` + `messages-path: ./locales`,但这两个模板**不带 `locales/` 目录**
渲染出来的服务启动时直接挂在:

```
failed to init i18n manager: 语言加载失败: 语言 en: 语言文件未找到: 路径: locales/en.json
```

`reset.sh` 会在渲染后补上最小的 `locales/{en,zh}.json`(只放 service-template 约定里本来就有的
`welcome` / `success` / `hello` / `goodbye` 四个键,没编造错误码) 两个模板的 CI 只做
「渲染 → 编译 → vet」、从不启动进程,所以这条漏了 —— 要根治得改模板,这个目录只做本地绕过

另外一条观察,在 go-rpc-gateway 而不是模板里:配置的 `health.port`(8391 / 8491 / …)并没有被监听,
`/health` 实际挂在 HTTP 端口上 探活请打 HTTP 端口

## 命令

```bash
bash reset.sh    # 删掉四个子项目重新渲染 + init.sh + 补 locales(不碰本目录下的其它东西)
bash smoke.sh    # 起 access-control + gateway,验反射发现,结束后自动收掉进程
```

`smoke.sh` 先 `go build` 再执行产出的二进制,而不是 `go run .` —— `go run` 会 fork 一个子进程去跑
编译结果,脚本拿到的是 `go run` 自己的 pid,收尾时杀它并不会杀掉真正在监听端口的那个子进程
现象很有欺骗性:下一次运行探活 1 秒就通过、`uptime` 却是几百秒,读到的是上一轮的残留进程,
而本轮因端口被占根本没起来 所以脚本开头还加了端口预检,被占着就直接停而不是报一个假的成功

真碰上残留进程时这样收:

```bash
lsof -nP -iTCP:8390 -iTCP:9390 -iTCP:8490 -iTCP:9490 -sTCP:LISTEN
kill <那几个 pid>
```

单独起一个服务:

```bash
cd kronos-gateway-service && go run .   # APP_ENV 未设置时读 resources/<项目名>-dev.yaml
```

启动日志里会有一行 `获取环境变量失败 ... APP_ENV 未设置`,那是 INFO/ERROR 级别的提示而不是故障,
不设 APP_ENV 就是读 dev 配置
