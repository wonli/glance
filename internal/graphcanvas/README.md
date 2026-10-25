# Glance GraphCanvas 内核引擎

`internal/graphcanvas` 是专为 Glance 桌面办公套件设计打造的**超大规模高性能知识图谱与无限画布交互引擎**。基于 Gio UI 原生 GPU 渲染管线构建，实测可在普通消费级设备上流畅支撑 **100,000+ 节点与 150,000+ 关系连线**以 60~120 FPS 运行。

---

## 架构核心与关键技术

### 1. 数据导向扁平内存模型 (Data-Oriented SOA)
- **零指针追踪与零 GC 压力**：`Graph` 结构体摒弃了面向对象的指针嵌套，采用扁平化连续切片结构（Structure of Arrays，SOA）：
  ```go
  type Graph struct {
      NodeCount int
      X, Y      []float32      // 坐标
      Radius    []float32      // 半径
      Color     []color.NRGBA  // 颜色
      Cluster   []int32        // 分类簇
      Label     []string       // 实体标签
      EdgeCount int
      EdgeSrc   []int32        // 边起点
      EdgeDst   []int32        // 边终点
      Adj       [][]int32      // 邻接表
  }
  ```
- 10 万节点全部数据在内存中紧凑排列，内存分配仅约 110 MB，几乎不触发 Go 垃圾回收停顿。

### 2. 空间网格剔除与增量宏录制 (Spatial Grid & Macro Caching)
- **$32 \times 32$ 均匀空间网格**：将世界坐标范围划分为 1024 个分块（Chunk），镜头平移缩放时基于视锥 AABB 快速视见体裁剪。
- **持久化宏指令缓存 (`op.Ops` + `op.CallOp`)**：未发生几何变动的分块仅在初次或 LOD 切换时录制一次，后续帧直接重放 GPU 绘制宏指令。
- **$O(1)$ 动态热重排 (`MoveNode`)**：维护 `NodeChunk []int32` 直接查找表。当节点被用户拖拽或物理位移跨越分块边界时，算法在毫秒内将节点移出旧块并推入新块，彻底根绝“幽灵节点”或坐标错位。

### 3. 多级视界细节金字塔 (Quad-LOD Visual Pyramid)
根据镜头缩放比例自适应切换渲染策略：
| LOD 级别 | 缩放倍率 (Zoom) | 渲染策略 |
| :--- | :--- | :--- |
| **LODOverview** | $z < 0.25$ | 预烘焙 2048 位图星系底图 + 核心领域 Hub 矢量大球 |
| **LODFar** | $0.25 \le z < 0.60$ | 中景聚类骨架，每块抽样渲染 8 个核心骨干节点与主要关系连线 |
| **LODMedium** | $0.60 \le z < 1.40$ | 次级展开，每块渲染 22 个节点，展现中等密集度拓扑 |
| **LODDetail** | $z \ge 1.40$ | 全量矢量圆球展开，高对比屏幕空间防重叠标签胶囊完整浮现 |

### 4. 局部弹簧力导向与微动呼吸 (Local Active Physics)
- **局部激活集合**：仅对用户点击或拖拽的节点激活其 1~2 度直接邻居（上限 36 节点、45 关系边），计算开销从全局 $O(N^2)$ 骤降至局部 $O(1)$。
- **微幅有机呼吸**：节点被点击聚焦后维持 $\pm 2.5$ 像素内的细微谐波拉扯与微动波动，呈现生命呼吸感而不破坏整体拓扑。
- **自然就地停留**：用户失焦或释放拖拽时，通过 `StopBreathing()` 平滑阻尼就地沉降于目标点 B，决不生硬瞬移弹回，且空间索引实时同步保持 100% 可再次点选。

### 5. 屏幕空间防重叠标签 (Screen-Space Collision-Free Labels)
- 标签以物理像素在屏幕空间绘制，保证无论镜头缩放多大，文字始终保持高清锐利。
- 4 向候选定位（右 $\to$ 左 $\to$ 上 $\to$ 下）配合屏幕 AABB 重叠碰撞检测与视口边界硬裁剪。
- 渲染优先级：当前聚焦选中节点 > 鼠标悬停高亮节点 > 局部活跃邻居 > 核心领域 Hub。

---

## 快速使用指南

### 1. 构造图谱数据
```go
package main

import (
    "image/color"
    "glance/internal/graphcanvas"
)

func createDemoGraph() *graphcanvas.Graph {
    g := &graphcanvas.Graph{
        NodeCount: 3,
        X:         []float32{100, 300, 200},
        Y:         []float32{100, 100, 280},
        Radius:    []float32{12, 12, 16},
        Color: []color.NRGBA{
            {R: 0x4F, G: 0x86, B: 0xF7, A: 0xFF}, // Word 蓝
            {R: 0x00, G: 0xB8, B: 0x94, A: 0xFF}, // Excel 绿
            {R: 0xE1, G: 0x70, B: 0x55, A: 0xFF}, // PPTX 橙红
        },
        Label: []string{"项目规划.docx", "财务预算.xlsx", "年度发布会.pptx"},
        Adj:   make([][]int32, 3),
    }

    // 添加关系边: 0 - 1, 1 - 2, 0 - 2
    g.EdgeSrc = []int32{0, 1, 0}
    g.EdgeDst = []int32{1, 2, 2}
    g.EdgeCount = len(g.EdgeSrc)

    g.Adj[0] = []int32{1, 2}
    g.Adj[1] = []int32{0, 2}
    g.Adj[2] = []int32{0, 1}
    g.HubIDs = []int32{2}

    return g
}
```

### 2. 在 Gio 窗口或视图中布局渲染
```go
type KnowledgeView struct {
    canvas *graphcanvas.Canvas
}

func NewKnowledgeView(g *graphcanvas.Graph) *KnowledgeView {
    return &KnowledgeView{
        canvas: graphcanvas.NewCanvas(g),
    }
}

func (kv *KnowledgeView) Layout(gtx layout.Context) layout.Dimensions {
    // 渲染图谱并自动处理漫游平移、滚轮缩放、单选呼吸、节点拖拽与悬停
    return kv.canvas.Layout(gtx)
}
```

---

## 办公套件 (Glance) 知识库整合最佳实践

### 1. 文档类型与品牌色映射
将 Glance 已支持的文档类型映射到知识星系中：
| 文档类型 | 节点视觉 | 推荐色彩 | 知识实体角色 |
| :--- | :--- | :--- | :--- |
| **Word (`.docx`, `.doc`)** | 蓝色光环 | `color.NRGBA{R: 0x2B, G: 0x57, B: 0x9A, A: 0xFF}` | 核心长篇需求、合同规章、综述报告 |
| **Excel (`.xlsx`, `.xls`, `.csv`)** | 翠绿色光环 | `color.NRGBA{R: 0x10, G: 0x7C, B: 0x41, A: 0xFF}` | 结构化数据源、报表台账、指标字典 |
| **PowerPoint (`.pptx`)** | 暖橙色光环 | `color.NRGBA{R: 0xD2, G: 0x47, B: 0x26, A: 0xFF}` | 演示胶囊、宣讲方案、汇报摘要 |
| **PDF (`.pdf`)** | 鲜红色光环 | `color.NRGBA{R: 0xE0, G: 0x2B, B: 0x20, A: 0xFF}` | 外部归档、扫描件单据、不可变终稿 |
| **Markdown (`.md`)** | 紫色光环 | `color.NRGBA{R: 0x9B, G: 0x51, B: 0xE0, A: 0xFF}` | 随笔卡片、双链知识卡、速记备忘 |

### 2. 关系拓扑的自然生长
知识库的关系连线无需用户手动绘制，可通过 Glance 现有内部组件自动生成：
1. **显式双向链接**：解析 Markdown 或 DOCX 批注中的 `[[文档名]]` 或超链接。
2. **工作区目录与项目聚类**：复用 `internal/workspace`，同一文件夹或项目归属下的文档自动形成星系引力簇。
3. **AI 语义关联提取**：结合 Glance 已有的 `internal/agent` 与 `internal/proofreading` 模块，提取文档关键词生成隐式主题关联边。

### 3. “看图 $\to$ 聚焦 $\to$ 抽屉预览 $\to$ 原生编辑” 闭环
- **单选节点**：触发轻微呼吸微动，并在屏幕右侧滑出 Glance 文档抽屉面板（显示字数、最近编辑者、关联文件列表）。
- **双击节点**：直接调度 Glance 原生文档控制器，以全屏/分屏模式进入该 Word/Excel/PPT/Markdown 的原生编辑状态。
- **全局搜索寻路**：顶栏输入关键词后，调用 `Camera.FlyTo(targetX, targetY, targetZoom)` 平滑飞行动画聚焦目标文档。
