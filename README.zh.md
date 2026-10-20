# rnl-virtuoso

[English](README.md)

在 Virtuoso schematic editor 中增加 **RNL → Export RNL...** 菜单，
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
tar -xzf rnl-virtuoso-0.5.2-linux-amd64.tar.gz -C ~/tools
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

更多选项与边界见 [导出说明](docs/export.md)。

## 开发

仅开发机需要 Go 1.22+ 和 Python 3.8+：

```sh
python scripts/package.py dist/rnl-virtuoso-0.5.2-linux-amd64.tar.gz
python scripts/test.py --rnl /path/to/rnl
```

未指定 RNL 检查器时，会明确跳过依赖它的图形检查。构建和测试细节见 [开发说明](docs/development.md)。

## 许可

代码、测试与文档采用 [MIT](LICENSE)；附带 symbol/style 包采用 CC0-1.0。
Go runtime 和标准库声明见 [Go-LICENSE.txt](Go-LICENSE.txt)。不包含 Cadence 软件、库符号或用户设计。
