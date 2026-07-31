---
layout: home

hero:
  name: flutter_go_bridge
  text: 连接 Go 与 Dart 的代码生成桥梁
  tagline: 基于 Gokit 的 Go → Dart/Flutter 代码生成工具，将 Go 服务能力转换为 Dart 可用接口。通过自动生成代码，降低跨语言开发成本，保持类型一致性。
  actions:
    - theme: brand
      text: 快速开始
      link: /zh/guide/getting-started
    - theme: alt
      text: 这是什么
      link: /zh/guide/
    - theme: alt
      text: GitHub
      link: https://github.com/star4277/flutter_go_bridge

features:
  - title: CST 进，DCO 出
    details: Dart → Go 的调用走真实的 C wire 结构体；Go → Dart 的结果用 Dart_CObject 回传。两者暂时都表达不了的类型回退到纯 Dart 的 standard codec —— 永远不会回退到 Flutter SDK。
    link: /zh/concepts/serialization
    linkText: 序列化策略
  - title: 唯一稳定 ABI
    details: 所有调用都经由 fgb_cst、fgb_init 等一组固定导出符号分发。新增 Go 函数不会新增 C 符号，你的 CMake 和 Gokit 配置永远不用改。
    link: /zh/concepts/stable-abi
    linkText: 导出符号
  - title: 一个 channel 就是一条 Stream
    details: 加一个 chan<- T 参数，Dart 侧就得到 Stream<T>。channel 由生成代码创建、抽干，并在你的函数返回后自动关闭。再加一个 context.Context，取消也一并接好。
    link: /zh/reference/stream
    linkText: Stream 参考
  - title: 结构体、接口、匿名嵌入
    details: 可翻译的结构体变成 Dart value class，匿名嵌入变成 Dart 继承，命名接口变成 abstract interface class 加 implements。翻译不了的一律降级为 GoOpaque 句柄。
    link: /zh/reference/structs-interfaces
    linkText: 结构体与接口
  - title: Dart 闭包当 Go 回调
    details: Go 函数可以直接接收原生 func(...) 参数，Dart 侧传闭包 —— 同步闭包和 async 闭包都能传，runtime 统一 await 后再把结果回传给 Go。
    link: /zh/reference/callbacks
    linkText: 回调参考
  - title: watch、run、热重载
    details: generate --watch 在保存后自动重新生成。run 替你拉起 flutter run —— 改 Dart 走 hot reload，改 Go 则重启整个进程，因为动态库无法原地热替换。
    link: /zh/guide/dev-server
    linkText: 开发服务器
---

<div class="fgb-flow">
  <div class="fgb-flow-step fgb-flow-step--go">
    <h3>你的 Go 包</h3>
    <p>就是普通 Go 代码。想要 Dart <code>Future</code> 就标 <code>//fgb:async</code>，其余什么都不用动。</p>
  </div>
  <div class="fgb-flow-arrow" aria-hidden="true">→</div>
  <div class="fgb-flow-step fgb-flow-step--gen">
    <h3>flutter_go_bridge_codegen</h3>
    <p>用 <code>go/packages</code> 读取类型，逐调用选择 codec，产出一个 <code>bridge_generated.go</code> 和一棵镜像的 Dart 目录树。</p>
  </div>
  <div class="fgb-flow-arrow" aria-hidden="true">→</div>
  <div class="fgb-flow-step fgb-flow-step--dart">
    <h3>你的 Dart API</h3>
    <p>每个 Go 源文件对应一个 Dart 文件，全部使用命名参数，所有 FFI 细节都只留在 <code>bridge_generated.dart</code> 里。</p>
  </div>
</div>
