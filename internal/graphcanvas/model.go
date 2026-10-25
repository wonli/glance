package graphcanvas

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
)

// Graph represents a large-scale graph stored in flat, contiguous slices
// to prevent GC overhead and pointer chasing.
type Graph struct {
	NodeCount int
	X         []float32
	Y         []float32
	Radius    []float32
	Color     []color.NRGBA
	Cluster   []int32
	Label     []string

	EdgeCount int
	EdgeSrc   []int32
	EdgeDst   []int32

	// HubIDs stores the indices of major cluster hub nodes
	HubIDs []int32

	// Adj stores neighbor node indices for each node.
	Adj [][]int32
}

// ChineseDomainTopics provides rich domain labels for knowledge clusters.
var ChineseDomainTopics = []string{
	"大语言模型架构", "深度强化学习", "计算机视觉前沿", "自然语言理解",
	"分布式共识系统", "图神经网络计算", "微服务服务网格", "Linux内核调度",
	"数据库ACID引擎", "知识图谱推理", "多模态扩散模型", "向量检索数据库",
	"量子计算算法", "高并发异步IO", "WebGPU渲染管线", "编译器与IR优化",
	"密码学与零知识证明", "可观测性监控", "领域驱动设计", "神经拟态计算",
	"容器编排K8s", "实时流计算Flink", "GPU通用计算CUDA", "复杂网络动力学",
	"具身智能机器人", "自动驾驶感知", "因果关系推理", "低秩适配LoRA",
	"边缘计算协同", "软件架构演进", "事件驱动架构", "自监督对比学习",
}

// SubEntityKeywords provides detailed terminology for knowledge entities.
var SubEntityKeywords = []string{
	"注意力机制", "旋转位置编码", "张量并行切分", "流水线并行", "模型量化压缩",
	"缓存穿透防护", "一致性哈希环", "读写分离路由", "倒排索引加速", "零拷贝数据通道",
	"虚拟内存映射", "轻量协程调度", "内存顺序屏障", "无锁环形队列", "动态剪枝优化",
	"奇异值分解SVD", "梯度范数裁剪", "自适应动量优化", "损失函数曲面", "超参数自动调优",
	"动态规划转移", "有向无环拓扑", "强连通分量Tarjan", "最小生成树Kruskal", "网络流最大流",
	"哈希冲突二次探测", "红黑树旋转平衡", "B+树节点分裂", "LSM分层压缩合并", "布隆过滤器哈希",
}
// ClusterPalette contains visually pleasing palette colors for clusters.
var ClusterPalette = []color.NRGBA{
	{R: 0x4F, G: 0x86, B: 0xF7, A: 0xFF}, // Electric Blue
	{R: 0x9B, G: 0x51, B: 0xE0, A: 0xFF}, // Amethyst Purple
	{R: 0x00, G: 0xB8, B: 0x94, A: 0xFF}, // Mint Teal
	{R: 0xFD, G: 0xCB, B: 0x6E, A: 0xFF}, // Warm Amber
	{R: 0xE1, G: 0x70, B: 0x55, A: 0xFF}, // Terracotta Coral
	{R: 0x09, G: 0x84, B: 0xE3, A: 0xFF}, // Cyan Blue
	{R: 0x6C, G: 0x5C, B: 0xE7, A: 0xFF}, // Soft Indigo
	{R: 0x00, G: 0xCE, B: 0xC9, A: 0xFF}, // Turquoise
	{R: 0xFA, G: 0x82, B: 0x31, A: 0xFF}, // Vivid Tangerine
	{R: 0xA2, G: 0x9B, B: 0xFE, A: 0xFF}, // Lavender
	{R: 0x26, G: 0xDE, B: 0x81, A: 0xFF}, // Emerald Green
	{R: 0xEB, G: 0x3B, B: 0x5A, A: 0xFF}, // Crimson Rose
}

// GenerateBenchmarkGraph creates a realistic synthetic knowledge graph
// with cluster topology, hub nodes, and bridge edges.
func GenerateBenchmarkGraph(nodeCount, clusterCount int, seed int64) *Graph {
	if nodeCount <= 0 {
		nodeCount = 100000
	}
	if clusterCount <= 0 {
		clusterCount = 120
	}

	r := rand.New(rand.NewSource(seed))

	g := &Graph{
		NodeCount: nodeCount,
		X:         make([]float32, nodeCount),
		Y:         make([]float32, nodeCount),
		Radius:    make([]float32, nodeCount),
		Color:     make([]color.NRGBA, nodeCount),
		Cluster:   make([]int32, nodeCount),
		Label:     make([]string, nodeCount),
		Adj:       make([][]int32, nodeCount),
	}

	// Generate cluster centers
	type clusterCenter struct {
		x, y   float32
		color  color.NRGBA
		radius float32
		hubID  int32
	}
	clusters := make([]clusterCenter, clusterCount)
	worldSpread := float32(5000.0)

	for i := 0; i < clusterCount; i++ {
		angle := r.Float64() * 2 * math.Pi
		dist := math.Sqrt(r.Float64()) * float64(worldSpread)
		col := ClusterPalette[i%len(ClusterPalette)]

		clusters[i] = clusterCenter{
			x:      float32(math.Cos(angle) * dist),
			y:      float32(math.Sin(angle) * dist),
			color:  col,
			radius: float32(350.0 + r.Float64()*450.0),
			hubID:  -1,
		}
	}

	// Step 1: Assign nodes to clusters
	nodesPerCluster := nodeCount / clusterCount
	for i := 0; i < nodeCount; i++ {
		cIdx := i / nodesPerCluster
		if cIdx >= clusterCount {
			cIdx = clusterCount - 1
		}
		c := clusters[cIdx]

		isHub := false
		if clusters[cIdx].hubID == -1 {
			// First node in cluster is the hub
			clusters[cIdx].hubID = int32(i)
			isHub = true
		}

		domainName := ChineseDomainTopics[cIdx%len(ChineseDomainTopics)]

		if isHub {
			g.X[i] = c.x
			g.Y[i] = c.y
			g.Radius[i] = 20.0
			g.Color[i] = c.color
			g.Label[i] = fmt.Sprintf("【%s】领域核心", domainName)
			g.HubIDs = append(g.HubIDs, int32(i))
		} else {
			// Gaussian distribution around cluster center
			u1 := r.Float64()
			u2 := r.Float64()
			if u1 < 1e-6 {
				u1 = 1e-6
			}
			z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
			z1 := math.Sqrt(-2.0*math.Log(u1)) * math.Sin(2.0*math.Pi*u2)

			spread := float64(c.radius) * 0.65
			g.X[i] = c.x + float32(z0*spread)
			g.Y[i] = c.y + float32(z1*spread)
			g.Radius[i] = float32(4.5 + r.Float64()*3.0)

			// Slightly vary color brightness
			brightness := float32(0.85 + r.Float64()*0.3)
			g.Color[i] = scaleColor(c.color, brightness)

			kw := SubEntityKeywords[i%len(SubEntityKeywords)]
			g.Label[i] = fmt.Sprintf("%s · %s", domainName, kw)
		}
		g.Cluster[i] = int32(cIdx)
	}

	// Step 2: Build edges (approx. 1.5 - 2 edges per node)
	// Intra-cluster links: connect to hub or near neighbors
	estEdges := int(float64(nodeCount) * 1.6)
	g.EdgeSrc = make([]int32, 0, estEdges)
	g.EdgeDst = make([]int32, 0, estEdges)

	addEdge := func(u, v int32) {
		if u == v {
			return
		}
		g.EdgeSrc = append(g.EdgeSrc, u)
		g.EdgeDst = append(g.EdgeDst, v)
		g.Adj[u] = append(g.Adj[u], v)
		g.Adj[v] = append(g.Adj[v], u)
	}

	for i := 0; i < nodeCount; i++ {
		cIdx := int(g.Cluster[i])
		hub := clusters[cIdx].hubID

		// Connect to hub with 40% probability
		if int32(i) != hub && r.Float64() < 0.40 {
			addEdge(int32(i), hub)
		}

		// Connect to a nearby neighbor in the same cluster
		neighborOffset := int(r.Int31n(15)) - 7
		target := i + neighborOffset
		if target >= 0 && target < nodeCount && target != i && g.Cluster[target] == int32(cIdx) {
			addEdge(int32(i), int32(target))
		}

		// Connect to a bridge node in another random cluster (small world)
		if r.Float64() < 0.05 {
			otherCluster := r.Intn(clusterCount)
			if otherCluster != cIdx {
				otherHub := clusters[otherCluster].hubID
				if otherHub >= 0 {
					addEdge(int32(i), otherHub)
				}
			}
		}
	}

	g.EdgeCount = len(g.EdgeSrc)
	return g
}

func scaleColor(c color.NRGBA, s float32) color.NRGBA {
	clamp := func(val float32) uint8 {
		if val < 0 {
			return 0
		}
		if val > 255 {
			return 255
		}
		return uint8(val)
	}
	return color.NRGBA{
		R: clamp(float32(c.R) * s),
		G: clamp(float32(c.G) * s),
		B: clamp(float32(c.B) * s),
		A: c.A,
	}
}
