# go-ohos-kit

**Go 做内核，ArkUI 做界面。** 一个能跑在 HarmonyOS NEXT 真机上的最小底座：
Go 负责领域逻辑、状态、网络、SQLite、加解密、后台任务；界面用原生 ArkTS/ArkUI
声明式写，想用什么组件、什么 Kit 都随你。

> **对照组** · 实验组：[isfk/rust-ohos-kit](https://github.com/isfk/rust-ohos-kit)
> 同一个 ArkTS 外壳、同样的八块面板，内核换成 Rust：**0 个 runtime 补丁**，产物 2.7 MB（这里 11 MB）。
> 那边唯一更贵的是"纯度"—— 它得把 SQLite 的 C 编进产物，这里用纯 Go 驱动、内核零 C 依赖；
> 代价是同样的 20k 行 seed 59.3 ms vs 12.3 ms。Go 侧有 **3 处 runtime 行为修补 + 1 个符号声明文件**（共 4 个 overlay 文件）；逐条对照、桥读数、体积表见[那边的 CONTEXT.md](https://github.com/isfk/rust-ohos-kit/blob/main/CONTEXT.md)。
>
> 语言、生态、踩过的坑、什么时候选哪个：[**Go vs Rust，在 HarmonyOS 上**](docs/go-vs-rust.md)。

```text
ArkTS / ArkUI  ──import 'libgocore.so'──▶  NAPI 桥  ──▶  Go 内核
（原生 UI）      ◀── 事件（threadsafe function）──      （goroutine 随便开）
```

它不是 UI 框架——**没有**自己的布局引擎、组件树、渲染器。ArkUI 是唯一的 UI。

界面用**状态管理 V2**（`@ComponentV2` + `@Local` / `@Monitor`，跨组件共享用
`@ObservedV2` + `@Trace` + `AppStorageV2`）。V1 的 `@State` / `@StorageLink` 一个没留。

| | 数字 |
|---|---|
| 桥 | 798 行（`go/main_linux.go` 347 + `go/napi_linux.c` 451），21 个 `//export` |
| Go 内核 | 845 行（`core`）+ 177 行（`store`），704 行测试（纯 Go，Mac 上 `go test`） |
| 产物 | `libgocore.so` 11 MB（SQLite 驱动占 6.8 MB），依赖只有 `libace_napi` / `libhilog` / `libc` |

## 为什么不用「Go 全包 UI」

| | Go 全包 UI | **本方案：Go 内核 + ArkUI** |
|---|---|---|
| UI 写在哪 | Go | ArkTS（鸿蒙原生） |
| 跨端共享 | 逻辑 + UI | 只共享逻辑 |
| 鸿蒙 Kit（Push/Map/Camera/支付…） | 每个都要自己架桥 | 原生直接可用 |
| 要维护的东西 | 上万行组件宿主 | ~470 行桥 + 3 个 Go runtime 补丁 |
| 谁来写界面 | 极小众 | ArkTS 开发者满地都是 |

**唯一的取舍**：UI 不跨端。如果 App 要在 iOS/Android/鸿蒙共享同一套界面，那要的是
「用 Go 写 UI」那类方案；如果只是「想在鸿蒙上写 Go」，这就是终点。

## 快速开始

需要：Go 1.27（`VERSION` 对得上）、DevEco SDK（`$DEVECO_SDK_HOME`，默认
`~/command-line-tools/sdk`）、一台开了 USB 调试的鸿蒙机。

```sh
cd app && ohpm install --all && cd ..   # 必须：oh_modules 不入库
platform/ohos/run.sh                    # 编 Go 内核 → 编 HAP → 装机 → 启动 → 看日志
hdc shell hilog -T gokit                # 内核的日志（fmt.Println / panic 都在这）
```

第一次装不上只有一个原因：**没签名**。见下一节。

### 签名（每个包名一次）

鸿蒙的 App 必须签名才能装，而**签名档里写着包名**，所以换包名就要重新签一次：

1. DevEco 打开工程目录（`app/`）→ **Project Structure → Signing Configs**
2. 勾 **Automatically generate signature**（要登录华为 ID），它会写回 `build-profile.json5` 的 `signingConfigs`
3. 之后 `platform/ohos/run.sh` 就能装

`run.sh` 发现只有 unsigned HAP 时会直接这么提示。包名在 `AppScope/app.json5` 的 `bundleName`。

**`app/build-profile.json5` 不在库里**（`.gitignore` 里），因为自动签名会往里写**这台机器的
证书绝对路径 + 两个密码**，而签名档还绑着包名 —— 对别人、对换包名都是废的。入库的是模板
`app/build-profile.example.json5`（`signingConfigs: []`，product 不引用签名）：

```sh
cp app/build-profile.example.json5 app/build-profile.json5   # 或者直接跑 run.sh，它会替你拷
```

`run.sh` 和 `new.sh` 都在真文件缺失时自动从模板生成，所以 clone 下来不会卡在构建失败上——
只是那份是**未签名**的，装之前要去 DevEco 点一次自动签名。

## 常用命令

| 想做什么 | 命令 | 要设备？ |
|---|---|---|
| 全套自检（test + vet + 补丁 + 交叉编译） | `scripts/check.sh` | 不要 |
| 没有 SDK 的机器上自检 | `scripts/check.sh --no-build` | 不要 |
| 只测 Go 逻辑 | `cd go && go test ./core/` | 不要 |
| 只编 `.so` | `platform/ohos/build.sh go -project app` | 不要 |
| 编 + 装 + 跑 + 抓日志 | `platform/ohos/run.sh` | **要** |
| 桥的读数（不点按钮，N 轮中位数） | `platform/ohos/bench.sh 5` | **要** |
| 新起一个工程 | `platform/ohos/new.sh myapp [--bundle com.x.y]` | 不要 |
| Go 升级后先跑这个 | `scripts/check-runtime.sh` | 不要 |
| 检查补丁还配不配这个 Go | `cd tools/patch && go run . check` | 不要 || 拿补丁的 overlay 给别的工程用 | `cd tools/patch && go run . overlay -o ov.json` | 不要 || 验一个 `.so` 能不能被 musl 加载 | `cd tools/patch && go run . verify libgocore.so` | 不要 |

## 目录

```text
go/                         Go 内核（编成 libgocore.so）
  core/                     纯 Go：状态、事件、持久化、文件鉴定、水平仪、像素分析
    core.go                 待办、事件、文件鉴定
    image.go                像素数学：亮度、直方图、主色、8×8 亮度图
    core_test.go            310 行测试，Mac 上直接跑
  main_linux.go             //go:build linux && cgo：13 个 //export，只做搬运
  napi_linux.c              NAPI 模块：JS ↔ Go（唯一需要 Node-API 知识的地方）
  cgo_linux.go              链接的系统库（libace_napi、libhilog）
  tls_linux.c               Go 的每线程 g 槽（替换 initial-exec TLS）
  log_linux.c               fd 1/2 → hilog（panic/崩溃日志靠它）
  fault_linux.c             SIGSEGV 等信号打地址（开发用）

platform/ohos/
  build.sh / run.sh / new.sh  交叉编译落地 / 装+启+抓日志 / 脚手架
  runtime/                  被 go build -overlay 替换/注入的 Go runtime 文件
    tls_arm64.s             g 走动态 TLS
    rt0_linux_arm64.s       库模式给空 argv/envp
    os_linux.go             page size 兜底
    gokit_ohos.go           注入 runtime 包的两个符号声明
    VERSION                 补丁取自哪个 Go 版本

scripts/
  check.sh                  一套命令跑完：test / vet / 补丁 / 交叉编译 / TLS 断言
  check-runtime.sh          检查 3 处 runtime 修补 + 1 个符号声明（Go 升级后跑）＝ tools/patch check 的壳

tools/patch/                Go 写的小工具，检查这 4 个 overlay 文件（自带 module + 测试）
  main.go                  check / overlay / verify 三个子命令
  patches.go               补丁表：替换谁、为什么、标记、要盯的上游锚点

app/                        HarmonyOS 工程（DevEco 结构）
  build-profile.example.json5  tracked 模板（不签名）；build-profile.json5 是本机的、被忽略
  entry/src/main/ets/
    core/                   与界面无关的底座
      GoCore.ts             类型化的内核封装（唯一知道传输格式的地方）
      events.ets            Go 事件的唯一入口：kind → 订阅方
    state/                  @ObservedV2 + @Trace，经 AppStorageV2 共享
      AppLife.ets           前台/后台（Ability 写、组件读）
    features/               一个能力一个目录，各自持有自己的状态与订阅
      todos/                TodoComposer（输入）+ TodoList（行 + 计数）
      photo/                PhotoCard：拍照/选图 → 平台解码 → Go 量像素
      camera/               CameraCard：权限 + surface + 生命周期
      location/             LocationCard：定位订阅 + 长时任务
      level/                LevelRow：50Hz 加速度计 → Go 平滑 → 水平仪
      heartbeat/            Heartbeat：内核 goroutine 每秒推一行
      bench/                MeasureCard / BridgeCard / DbCard 三组实测
    pages/Index.ets         界面：纯装配（布局 + 避让区 + 待办）
  entry/src/main/cpp/types/libgocore/index.d.ts   内核的 TS 类型
  entry/src/main/resources/{base,dark}/element/color.json   明/暗两套颜色 token
```

## 桥怎么用

### ArkTS → Go（同步，主线程）

```ts
import core from 'libgocore.so';

const todos = core.add('buy milk');   // 返回 JSON 字符串
```

**必须快**：这是主线程上的同步调用。数据库查询、网络请求这类，要么异步化成
「Go 做完推事件回来」，要么自己开 worker。超过一帧（16ms）就会掉帧。

### Go → ArkTS（异步，任意 goroutine）

```go
// Go 内核里
store.Listen(func(json string) { C.gokit_emit(C.CString(json)) })

go func() {
    for range ticker.C {
        store.push(Event{Kind: "text", Text: "up " + elapsed()})  // 随便哪个 goroutine
    }
}()
```

```ts
// ArkTS
gocore.listen((e: Event) => { this.heartbeat = e.text ?? '' });
```

C 侧用 `napi_create_threadsafe_function` 把事件搬到主线程——**这是唯一安全的方式**，
千万别在 goroutine 里直接碰 JS 引擎。

### 内存与所有权

- Go 返回给 JS 的 `*C.char`：桥把它变成 JS 字符串后 `free()` 掉，ArkTS 不用管
  （返回的是 `C.CString`，malloc 出来的，C 能直接释放）。
- Go 推出去的 JSON：所有权交给桥，桥在 TSFN 回调里释放。
- 大块数据（图片、几 MB 的列表）别用 JSON，用 `napi_create_arraybuffer` 传
  `[]byte`——省一次拷贝和一次解析。

## 接平台能力：五个由浅到深的例子

### 一、一次性：系统相册 → Go 做真活

最典型的用法。**ArkTS 去申请/调起平台能力，Go 只处理数据。**

```ts
// ArkTS：系统相册（免权限），拿到 URI → 打开 → 把 fd 交给 Go
const picker = new photoAccessHelper.PhotoViewPicker();
const chosen = await picker.select({ MIMEType: IMAGE_TYPE, maxSelectNumber: 1 });
const file = fs.openSync(chosen.photoUris[0], fs.OpenMode.READ_ONLY);
const result = gocore.inspect(file.fd, name);   // Go 读文件、判格式、算 SHA-256
fs.closeSync(file);                             // 描述符只是借给 Go
```

```go
// Go：一个 io.Reader 就够了，于是同一份代码在测试里喂 bytes.Reader
info, err := core.Inspect(f, name)   // → {Bytes, Kind: "image/jpeg", SHA256}
```

实测（真机）：点「Check a photo」→ 系统相册 → 选一张 → `image/jpeg · 0.4 MB · 454d56e2…`。
全程 4 行 ArkTS + 20 行 Go，**没用任何权限**。

### 二、高频：加速度计 50Hz

一个会每秒叫 50 次的平台回调，是这套桥另一个极端。实测（真机）：

```text
●   pitch -1.9° · roll +1.0° · 50 samples/s
```

- ArkTS 侧：`sensor.on(ACCELEROMETER, cb, { interval: 20000000 })`（20ms），回调里只有一句
  `gocore.accel(x, y, z)`；
- 桥：三个 double 过去，**不分配、不碰 JS**；
- Go 侧：低通滤波 + `atan2` 算 pitch/roll，并且**自己决定多久通知 UI 一次**（33ms 一帧，
  不该每个样本都去改界面）。

实测 50 samples/s 稳定，同时页面其它部分（1Hz 心跳、列表）不受影响。

> 坑：加速度计要 `ohos.permission.ACCELEROMETER`（`system_grant`，装包生效、不弹窗）。
> 忘了它会得到 `Error: Permission denied`——每个传感器都有自己的一条。

### 三、长命：相机预览

选图是一次性的，相机是「一直开着」的那种，它多了三样东西：**运行时权限**、
**一块交给系统服务的 surface**、**一个必须在正确时机拆掉的会话**。

```ts
// 权限：user_grant，第一次用到才弹
const result = await abilityAccessCtrl.createAtManager()
  .requestPermissionsFromUser(ctx, ['ohos.permission.CAMERA']);

// 预览：XComponent 的 surface 给相机服务，帧不经过 ArkTS
const manager = camera.getCameraManager(ctx);
const input   = manager.createCameraInput(back);
const preview = manager.createPreviewOutput(profile, this.controller.getXComponentSurfaceId());
const session = manager.createSession<camera.PhotoSession>(camera.SceneMode.NORMAL_PHOTO);
session.beginConfig(); session.addInput(input); session.addOutput(preview);
session.commitConfig(); session.start();
```

实测（真机）：`1280×720 preview` 出画面；按 Home 后
`Ability onBackground 15:43:06.723` → `CloseCamera 15:43:06.965`（**242ms** 后相机全通道关闭）。

坑都在 `features/camera/CameraCard.ets` 的注释里，摘几条：

- **`user_grant` 权限必须写 `reason` + `usedScene`**，否则 hvigor 报
  `00303218 Configuration Error: The reason and usedScene attributes are mandatory for user_grant permissions`。
  `system_grant`（加速度计）不需要。`reason` 指向 `$string:xxx`，`string.json` 里得真有这一条。
- **`session.start()` 在这个版本不是 Promise**。写成 `session.start().then(...)` 不报错——
  它静默抛在 `.then` 上，于是**相机真的开了、UI 永远显示 "camera off"**。同步设状态，出错再靠 catch 改。
- **高度为 0 的 `XComponent` 没有 surface**，状态机会自锁在 `waiting for the surface…`。
- **别在 Stack 里随状态增删 XComponent 的兄弟节点**：`if (!running) Text(...)` 会在
  `running` 翻 true 时把兄弟删掉 → **XComponent 被重建、换了一个新 surface**，而会话还画在
  老的那块上 → 一块黑。要隐藏用 `opacity(0)`，别动组件树。
- **预览的比例要按屏幕算，不是按 profile 算**：档位全是横的（`1280×720` 这种），但竖屏
  拿着时摄像头**把画面转好了才送进 surface**，到手是一张竖图。照 `profile` 的比例画 =
  横向拉宽约 3 倍（真机实测）。正确做法：

  ```ts
  const screen = display.getDefaultDisplaySync();          // @kit.ArkUI
  const portrait = screen.width < screen.height;
  this.aspect = portrait ? profile.size.height / profile.size.width
                         : profile.size.width / profile.size.height;
  ```

  面板盒子另算（16:9，让闲置面板不至于把下面的卡片顶出第一屏），画面用真实比例撑出去、
  由盒子的 `.clip(true)` 居中裁掉超出的部分——**裁切，不拉伸**。固定高度装预览同样会压扁。
- **`aboutToDisappear` 抓不到 Home 键**（页面还活着，只是看不见）。真正的时机是 ability 的
  `onBackground`：它把 `appLife().foreground = false` 写进那个 `@ObservedV2` 对象，组件用
  `@Local life` + `@Monitor('life.foreground')` 接住（V2 的深层路径监听）。相机在后台开着是
  实打实的 bug（耗电 + 占设备）。
- **预览 profile 别贪大**：这台机器最大档是 2688×1216，铺到小面板里纯属浪费，≤1280 取最大即可。

### 四、真像素：平台解码，Go 做数学

照片要「看懂」就得碰像素。这里有个分工问题：**解码留给平台，数学留给 Go**——
鸿蒙的解码器认识相机写出的所有格式（含 HEIF）还能顺手缩放，Go 不需要知道 JPEG 是什么；
而像素数学是纯函数，能在 Mac 上测。

```ts
// ArkTS：系统解码 + 缩到 320 宽，拿 RGBA
const source = image.createImageSource(file.fd);
const pixels = await source.createPixelMap({
  desiredSize: { width, height },
  desiredPixelFormat: image.PixelMapFormat.RGBA_8888,
});
const buffer = new ArrayBuffer(width * height * 4);
await pixels.readPixelsToBuffer(buffer);
const result = gocore.analyze(buffer, width, height);   // ArrayBuffer → Go，借指针不拷贝语义
```

```go
// Go：亮度、直方图、主色、8×8 亮度图，全是整数运算
a, err := core.AnalyzePixels(px, width, height, stride)   // → {MeanLuma, Histo[8], Grid[64], Colors}
```

实测（真机，同一段 ArkTS 换两个图源）：

```
240×320 · 300 KB RGBA · 67% bright · 3.5 ms in Go     ← 相册里 2.5 MB 的秋景
240×320 · 300 KB RGBA · 33% bright · 3.8 ms in Go     ← 系统相机现拍的 3.0 MB 照片
```

300 KB RGBA 过桥 + 全部数学 ≈ **3.5 ms**（约 85 MB/s），UI 直接画：8×8 亮度方块、
8 根直方图、三个主色。缩到 320 宽是刻意的——12 MP 全量算一遍要 300 ms，而结论一样。

**要点**

- **`stride` 是真的**：平台缓冲按行对齐，行尾有填充字节；当 `width*4` 处理会让画面斜切。
  `readPixelsToBuffer` 给的是紧排的，但接口留了 `stride` 参数，测试里也覆盖了填充行。
- **主色是按像素数统计的**，所以一张一半天空的照片里「主色」会是浅色。这是算术，不是判断；
  想要「最鲜艳的颜色」得另算一个指标。
- **别把解码塞进 Go**：`image/jpeg` 能解 JPEG，但解不了 HEIF，而相机默认存 HEIF。

### 五、订阅 + 后台：定位与长时任务

定位有两层：**权限**（一对，粗+精必须一起申请）和**后台**（要长时任务，否则一切到后台就被冻结）。

```ts
// 前台：一个订阅，回调里一句话交给 Go
const request: geoLocationManager.ContinuousLocationRequest = {
  interval: 1,
  locationScenario: geoLocationManager.PowerConsumptionScenario.HIGH_POWER_CONSUMPTION,
};
geoLocationManager.on('locationChange', request, (loc) => {
  gocore.location(loc.latitude, loc.longitude, loc.accuracy);   // → 走了多远、朝哪、多快
});

// 后台：向系统申请一个「长时任务」（会有一条常驻通知，用户能看见、能停）
await backgroundTaskManager.startBackgroundRunning(ctx, ['location'], await wantAgent.getWantAgent(info));
```

`module.json5` 里两处缺一不可：`requestPermissions` 加 `ohos.permission.KEEP_BACKGROUND_RUNNING`
（system_grant），ability 加 `"backgroundModes": ["location"]`。

实测（真机，静止放桌上）：

| | 前台 | 切后台 45 s 后回来 |
|---|---|---|
| fix 数 | 2 | **61**（≈1/s，一直没断） |
| 日志 | — | `NotifyContinuousTask Resume … mode: 4`、`location:background to provide network position` |

**Go 在后台真的在跑吗？**（这是个该量的问题，不是该相信的问题）

同一份代码、同一台机器，只差长时任务，用 Go 自己的 `time.Tick` 每 5 秒打一行探针：

| | 后台 30 秒里 Go 的输出 |
|---|---|
| **没开长时任务** | 冻结前 10 行，之后 **20 秒零输出** |
| **开了长时任务** | `#15 … #20`，17:15:44 → 17:16:09，**每 5 秒一条，无缺口** |

结论：**申请保活的权力在 ArkTS（没有 Go 绑定，第三方应用也绕不过去），被保活的进程里跑的是
Go**。进程活着 = Go 活着；进程被冻结 = Go 一起停。别指望一个 goroutine 自己撑住后台。

> 顺带一个反面教训：`up 2m23s` 那种数字**不能**当证据——它是墙钟时间，进程冻 60 秒再解冻也
> 会跳到这个值。要证明「一直在跑」，只能让代码自己在后台留下带时间戳的痕迹。

**要点 / 坑**

- **「走了 234 m」是假的**：人没动，±22 m 的 GPS 抖动累加起来就是一分钟 234 米。现在按
  `step > max(上次精度, 2 m)` 过滤——小于精度半径的位移不是位移，界面照样显示 `±22 m`
  让用户知道为什么。修完同样场景是 `0 m walked · 0.0 m/s`。
- **`interval` 只是上限**，不是节奏保证：静止时系统会合并上报（实测 40 s 里只有 3 个 fix，
  另一次 87 s 里有 61 个）。别在 UI 上承诺「每秒一个点」。
- **后台就要有通知**：长时任务必然在通知栏留一条，这是设计（用户有权看见谁在定位）。
- **`background` 是 ArkUI 组件的内置属性名**，`@Local background` 会编译报
  "not assignable to the same property in base type"——换个名字（这里是 `bgTask`）。
- **回前台不会自动恢复**如果你没开长时任务（那时后台就被停了）；开了长时任务则一直在跑。

### 五个例子的共性

- **高频回调在 Go 里节流**：50Hz 进来、30Hz 出去。不要在回调里改 ArkTS 状态。
- **传 fd，不传路径**：系统 picker 的 URI 不是普通路径，`fs.openSync` 之后把 `fd`
  交给 Go（`os.NewFile(uintptr(fd))`），比让 Go 去解析 URI 稳得多。
- **逻辑放 `core`，签名只依赖 `io.Reader`**：所以 `go test` 能不碰设备就覆盖格式判断、
  边界情况（空文件、不认识的头）。
- **生命周期**：页面 `onPageShow/onPageHide` → `gocore.lifecycle(true/false)` → Go 暂停
  后台的 goroutine（demo 里心跳在后台会停）。真 App 该在这儿停网络轮询、定时器。

## 实测：JSON 还是二进制？（别猜，量）

真机（HBN-AL00 / API 26）上，"Measure 20k" 按钮量的同一页 2000 行：

| 路径 | 时间 | 字节 |
|---|---|---|
| JSON（字符串 + `JSON.parse`） | **2.8 ms** | 173 KB |
| 二进制（ArrayBuffer + 手写解码） | 2.9 ms | **120 KB** |
| 二进制**只过桥、不解码** | **0.3 ms** | 120 KB |

三条结论，其中一条推翻了我最初的假设：

1. **桥不是瓶颈**：120 KB 过去只要 0.3 ms，cgo/NAPI 的拷贝可以忽略。
2. **`JSON.parse` 不慢**：它是原生实现，比 JS 写解码循环还略快。所以"JSON 在大数据上一定慢"
   **是错的**——至少 2000 行这个量级完全不是问题。
3. 二进制的价值在**另外两点**：体积小 30%，以及**可以只解码你要渲染的行**。
   列表只显示 20 行时，代价是 0.3 ms + 20 行的解码，而不是 2000 行。

**所以怎么选**：

- 表单、详情、几十到几千行的数据 → **就用 JSON**，简单、可读、够快。
- 上万行的列表 / 图表 / 图片 / 音视频帧 → 用二进制（`ArrayBuffer`），并且**按需解码**；
  别把整个列表解成对象数组，让 `LazyForEach` 在 `getData(index)` 里解那一页。
- 真正的性能瓶颈通常在**渲染**（每帧要创建/更新多少个 ArkUI 节点），不在传输。

## 实测：Go 快在哪，不快在哪

四组数字，真机（HBN-AL00 / API 26）和 Mac M3 都量过。**结论先给：Go 本身不带来速度；
带来速度的是数据结构、事务和线格式。**

### 一、过桥（真机，`Bridge µs` 按钮，4 次采样）

| 调用 | 时间 |
|---|---|
| `remaining()`：一个 int 回来 | **277–296 ns** |
| `snapshot()`：一个小 JSON 回来并 `JSON.parse` | **1.5–1.7 µs** |
| `analyze()`：300 KB RGBA 进去，数学全部算完 | 560–615 µs（其中桥 26–143 µs） |

一次跨语言调用 **0.28 µs**。所以「数据库查询要不要异步」这个问题问错了：查一次点查是
3–8 µs（见下），过桥 1.5 µs，合计是一个 16 ms 帧的 0.05%。**同步查，没事。**
（第三个数字里的桥份额是两个时钟相减来的，抖动大，只当量级看。）

> 这几个数会随会话漂：同一天晚些时候重测 5 次，`snapshot()` 那行读到过 **3.0–3.4 µs**
> （同一个 build、同一个按钮）。后来加了 `platform/ohos/bench.sh`（`aa start --pi bench N`，
> 不点按钮、直接从 hilog 取数），连跑 5 轮又是 **1.5–1.7 µs** —— 所以 3.0–3.4 那次是**会话异常**，
> 不是这个 build 的性质。结论：绝对读数跨会话不可比，要比就在一次会话里、用同一条命令比完。

### 二、像素数学：两个实现错，3.7×（真机）

同样 240×320 / 300 KB RGBA：

| | 真机 | Mac M3 | allocs/op |
|---|---|---|---|
| 初版：`map[uint16]*colorBucket` 分色 + `sort` 找前三 | 1906–1939 µs | 812–919 µs | 144 |
| `map` → `[4096]colorBucket` | — | 398–440 µs | 12 |
| 再砍掉排序，三槽插入选优 | **472–534 µs** | **359–376 µs** | **9** |

两处都是实现错，不是语言问题：只有 4096 个可能的 key，用哈希表是白付一次查找/像素；
而为了拿前三名去排 4096 个桶，等于把省下的钱又花回去——同一个函数在单色图上只要 217 µs，
彩色渐变要 415 µs，差的就是那笔排序。

代价：每次调用多分配 128 KB（4096 × 32 字节，要清零）。照片缓冲本身 300 KB，这个交换
划算；真要在 30 fps 上跑小图，值得再看一眼。

`go test ./core/ -bench Analyze -benchmem` 有 240×320 / 10 MP / 带 padding 三个用例。
10 MP 那个是 `maxSamples` 的性质证明：全分辨率照片只要约 1.7 ms（Mac）。

### 三、SQLite 驱动（Mac M3，2 万行，WAL + `synchronous=NORMAL`）

| | `mattn/go-sqlite3`（cgo） | `modernc.org/sqlite`（纯 Go） |
|---|---|---|
| 单事务插入 | **672–720 ns/行** | 811–835 ns/行 |
| autocommit，每行一次 prepare | 9.6–10.0 µs/行 | 12.0–13.0 µs/行 |
| 2000 行整页扫进 Go | 653–658 µs | 636–679 µs |
| 主键查一行 | 2.6–3.3 µs | 3.1–3.4 µs |
| `COUNT(*) + SUM(id)` 全表 | **366–396 µs** | 1.05 ms（2.8×） |

两条反直觉的：

1. **事务比驱动重要 14 倍**：9.6 µs → 0.67 µs 是「进不进事务」的差别，两个驱动同比例。
2. **读页面两个驱动打平**：钱在 `database/sql` 的 `Scan` 和每行字符串分配（7659 allocs/op），
   不在驱动。纯 Go 驱动只在计算密集型查询（聚合）上露馅。

复现：`cd go/bench/sqlite && CGO_ENABLED=1 go test -bench . -benchtime 5x -count 2`
（独立 module，主 `go.mod` 不受影响；mattn 那条**必须**显式 `CGO_ENABLED=1`，否则测的是 stub。）

### 四、Go+SQLite 对 relationalStore（真机，同一张表、同样 2 万行）

两边**都是 SQLite**，所以量的是封装层和线格式：

| | Go（`modernc`，core 自己的文件） | `relationalStore`（平台的文件） |
|---|---|---|
| 批量写 2 万行 | **57.2–59.9 ms**（单事务） | 245.0–257.9 ms（10 个 batchInsert） |
| 读一页 2000 行 | 6.3–10.4 ms（**库本身 4.6–8.7 ms**） | 4.5–8.8 ms |

- **写快 4.2×**：单事务 + 一条 prepared statement。
- **读打平**：库本身 Go 不吃亏，但 JSON 过桥要再花 2–4.5 ms，正好把优势吃掉。想赢就换
  二进制线格式（见上一节），这也是 `rows()` 存在的理由。
- **两个文件，不是一个**：一个文件只有一个写者。让两个句柄写同一个 `.db` 是 `SQLITE_BUSY`
  的预约单，而且只在最不方便的时候出现。

代价：`modernc.org/sqlite` 让 `libgocore.so` 从 4.2 MB 涨到 **11 MB**。换来的好处不小：
纯 Go，所以 `scripts/check.sh` 的主机侧能直接跑 `go test ./store/`；而且它的 **TLS 断言
仍然 clean**——没有破坏那三个 runtime 补丁的前提。

## 为什么需要那三个 runtime 补丁

这三个坑跟框架无关，是**任何 Go 库进鸿蒙 App** 都要面对的：

| 现象 | 原因 | 补丁 |
|---|---|---|
| `initial-exec TLS resolves to dynamic definition`，`.so` 加载失败 | Go 用 IE TLS 存 `runtime.tls_g`；musl **不给 dlopen 的库分配静态 TLS** | `runtime/tls_arm64.s`：g 存进本库自己的动态 TLS（`tls_linux.c` 的 `__thread`） |
| `fatal error: failed to get system page size` → 进程 exit(2) | musl 调库构造函数**不传 argc/argv**，Go 把寄存器垃圾当 argv/envp/auxv | `runtime/rt0_linux_arm64.s`：库模式给一个空的 argv/envp |
| 同上（auxv 读不到时的兜底） | app 沙箱里 `/proc/self/auxv` 读不出来 | `runtime/os_linux.go`：`physPageSize==0` 时用 `mincore` 探针 |

`build.sh` 用 `go build -overlay` 把三处运行时实现换掉，另注入 `runtime/gokit_ohos.go`
的两个符号声明，**不动你机器上的 Go**。因此修补行为是 3 处，overlay 文件共 4 个。
overlay 的清单和这些检查都由 `tools/patch` 一处给出（`build.sh` 调它生成 overlay，不再自己抄一份文件名）。

它盯三件事，都是「升级 Go 之后会静默坏掉」的那种：

| 命令 | 盯什么 | 坏了会怎样（不检查的话） |
|---|---|---|
| `check` | 补丁的标记还在、**上游锚点**还在（`sysargs`、`load_g`、`_rt0_arm64_linux`） | 编译照过，运行时崩，手机上只剩白屏 |
| `verify` | 产物里 **没有任何 initial-exec TLS 重定位**、该链的库链了 | musl 拒绝加载，ArkTS 报 `does not provide an export name 'start'` |
| `overlay` | 把这份清单交给任何工程用（`go build -overlay`） | 各抄一份，慢慢漂移 |

```sh
cd tools/patch && go run . check                    # 4 patches are in place and still fit go1.27.3
cd tools/patch && go run . verify libgocore.so      # clean (4.2 MB, links …, no initial-exec TLS)
```

两条失败路径都实测过：把上游 `sysargs` 改名 → `check` 报
`os_linux.go no longer contains "func sysargs(argc int32, argv **byte) {"`，退出码 1；
`verify -expect libbogus.so` → `would not load`。

**升级 Go 时**：`runtime/*.s`、`runtime/os_linux.go` 跟着 Go 版本走（当前取自重
go1.27.3），升级后跑 `scripts/check-runtime.sh`，再跑一遍 `platform/ohos/run.sh` 看
`hilog -T gokit` 里有没有上面那两条错误。`gokit_ohos.go` 是新增文件，一般不用改。

## 已知边界

诚实清单，别指望它做这些：

- **UI 不跨端**。这是设计选择，不是缺陷——见开头。
- **没有热重载**。改 Go 要重编 `.so` + 重装 HAP，`run.sh` 一次几十秒。
- **Go 侧没有调试器**。没有 dlv，只能 `hilog` + `llvm-symbolizer` 还原地址。
- **同步调用在主线程**：>16ms 的活儿要异步化，否则掉帧。
- **后台不保活**。goroutine 活在 App 进程里，App 被冻结就一起停；真正的后台任务要用
  `workScheduler` / `backgroundTaskManager`（ArkTS 侧申请，Go 侧执行）。
- **只在 arm64 真机验过**（HBN-AL00 / HarmonyOS 6.1 / API 26）。x86_64 模拟器的 TLS 细节
  不一样，没测。
- **`.so` 4.1 MB**：Go runtime 自带一套（GC、调度器、TLS），这是它换来的东西。

## 排错手册

| 现象 | 先看哪 |
|---|---|
| App 一起就退出，ArkTS 报 `does not provide an export name 'start'` | **`.so` 没加载成功**。`llvm-readelf -d libgocore.so \| grep NEEDED` 看依赖；`llvm-nm -D` 找未定义符号；忘了链接某个库就是这样 |
| Go 里 `fmt.Println` 看不到 | `hdc shell hilog -T gokit`（`log_linux.c` 把 fd 1/2 接到 hilog） |
| Go panic 没输出 | 同上；traceback 走 fd 2，已被接住 |
| `Error: Permission denied`（传感器/相机/定位） | 每类能力一条权限，写在 `module.json5` 的 `requestPermissions`：加速度计 `ohos.permission.ACCELEROMETER`（装包生效），相机/定位要运行时申请 + `reason`/`usedScene` |
| 崩溃只看到一串地址 | `hdc shell hilog -x \| grep FAULT`（`fault_linux.c`）→ `llvm-symbolizer --obj=libgocore.so 0x<偏移>` 还原 Go 函数名 |
| `filesDir` 相关报错 | 环境变量在鸿蒙里是空的（空 argv/envp 补丁），路径必须从 ArkTS 传进 Go |
| `getContext` 弃用警告 | 用 `this.getUIContext().getHostContext()` |
| 编辑器对 `main_linux.go` 报一堆 `undefined: C.xxx` | gopls 没有 cgo/linux 环境，对**被 build tag 排除的 cgo 文件**的孤儿分析。以 `platform/ohos/build.sh`（交叉编译）和 `go test ./core/` 为准 |
| 装不上，hvigor 说 `No signingConfig found` | 见「签名」一节：模板生成了但还没在 DevEco 里签过 |

## 扩展它

**加一个能力，三处，一分钟**：

1. `go/core/` 里写逻辑 + 测试（纯 Go，Mac 上就能验）
2. `go/main_linux.go` 加一个 `//export` 函数；`go/napi_linux.c` 里加一个 `napi_property_descriptor`
3. `app/entry/src/main/cpp/types/libgocore/index.d.ts` 加类型，`core/GoCore.ts` 包一层

**新起一个工程**：

```sh
platform/ohos/new.sh myapp                  # apps/myapp/{go,app} + 自己的 build.sh/run.sh
platform/ohos/new.sh myapp --bundle com.x.y # 换包名（会摘掉旧签名，需在 DevEco 里重新自动签名）
```

## 这个 demo 验过什么

`app/` 是个待办清单：Go 管状态、去重、持久化到 `filesDir/todos.json`，还有一个
goroutine 每秒推一个心跳（右上角 "up 12s"）；ArkTS 只做渲染和转发用户操作。
旁边挂了五张卡：`Photo` / `Camera`（拍照或选图 → 平台解码 → Go 量像素）、
水平仪（50Hz 加速度计）、`Start camera`（权限 + surface + 后台关闭），
外加量化过的三个按钮组：`Measure 20k`（传输格式）、`Bridge µs`（过桥成本）、
`SQLite`（Go 自己的库 vs 平台 `relationalStore`）。

真机验过的清单：

- 同步调用（增 / 改 / 删），异步推送（goroutine → 主线程，心跳）
- 沙箱持久化：重启 App 后待办还在
- 免权限系统 picker → fd → Go 判格式 + 算哈希
- 系统相机拍照 + 相册选图，两条路都到同一段像素分析（240×320 / 300 KB；Go 侧 1.9 ms →
  0.56 ms，见「实测：Go 快在哪」）。这两个数来自照片卡，是**单次**读数、会随设备状态飘；
  要引用请看 `platform/ohos/bench.sh` 的中位数。
- 定位：权限（粗+精）→ 订阅 → Go 算距离/方位/速度；长时任务让它在后台继续报点
- 50Hz 加速度计（`50 samples/s`，同页其它部分不受影响）
- 相机预览 + 按 Home 后 242ms 关闭
- 2000 行列表的三种传输格式实测（见上表）
- Go 自己的 SQLite 文件（纯 Go 驱动）与平台 `relationalStore` 的同表对比

## License

MIT
