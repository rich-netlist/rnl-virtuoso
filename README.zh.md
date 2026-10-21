# rnl-virtuoso

[English](README.md)

在 Virtuoso schematic editor 中增加 **RNL → Export RNL...** 菜单，
在 CIW 中增加 **File → Export → RNL...**，支持选择 cell 或导出整个 library，
将原理图导出为 [Rich Netlist（RNL）](https://github.com/rich-netlist/rnl)。
SKILL 前端调用原生 Spectre/CDL 和 EDIF 导出器，Go 后端组合网表与图形。

保留层次、子电路 symbol、位置、方向、连线、总线和阵列画法；支持合并文件、
独立子电路文件及只导出当前层。带 port 的原理图封装为 subcircuit，否则作为 top。
`eda-symbol` 与 `eda-style` 分别引用，姿态补偿表可编辑。

## 安装

从 [Releases](https://github.com/rich-netlist/rnl-virtuoso/releases) 下载 Linux x86-64 安装包，
在 Virtuoso 服务器上解压：

```sh
mkdir -p ~/tools
tar -xzf rnl-virtuoso-0.8.0-linux-amd64.tar.gz -C ~/tools
```

在 CIW 中执行一行即可，也可加入项目的 `.cdsinit`：

```skill
load("~/tools/rnl-virtuoso/rnl-virtuoso.il")
```

菜单自动注册，重复加载不会重复添加。服务器无需安装 Python、Go 或 bridge，
但需要原生 `si`、`edifout` 和对应 Cadence 许可证。已在 IC6.1.8-64b.500.26、
Linux x86-64 上测试。

## 使用

先对原理图层次执行 Check and Save，再选择 **RNL → Export RNL...**，指定新的输出目录。
仿真源、开关和 Verilog-A 模型优先使用 **Spectre (simulation)**；CDL 用于 LVS，可能省略这些器件。
导出后保持整个目录一起使用。模型引用仍为外部引用，本工具不会打包完整仿真环境。

在 **CIW → File → Export → RNL...** 中，选择 **Cell** 可直接选择库和 cell，
无需先打开 schematic editor；选择 **Library** 可遍历整库，包括只有 symbol 的 cell。
每个 cell 的实际电路直接保存为根目录的 `<cell>.scs` / `.sp`，
外部 cell 保存到 `<外部库名>/<cell>.scs` / `.sp`，同一外部 cell 只保存一份。
不再拆分入口文件和定义文件，也没有 `netlists/` 目录。
带 port 的 subcircuit 不写原生 include；无 port 的 top 平铺 include 全部所需下级文件，每个文件一次。
单 cell 导出仍可选合并文件；整库固定每个 cell 一个文件，共用 `symbols.rnl` 和 symbol/style 包。
关闭外部 cell 导出时保留外部调用和符号，开启后连带导出原生网表中的外部子电路定义。
`library-report.json` 列出入口、定义、符号及跳过的无 schematic/symbol 的 cell。
导出失败的 cell 会跳过并继续处理其余 cell；报告的 `failed_cells` 保存失败原因与日志路径，
只要有失败项就会标记 `complete: false`，完成提示也会显示失败数量。

更多选项与边界见 [导出说明](docs/export.md)。

## 脚本与 agent

公开 SKILL API 无需打开编辑器，也不会弹出完成窗口：

```skill
job = rnlExportCell("myLib" "myCell" "~/exports/myCell")
rnlExportStatus(job)
```

整库使用 `rnlExportLibrary`；`rnlExporterInfo` 查询安装信息。
可通过关键字参数设置格式、层次、外部 cell 等。导出异步执行，启动返回任务 ID，
之后查询成功、部分失败或失败状态。完整用法见 [API 文档](docs/skill-api.md)。

将 [skills/rnl-virtuoso](skills/rnl-virtuoso) 复制到 agent 的技能目录
（Codex 为 `~/.codex/skills/rnl-virtuoso`）即可使用。
该技能依赖已有的 `virtuoso` bridge 技能，先检查远端安装；已有兼容版本直接使用，
未安装时征得同意后从本仓库获取最新发布版，安装到用户目录。
技能不附带导出工具或 bridge 资产。

## 开发

仅开发机需要 Go 1.22+ 和 Python 3.8+：

```sh
python scripts/package.py dist/rnl-virtuoso-0.8.0-linux-amd64.tar.gz
python scripts/test.py --rnl /path/to/rnl
```

未指定 RNL 检查器时，会明确跳过依赖它的图形检查。构建和测试细节见 [开发说明](docs/development.md)。

## 许可

代码、测试与文档采用 [MIT](LICENSE)；附带 symbol/style 包采用 CC0-1.0。
Go runtime 和标准库声明见 [Go-LICENSE.txt](Go-LICENSE.txt)。不包含 Cadence 软件、库符号或用户设计。
