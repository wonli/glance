package pptx

import (
	"context"
	"fmt"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

func (d *Document) AddTableByID(ctx context.Context, slideID uint32, bounds document.PresentationRect, table document.PresentationTableData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddTable(ctx, gopptx.TableSpec{
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H),
			Rows: table.Rows, FirstRow: table.FirstRow, BandRow: table.BandRows,
		})
		if err != nil {
			return 0, fmt.Errorf("新建表格失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) AddChartByID(ctx context.Context, slideID uint32, bounds document.PresentationRect, chart document.PresentationChartData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	chartType := gopptx.ChartBar
	switch chart.Type {
	case "bar":
		chartType = gopptx.ChartBar
	case "line":
		chartType = gopptx.ChartLine
	case "pie":
		chartType = gopptx.ChartPie
	default:
		return 0, fmt.Errorf("暂不支持此图表类型: %q", chart.Type)
	}
	series := make([]gopptx.ChartSeries, len(chart.Series))
	for index, item := range chart.Series {
		series[index] = gopptx.ChartSeries{Name: item.Name, Values: append([]float64(nil), item.Values...)}
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddChart(ctx, gopptx.ChartSpec{
			Type: chartType, Title: chart.Title,
			Categories: append([]string(nil), chart.Categories...), Series: series,
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H),
		})
		if err != nil {
			return 0, fmt.Errorf("新建图表失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) AddAudioByID(ctx context.Context, slideID uint32, bounds document.PresentationRect, audio document.PresentationAudioData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	role := gopptx.AudioRoleEffect
	switch audio.Role {
	case "narration":
		role = gopptx.AudioRoleNarration
	case "background":
		role = gopptx.AudioRoleBackground
	case "effect":
	default:
		return 0, fmt.Errorf("暂不支持此音频角色: %q", audio.Role)
	}
	if audio.TrackKey == "" || len(audio.Bytes) == 0 {
		return 0, fmt.Errorf("音频标识和文件内容不能为空")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddAudio(ctx, gopptx.BytesMedia(audio.Bytes, audio.MediaType), gopptx.AudioSpec{
			TrackKey: audio.TrackKey, Role: role,
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H),
		})
		if err != nil {
			return 0, fmt.Errorf("嵌入音频失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) AddVideoByID(ctx context.Context, slideID uint32, bounds document.PresentationRect, video document.PresentationVideoData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	if video.TrackKey == "" || len(video.Bytes) == 0 {
		return 0, fmt.Errorf("视频标识和文件内容不能为空")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddVideo(ctx, gopptx.BytesMedia(video.Bytes, video.MediaType), gopptx.VideoSpec{
			TrackKey: video.TrackKey, Role: gopptx.VideoRoleMain,
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H),
			AltText: video.AltText, IsDecorative: video.Decorative,
		})
		if err != nil {
			return 0, fmt.Errorf("嵌入视频失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}
