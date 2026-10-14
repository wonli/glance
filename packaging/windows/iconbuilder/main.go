package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: iconbuilder SOURCE.png OUTPUT.ico")
		os.Exit(2)
	}
	if err := buildICO(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildICO(sourcePath, outputPath string) error {
	input, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open icon PNG: %w", err)
	}
	source, err := png.Decode(input)
	closeErr := input.Close()
	if err != nil {
		return fmt.Errorf("decode icon PNG: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close icon PNG: %w", closeErr)
	}
	bounds := source.Bounds()
	if bounds.Dx() != bounds.Dy() || bounds.Dx() == 0 {
		return fmt.Errorf("icon PNG must be square, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	images := make([][]byte, 0, len(iconSizes))
	for _, size := range iconSizes {
		resized := image.NewNRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), source, bounds, draw.Over, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, resized); err != nil {
			return fmt.Errorf("encode %dx%d icon: %w", size, size, err)
		}
		images = append(images, encoded.Bytes())
	}
	return writeICO(outputPath, iconSizes, images)
}

func writeICO(path string, sizes []int, images [][]byte) error {
	if len(sizes) == 0 || len(sizes) != len(images) || len(sizes) > 0xffff {
		return errors.New("invalid ICO image set")
	}
	const headerSize = 6
	const entrySize = 16
	dataOffset := headerSize + len(sizes)*entrySize
	var output bytes.Buffer
	write16 := func(value uint16) { _ = binary.Write(&output, binary.LittleEndian, value) }
	write32 := func(value uint32) { _ = binary.Write(&output, binary.LittleEndian, value) }
	write16(0)
	write16(1)
	write16(uint16(len(sizes)))
	for i, size := range sizes {
		if size <= 0 || size > 256 || len(images[i]) == 0 {
			return fmt.Errorf("invalid ICO entry at index %d", i)
		}
		width := byte(size)
		if size == 256 {
			width = 0
		}
		output.WriteByte(width)
		output.WriteByte(width)
		output.WriteByte(0)
		output.WriteByte(0)
		write16(1)
		write16(32)
		write32(uint32(len(images[i])))
		write32(uint32(dataOffset))
		dataOffset += len(images[i])
	}
	for _, imageData := range images {
		_, _ = output.Write(imageData)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create icon output directory: %w", err)
	}
	if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write ICO: %w", err)
	}
	return nil
}
