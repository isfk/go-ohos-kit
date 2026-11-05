# AGENTS.md

给在这个仓库里干活的 agent（也给我自己）。README 讲「是什么、为什么」，这里只讲
**动手时要知道的事**。

## 心智模型

三层，边界别糊：

```text
ArkTS / ArkUI   ← 界面、权限、平台 Kit、生命周期都在这一层
    │  import 'libgocore.so'
NAPI 桥         ← go/napi_linux.c：只搬运，不含业务
    │  cgo
Go 内核         ← go/core：状态、逻辑、IO；goroutine 随便开
```

## 边界：什么放 Go，什么放 ArkTS

四条规则。定它们是为了下次加功能时不用再讨论一遍。

**1. 门槛是「能不能测」，不是「哪个更快」**

| 放 Go | 放 ArkTS |
|---|---|
| 领域规则、状态机、校验、持久化、迁移、调度、并发 | `getContext()`、权限、Kit、surface、Ability/页面生命周期、渲染 |

一行判据：**这段代码能不能在没有 ArkUI 的情况下写单测？能 → Go，不能 → ArkTS。**
性能不是理由（见「已被证伪的说法」：桥 0.28µs，换语言也不变快）。

**2. 桥只搬数据，`GoCore.ts` 是唯一知道传输格式的文件**

- 组件不许 `import core from 'libgocore.so'`；JSON 还是二进制，只在一个地方决定。
- `main_linux.go` / `napi_linux.c` 里不许出现业务分支（铁律 3）。

**3. 状态分两档**

| 装饰器 | 放哪 | 给什么用 |
|---|---|---|
| `@Local` / `@Param` / `@Event` | **组件文件里**，不出文件 | 只有这个组件关心 |
| `@ObservedV2` + `@Trace` + `AppStorageV2` | `state/` 目录 | 跨组件/跨页面，或被 Ability 写 |

别把只有一处用的 `@Local` 提权成全局 store。全局状态每多一个，「谁在什么时候改了它」
就难查一分；`state/` 里文件少是正常的，不是缺陷。加进去的判据只有一个：**同一份数据被两个
以上页面/组件写，或被 Ability 写。**（`AppLife` 就是标准例子：Ability 写、组件读。）

**4. Go 的事件只有一个入口**

`events.ets` 决定哪个 `kind` 交给谁，页面只调一次 `startCoreEvents()`。
新增一类事件只改订阅方，**不改页面**。（之前页面里那串 `if (e.kind === …)` 就是反例：
每加一个事件都得去改一个本不该碰事件分发的页面文件。）

## 五条铁律

1. **`go/core/` 必须保持纯 Go**：不 cgo、不 import 任何 HarmonyOS 相关的包、不认识
   `filesDir`。它要能在 Mac 上 `go test ./core/` 跑通——这是唯一不用设备就能验证的那部分。
   路径、平台句柄一律从 ArkTS 传进来（参数或 `io.Reader`/`fd`）。
   `go/store/` 同理（SQLite 选 `modernc.org/sqlite` 就是为了这条）：文件路径也从 ArkTS 来。
2. **平台的东西留在平台侧**：解码（HEIF/JPEG 都归系统解码器）、权限、surface、Kit 全在
   ArkTS；Go 只收已经规整的数据（RGBA 字节、fd、JSON）。别在 Go 里塞 `image/jpeg` 去解相机
   照片——相机默认存 HEIF。
3. **业务逻辑进 `core`**。`go/main_linux.go` 只做搬运（解参数 → 调 core → 序列化），
   `go/napi_linux.c` 只做桥（NAPI ↔ cgo）。往这两个文件里塞业务，就是在把可测的东西变不可测。
4. **ArkTS 只用状态管理 V2**：`@ComponentV2` + `@Local` / `@Param` / `@Event` / `@Monitor`；
   跨组件共享用 `@ObservedV2` + `@Trace` + `AppStorageV2`（见 `app/entry/src/main/ets/state/AppLife.ets`）。
   V1 的 `@State` / `@Prop` / `@Link` / `@Watch` / `@StorageLink` **不能和 V2 混用**，一个也别加。
5. **不要动 `platform/ohos/runtime/` 里的三个补丁**，除非在升级 Go。那时按
   `scripts/check-runtime.sh` 的提示做，并保证 `scripts/check.sh` 里的硬断言还成立：
   **产物的 `TLS_TPREL64` 重定位计数必须为 0**。

   改补丁之后必须跑 `cd tools/patch && go run . check`：它盯「上游锚点还在不在」，
   这才是升级 Go 时真正会坏的东西（编译不会有任何意见）。补丁清单只在
   `tools/patch/patches.go` 一处，`build.sh` 的 overlay 由工具生成，别再手抄。

   **`platform/ohos/runtime/VERSION` 和 `go/go.mod` 的 `go` 指令是同一件事**：前者说
   “这些源码取自哪个 release”，后者说“这个模块至少需要哪个 release”。升级 Go 时两个一起
   改，`tools/patch check` 现在会把它们对一遍 —— 曾经一个写 1.27.3、一个写 1.26.0，然后
   什么也没发生（`go build` 照过，`check` 只打一行 `note:`）。想换一个更硬的检查？
   下一步是让 `check` 在版本不一致时直接失败，现在的取舍写在 `cmdCheck` 里。

   已决定**不往 Go 上游提这三个补丁**（2026-11 讨论）：它们碰的是
   `-buildmode=c-shared` + musl 的坑，而 OpenHarmony 不是 Go 的 GOOS；要能提，唯一
   卖点是把复现搬成「Alpine 上 dlopen Go 库」，成本高、`os_linux.go` 那 34 行还要证明
   对所有 linux 构建无副作用。结论：本地用 `-overlay` 维护，够了。别再重复论证这件事。

## 命令

| 想做什么 | 命令 | 要设备？ |
|---|---|---|
| Go 逻辑 | `cd go && go test ./core/` | 不要 |
| 数据库逻辑 | `cd go && go test ./store/` | 不要 |
| 驱动对比（2 个驱动） | `cd go/bench/sqlite && CGO_ENABLED=1 go test -bench . -benchtime 5x` | 不要 |
| 像素数学基准 | `cd go && go test ./core/ -bench Analyze -benchmem` | 不要 |
| 全套自检 | `scripts/check.sh` | 不要 |
| 无 SDK 机器 | `scripts/check.sh --no-build` | 不要 |
| 只编 `.so` | `platform/ohos/build.sh go -project app` | 不要 |
| 编 + 装 + 跑 | `platform/ohos/run.sh` | **要** |
| 内核日志 | `hdc shell hilog -T gokit` | 要 |
| 崩溃地址还原 | `hdc shell hilog -x` 筛 `FAULT` → `llvm-symbolizer --obj=libgocore.so 0x<偏移>` | 要 |

`hdc` 在 `$DEVECO_SDK_HOME/default/openharmony/toolchains/hdc`（默认
`~/command-line-tools/sdk`）。

## 改完怎么验：不许「应该没问题」

按改动面选证据，**每条都要有实物**：

| 改了什么 | 拿什么结账 |
|---|---|
| `go/core` | `go test ./core/` 输出（新逻辑必须有新用例） |
| 桥 / 新 `//export` | `scripts/check.sh`（内含交叉编译）+ 真机调用一次 |
| ArkTS / UI | `platform/ohos/run.sh` + 真机截图；`hilog` 佐证 |
| runtime 补丁 | `scripts/check.sh` 的 TLS 断言 + 真机启动 |

- **截图是证据，但要看清**：`hdc shell snapshot_display -f /data/local/tmp/x.jpeg` +
  `hdc file recv`。别凭想象描述画面。
- **别用自动化工具替用户验手势**：`uitest uiInput drag/swipe` 对拖拽/捏合测不准；
  需要真手指的地方，把步骤写清楚请人来点。
- **判断错了要改口**：先复现、再归因，别把「我看到的现象」直接当「原因」。黑屏可能是
  摄像头对着暗处，压扁才是布局问题——两者混在一起会连着修错两个地方。

## 加一个能力：三处，一分钟

1. `go/core/` 写逻辑 + 测试；
2. `go/main_linux.go` 加一个 `//export` 函数，`go/napi_linux.c` 里注册
   `napi_property_descriptor`（返回 JSON 字符串，或走 TSFN 推事件）；
3. `app/entry/src/main/cpp/types/libgocore/index.d.ts` 加类型，
   `app/entry/src/main/ets/core/GoCore.ts` 包一层带类型的函数。

平台侧能力（权限、Kit、surface）留在 ArkTS，不要试图在 Go 里模拟。

## 陷阱清单

细节在 README「接平台能力」和「排错手册」两节，这里只列最容易再踩的：

- `session.start()` 在本项目的相机 API 版本里**不是 Promise**，`.then` 会静默炸。
- `XComponent` 高度 0 = 没有 surface；别在 `Stack` 里随状态增删兄弟节点（会重建 surface）。
- `user_grant` 权限（相机/定位）必须写 `reason` + `usedScene`，`system_grant`（传感器）不用。
- 预览面板要用 `.aspectRatio(...)`，固定高度会把画面压扁。
- `aboutToDisappear` 抓不到 Home 键，后台处理要接 ability 的 `onBackground`（`AppStorage`）。
- 鸿蒙里环境变量是空的（空 argv/envp 补丁），沙箱路径必须从 ArkTS 传进 Go。
- 大块数据别用 JSON，用 `ArrayBuffer` + 按需解码（2000 行这个量级 JSON 完全够用，别优化错方向）。
- **列表事件的 `todos` 字段不能 `omitempty`**（真机上踩过）：“删掉最后一行”时列表是空的，
  `omitempty` 把整个字段省掉，发出去就是 `{"kind":"todos"}`——UI 把“回包没有列表”
  当成存档失败（曾经的错误对象就长这样），于是行删了却弹一行红字。`core.go` 的
  `todosJSON()` 现在是唯一的列表线格式出口：永远带 `todos`，nil 也要先规范成 `[]`（否则是 `null`）。
  `TestAnEmptyListStillSendsTheField` 钉住了这个形状。
- **借来的 fd：`dup` 保不住偏移量**。fd 的偏移量属于 open file description，而 `dup` 拿到的是
  **同一个** description —— ArkTS 让我们 inspect 一个 fd，紧接着又拿同一个 fd 去
  `createImageSource`。所以 `InspectFD` 里的 `dup` 只为“有权关掉它”，读走的是
  `io.NewSectionReader`（`ReadAt`/pread），偏移量一点不动，hash 也覆盖整个文件。
  **别把它改回 `f.Read`**：今天不炸只是因为解码器自己 seek 回了 0。Rust 侧同理（`pread`）。
- 像素缓冲**按行对齐**：`stride` 不等于 `width*4`，当紧排处理会把画面斜切。
- 12 MP 全量算像素要 ~300 ms；先缩到 320 宽（约 3.5 ms），结论一样。
- **后台 = 长时任务**：`KEEP_BACKGROUND_RUNNING` 权限 + ability 的 `backgroundModes` +
  `startBackgroundRunning(ctx, ['location'], wantAgent)` 三件套；不做就一切后台冻结。
  它会留一条常驻通知，这是规矩不是 bug。
- **传感器/定位的数字要按精度过滤**：噪声累加会给出「静止走了 234 m」这种假结论。
- **`background` 是 ArkUI 组件内置属性名**，取同名 `@Local` 直接编译失败。
- **`.zshrc` 里的 `CGO_ENABLED=0`**：cgo 驱动（mattn）会变成一个 stub，第一次查询才报
  「requires cgo」。跑 `go/bench/sqlite` 得显式 `CGO_ENABLED=1`；`build.sh` 自己会设，不用管。
- **热循环里别用 map**：`AnalyzePixels` 每像素一次 map 查找，贵了 2.3×。key 空间就 4096 个，
  上平数组。反过来，**为了拿前三名去 `sort` 四千个桶**也会把钱花回去——三槽插入就够。
- **一个 `.db` 文件只有一个写者**：core 的 `filesDir/gokit.db` 和平台 `relationalStore`
  各自拥有自己的文件。两个句柄写同一个文件 = `SQLITE_BUSY`，而且只在最不方便的时候出现。
- **JSON 过桥会把数据库赢的优势吃掉**：读一页 2000 行，库本身 4.6–8.7ms，加上 JSON 过桥
  变 6.3–10.4ms。要赢就换二进制线格式。

## 已被证伪的说法（别再写回文档）

- ~~「JSON 在大数据上一定慢」~~：2000 行 JSON 2.8ms，二进制 2.9ms。二进制赢在**体积**和
  **按需解码**，不在速度快。
- ~~「桥的拷贝是瓶颈」~~：120KB 过桥 0.3ms；一次空往返（int 回来）**0.28µs**。
- ~~「Go 内核比 ArkTS 快」~~：没有一项数字支持。桥很便宜（所以「接进来变慢」不成立），
  但也没有「换语言变快」：像素数学那 1.9ms 是 `map` + `sort` 的错，换数组 + 三槽选优后
  0.47ms——快的是数据结构，不是语言。
- ~~「数据库查询必须异步化」~~：点查 3–8µs + 过桥 1.5µs ≈ 一个 16ms 帧的 0.05%。同步查没事。
  该异步的是全表扫（2 万行 ≈ 14ms）和聚合。
- ~~「纯 Go 的 sqlite 驱动读页面慢一倍」~~：2000 行页面 636–679µs vs cgo 653–658µs，打平;
  纯 Go 只在计算密集型查询上慢（聚合 2.8×）。
- ~~「12 MP 全量算像素要 300 ms」~~：那是旧代码。有 `maxSamples` 之后 10 MP 约 1.7ms（Mac）。
- ~~「预览用最大 profile 最好」~~：2688×1216 铺到小面板纯属浪费。

## 别提交、别碰

- `app/build-profile.json5`：**已 gitignore**，里面是本机证书路径 + 密码占位、还绑着包名。
  入库的是 `app/build-profile.example.json5`（`signingConfigs: []`）。改模板可以，改真文件不许入库。
- `libgocore.so` / `*.h`、`.cache/`、`oh_modules/`、`build/`、`.hvigor/`：都在 `.gitignore` 里。
- 删文件、改包名、卸载设备上的 App：先问人。

## 假警报（别浪费时间）

- **Mac 上看不到「Linux-only cgo」问题**：桥的文件全是 `*_linux.c`，在 macOS 上根本不会被
  编译；CI 是 Linux，`go test ./...` 会把它们编进去，然后死在 `hilog/log.h: No such file`
  （这台机器没装鸿蒙 NDK）。所以 `scripts/check.sh` 的主机侧检查只跑 `./core/...`，
  桥的正确性由交叉编译那步负责。
- **gopls 对 `go/main_linux.go` 报一堆 `undefined: C.xxx`**：gopls 没有 cgo/linux 环境，
  那是被 build tag 排除的 cgo 文件的孤儿分析。以交叉编译为准。
- **clangd / semgrep 对 `go/napi_linux.c` 报缺失头文件**：那文件用 `__has_include` 守卫，
  NDK 头只在 hvigor 交叉编译时存在于搜索路径。以 `scripts/check.sh` 的交叉编译为准。

## 约定

- **语言**：代码注释和 commit message 用英文；README / AGENTS.md / 文档用中文。
- **注释写「为什么」**：这类桥接代码最容易变成「注释复述代码」，别那样。
- **`go vet` 必须干净**，`gofmt` 必须跑（`scripts/check.sh` 会查前者）。
