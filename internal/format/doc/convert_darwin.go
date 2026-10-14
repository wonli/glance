//go:build darwin

package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"glance/internal/format/docx"
)

const textutilPath = "/usr/bin/textutil"

var compoundFileMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

func convertToTempDOCX(ctx context.Context, sourcePath string) (*TempDOCX, error) {
	if ctx == nil {
		return nil, errors.New("DOC import context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open legacy DOC source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("legacy DOC source is not a regular file")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read legacy DOC source: %w", err)
	}
	header := make([]byte, len(compoundFileMagic))
	_, headerErr := io.ReadFull(source, header)
	closeErr := source.Close()
	if headerErr != nil || closeErr != nil || !bytes.Equal(header, compoundFileMagic) {
		return nil, errors.Join(errors.New("legacy .doc source is not a valid OLE Word document"), headerErr, closeErr)
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read DOC native layout: %w", err)
	}
	native, err := nativeImportLayout(sourceBytes)
	if err != nil {
		return nil, fmt.Errorf("read DOC native layout: %w", err)
	}
	if _, err := os.Stat(textutilPath); err != nil {
		return nil, fmt.Errorf("macOS textutil is unavailable for legacy DOC import: %w", err)
	}
	directory, err := os.MkdirTemp("", "glance-doc-import-*")
	if err != nil {
		return nil, fmt.Errorf("create private DOC import directory: %w", err)
	}
	converted := &TempDOCX{Path: filepath.Join(directory, "converted.docx"), directory: directory}
	stagedSource := filepath.Join(directory, "source.doc")
	if err := os.WriteFile(stagedSource, sourceBytes, 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("stage DOC source snapshot: %w", err), converted.Cleanup())
	}
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, textutilPath, "-convert", "docx", "-output", converted.Path, stagedSource)
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		cleanupErr := converted.Cleanup()
		if ctx.Err() != nil {
			return nil, errors.Join(ctx.Err(), cleanupErr)
		}
		message := stderr.String()
		if message == "" {
			message = err.Error()
		}
		return nil, errors.Join(fmt.Errorf("convert legacy DOC to DOCX: %s", message), cleanupErr)
	}
	if err := validateDOCX(converted.Path); err != nil {
		return nil, errors.Join(err, converted.Cleanup())
	}
	if native != nil {
		d, err := docx.Open(converted.Path)
		if err != nil {
			return nil, errors.Join(err, converted.Cleanup())
		}
		recovered, recoveryErr := d.RestoreLegacyImportLayout(*native)
		if recoveryErr == nil && recovered {
			recoveryErr = d.Save()
		}
		closeErr := d.Close()
		if recoveryErr != nil || closeErr != nil {
			return nil, errors.Join(recoveryErr, closeErr, converted.Cleanup())
		}
	}
	return converted, nil
}

func validateDOCX(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("macOS textutil did not produce a valid DOCX package: %w", err)
	}
	defer reader.Close()
	var contentTypes, documentPart bool
	for _, file := range reader.File {
		switch file.Name {
		case "[Content_Types].xml":
			contentTypes = true
		case "word/document.xml":
			documentPart = true
		}
	}
	if !contentTypes || !documentPart {
		return errors.New("macOS textutil produced an incomplete DOCX package")
	}
	return nil
}
