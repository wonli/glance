package pptx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gopptx "github.com/F31/go-pptx/v2/pptx"
	"glance/internal/fileutil"
)

// Save saves the presentation to its original path using an atomic temporary file swap.
func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.path == "" {
		return fmt.Errorf("未指定保存路径，请使用另存为")
	}

	return d.saveAtomicLocked(d.path)
}

// SaveAs saves the presentation to a new path.
func (d *Document) SaveAs(targetPath string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	targetPath = filepath.Clean(targetPath)
	if err := d.saveAtomicLocked(targetPath); err != nil {
		return err
	}

	d.path = targetPath
	return nil
}

func (d *Document) saveAtomicLocked(targetPath string) error {
	expected, err := d.presentationSaveExpectationsLocked()
	if err != nil {
		return err
	}
	slideLayouts, err := d.presentationLayoutOverridesByIndexLocked()
	if err != nil {
		return err
	}
	slideTimings, err := d.presentationAnimationOverridesByIndexLocked()
	if err != nil {
		return err
	}
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpFile, err := fileutil.CreateTemp("glance-pptx-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时保存文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	// Save to temporary file in Glance temp directory
	if _, err := d.pres.Save(context.Background(), tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		return fmt.Errorf("写入演示文稿失败: %w", err)
	}
	validationPath := tmpPath
	var transformedPath string
	var packageEdits map[string][]byte
	if d.presentationComments != nil {
		packageEdits = d.presentationComments.PackageEdits
	}
	if hasPresentationPackageEdits(d.slideTransformChanges, d.slidePictureCropChanges, d.slideStyleChanges, d.slidePlaceholderChanges, d.slideBackgroundChanges, d.slideSizeChange, slideLayouts, slideTimings, packageEdits) {
		transformed, err := fileutil.CreateTemp("glance-pptx-patched-*.tmp")
		if err != nil {
			return fmt.Errorf("创建几何校验文件失败: %w", err)
		}
		transformedPath = transformed.Name()
		_ = transformed.Close()
		defer os.Remove(transformedPath)
		if err := patchPresentationEdits(tmpPath, transformedPath, d.slideTransformChanges, d.slidePictureCropChanges, d.slideStyleChanges, d.slidePlaceholderChanges, d.slideBackgroundChanges, d.slideSizeChange, slideLayouts, slideTimings, packageEdits); err != nil {
			return err
		}
		validationPath = transformedPath
	}
	if err := validateSavedPresentationEdits(validationPath, expected, d.slideStyleChanges, d.slideBackgroundChanges, d.slideSizeChange); err != nil {
		return err
	}
	if err := validatePPTXCommentEdits(validationPath, packageEdits); err != nil {
		return err
	}
	if err := validatePPTXSlideLayoutEdits(validationPath, slideLayouts); err != nil {
		return err
	}
	if err := validatePPTXAnimationEdits(validationPath, slideTimings); err != nil {
		return err
	}

	// Verify reopening before replacement
	checkPres, err := gopptx.Open(validationPath)
	if err != nil {
		return fmt.Errorf("校验保存后的演示文稿失败: %w", err)
	}
	_ = checkPres.Close()

	// Atomic replace
	if err := fileutil.ReplaceFile(validationPath, targetPath); err != nil {
		return fmt.Errorf("替换目标文件失败: %w", err)
	}

	d.dirty = false
	d.revision++
	return nil
}

// SnapshotBytes serializes the presentation to bytes for workspace recovery or draft snapshotting.
func (d *Document) SnapshotBytes() ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.snapshotBytesLocked()
}

func (d *Document) snapshotBytesLocked() ([]byte, error) {
	if d.pres == nil {
		return nil, fmt.Errorf("演示文稿已关闭")
	}
	expected, err := d.presentationSaveExpectationsLocked()
	if err != nil {
		return nil, err
	}
	slideLayouts, err := d.presentationLayoutOverridesByIndexLocked()
	if err != nil {
		return nil, err
	}
	slideTimings, err := d.presentationAnimationOverridesByIndexLocked()
	if err != nil {
		return nil, err
	}

	tmpFile, err := fileutil.CreateTemp("glance-pptx-snap-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建快照临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if _, err := d.pres.Save(context.Background(), tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		return nil, fmt.Errorf("生成演示文稿快照失败: %w", err)
	}
	var packageEdits map[string][]byte
	if d.presentationComments != nil {
		packageEdits = d.presentationComments.PackageEdits
	}
	if !hasPresentationPackageEdits(d.slideTransformChanges, d.slidePictureCropChanges, d.slideStyleChanges, d.slidePlaceholderChanges, d.slideBackgroundChanges, d.slideSizeChange, slideLayouts, slideTimings, packageEdits) {
		if err := validateSavedPresentationEdits(tmpPath, expected, d.slideStyleChanges, d.slideBackgroundChanges, d.slideSizeChange); err != nil {
			return nil, err
		}
		return os.ReadFile(tmpPath)
	}
	patchedFile, err := fileutil.CreateTemp("glance-pptx-snap-patched-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建几何快照文件失败: %w", err)
	}
	patchedPath := patchedFile.Name()
	_ = patchedFile.Close()
	defer os.Remove(patchedPath)
	if err := patchPresentationEdits(tmpPath, patchedPath, d.slideTransformChanges, d.slidePictureCropChanges, d.slideStyleChanges, d.slidePlaceholderChanges, d.slideBackgroundChanges, d.slideSizeChange, slideLayouts, slideTimings, packageEdits); err != nil {
		return nil, err
	}
	if err := validateSavedPresentationEdits(patchedPath, expected, d.slideStyleChanges, d.slideBackgroundChanges, d.slideSizeChange); err != nil {
		return nil, err
	}
	if err := validatePPTXCommentEdits(patchedPath, packageEdits); err != nil {
		return nil, err
	}
	if err := validatePPTXSlideLayoutEdits(patchedPath, slideLayouts); err != nil {
		return nil, err
	}
	if err := validatePPTXAnimationEdits(patchedPath, slideTimings); err != nil {
		return nil, err
	}
	return os.ReadFile(patchedPath)
}

// OpenBytes loads a PPTX presentation from in-memory byte slice.
func OpenBytes(data []byte) (*Document, error) {
	tmpFile, err := fileutil.CreateTemp("glance-pptx-restore-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建恢复临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("写入恢复数据失败: %w", err)
	}
	_ = tmpFile.Close()

	doc, err := Open(tmpPath)
	if err != nil {
		return nil, err
	}
	doc.path = "" // restored drafts have no canonical disk path yet
	return doc, nil
}
