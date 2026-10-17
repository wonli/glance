package desktopui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestSchemeAVerification(t *testing.T) {
	fmt.Println("\n=======================================================")
	fmt.Println("=== 验证 1: 检查 ~/Downloads/100mb.docx ===")
	fmt.Println("=======================================================")
	home, _ := os.UserHomeDir()
	p100mb := filepath.Join(home, "Downloads", "100mb.docx")
	info, err := os.Stat(p100mb)
	if err != nil {
		fmt.Printf("无法访问 %s: %v\n", p100mb, err)
	} else {
		fmt.Printf("文件大小: %d 字节 (%.2f MB)\n", info.Size(), float64(info.Size())/(1024*1024))
		buf := make([]byte, 256)
		f, _ := os.Open(p100mb)
		n, _ := f.Read(buf)
		f.Close()
		fmt.Printf("文件前 %d 字节内容摘要:\n%s\n", n, string(buf[:n]))
		fmt.Printf("尝试通过 format.Open 打开:\n")
		doc, err := docx.Open(p100mb)
		if err != nil {
			fmt.Printf(">> 打开结果: 格式校验失败（符合预期，因为下载的测试文件是非 ZIP 的纯文本伪装文件）: %v\n", err)
		} else {
			doc.Close()
			fmt.Println(">> 打开结果: 成功")
		}
	}

	fmt.Println("\n=======================================================")
	fmt.Println("=== 验证 2: 方案A (两阶段渐进式首屏快出) 真实多页文档测试 ===")
	fmt.Println("=======================================================")
	realDocs := []string{
		filepath.Join(home, "Downloads", "成都市卫生健康委员会-安全隐患管理系统-建设方案-230812.docx"),
		filepath.Join(home, "Downloads", "AI大语言模型应用系统建设方案.docx"),
		filepath.Join(home, "Downloads", "office", "大猫OA软件使用说明.docx"),
	}

	shaper, err := newWordTextShaper()
	if err != nil {
		t.Fatalf("初始化 text shaper 失败: %v", err)
	}
	engine := wordlayout.NewEngine(shaper)

	for _, docPath := range realDocs {
		if _, err := os.Stat(docPath); err != nil {
			continue
		}
		runProgressiveVerification(t, docPath, engine)
	}
}

func runProgressiveVerification(t *testing.T, path string, engine *wordlayout.Engine) {
	base := filepath.Base(path)
	info, _ := os.Stat(path)
	fmt.Printf("\n--- 测试文档: %s (%.2f MB) ---\n", base, float64(info.Size())/(1024*1024))

	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)

	t0 := time.Now()
	doc, err := docx.Open(path)
	if err != nil {
		fmt.Printf("打开文档失败: %v\n", err)
		return
	}
	defer doc.Close()
	openDuration := time.Since(t0)

	blocks := doc.WordBlocks()
	fmt.Printf("解析耗时: %v, 共 %d 个正文数据块\n", openDuration, len(blocks))

	layoutData := doc.WordLayoutData()

	var phase1Time time.Duration
	var phase1Pages int
	var streamUpdates []string

	input := wordlayout.Input{
		DocumentID:       base,
		Revision:         1,
		Blocks:           blocks,
		Layout:           layoutData,
		DefaultPageSetup: wordlayout.DefaultPageSetup(),
		ProgressiveCallback: func(partial wordlayout.Result) {
			now := time.Since(t0)
			if phase1Time == 0 {
				phase1Time = now
				phase1Pages = len(partial.Pages)
				streamUpdates = append(streamUpdates, fmt.Sprintf("阶段1（首屏2页快出）: %v (已排 %d 页，UI已可渲染首屏)", phase1Time, phase1Pages))
			} else {
				streamUpdates = append(streamUpdates, fmt.Sprintf("流式阶段2更新: %v (已排 %d 页)", now, len(partial.Pages)))
			}
		},
	}

	layoutStart := time.Now()
	result, err := engine.Layout(context.Background(), input)
	totalLayoutTime := time.Since(layoutStart)
	if err != nil {
		fmt.Printf("排版失败: %v\n", err)
		return
	}

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	for _, u := range streamUpdates {
		fmt.Println("  -> " + u)
	}
	fmt.Printf("排版全部完成: 总耗时 %v, 共 %d 页\n", totalLayoutTime, len(result.Pages))
	fmt.Printf("首屏到达耗时: %v (用户可见前两页的时间)\n", phase1Time)
	fmt.Printf("内存分配变化: Alloc=%+.2f MB, HeapInuse=%+.2f MB, Sys=%+.2f MB\n",
		(float64(m1.Alloc)-float64(m0.Alloc))/(1024*1024),
		(float64(m1.HeapInuse)-float64(m0.HeapInuse))/(1024*1024),
		(float64(m1.Sys)-float64(m0.Sys))/(1024*1024))
}
