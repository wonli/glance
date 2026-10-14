package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	peSignature             = "PE\x00\x00"
	pe32PlusMagic           = 0x20b
	imageFileMachineAMD64   = 0x8664
	imageScnInitializedData = 0x00000040
	imageScnMemRead         = 0x40000000
	resourceTypeIcon        = 3
	resourceTypeGroupIcon   = 14
)

type iconImage struct {
	width, height, colorCount, reserved byte
	planes, bitCount                    uint16
	data                                []byte
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: embedicon WINDOWS.exe ICON.ico")
		os.Exit(2)
	}
	if err := embed(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func embed(exePath, icoPath string) error {
	exe, err := os.ReadFile(exePath)
	if err != nil {
		return fmt.Errorf("read Windows executable: %w", err)
	}
	icoData, err := os.ReadFile(icoPath)
	if err != nil {
		return fmt.Errorf("read ICO: %w", err)
	}
	icons, err := parseICO(icoData)
	if err != nil {
		return err
	}
	patched, err := addIconResources(exe, icons)
	if err != nil {
		return err
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return fmt.Errorf("stat Windows executable: %w", err)
	}
	tempPath := exePath + ".icon-resource.tmp"
	defer os.Remove(tempPath)
	if err := os.WriteFile(tempPath, patched, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write Windows executable with icon: %w", err)
	}
	if err := os.Rename(tempPath, exePath); err != nil {
		return fmt.Errorf("replace Windows executable with icon resource: %w", err)
	}
	return nil
}

func parseICO(data []byte) ([]iconImage, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, errors.New("invalid ICO header")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 || len(data) < 6+16*count {
		return nil, errors.New("ICO contains no complete image entries")
	}
	icons := make([]iconImage, 0, count)
	for i := 0; i < count; i++ {
		entry := data[6+i*16 : 6+(i+1)*16]
		length := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		end := uint64(offset) + uint64(length)
		if length == 0 || end > uint64(len(data)) {
			return nil, fmt.Errorf("ICO entry %d has an invalid image range", i)
		}
		icons = append(icons, iconImage{
			width: entry[0], height: entry[1], colorCount: entry[2], reserved: entry[3],
			planes:   binary.LittleEndian.Uint16(entry[4:6]),
			bitCount: binary.LittleEndian.Uint16(entry[6:8]),
			data:     append([]byte(nil), data[offset:uint32(end)]...),
		})
	}
	return icons, nil
}

func addIconResources(exe []byte, icons []iconImage) ([]byte, error) {
	if len(icons) > 0xffff-1 {
		return nil, errors.New("ICO has too many image entries")
	}
	parsed, err := pe.NewFile(bytes.NewReader(exe))
	if err != nil {
		return nil, fmt.Errorf("parse Windows executable: %w", err)
	}
	if parsed.FileHeader.Machine != imageFileMachineAMD64 {
		return nil, fmt.Errorf("Windows icon embedding supports amd64 executables, got machine 0x%x", parsed.FileHeader.Machine)
	}
	optional, ok := parsed.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return nil, errors.New("Windows executable does not use the PE32+ optional header")
	}
	if optional.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].VirtualAddress != 0 {
		return nil, errors.New("Windows executable already contains a resource section")
	}
	if optional.SectionAlignment == 0 || optional.FileAlignment == 0 || optional.FileAlignment&(optional.FileAlignment-1) != 0 {
		return nil, errors.New("Windows executable has invalid PE section alignment")
	}
	peOffset, err := peHeaderOffset(exe)
	if err != nil {
		return nil, err
	}
	fileHeaderOffset := peOffset + 4
	optionalOffset := fileHeaderOffset + 20
	optionalSize := int(binary.LittleEndian.Uint16(exe[fileHeaderOffset+16 : fileHeaderOffset+18]))
	sectionTableOffset := optionalOffset + optionalSize
	sectionCount := int(binary.LittleEndian.Uint16(exe[fileHeaderOffset+2 : fileHeaderOffset+4]))
	newHeaderOffset := sectionTableOffset + sectionCount*40
	if uint64(newHeaderOffset+40) > uint64(optional.SizeOfHeaders) {
		return nil, errors.New("Windows executable has no room for an additional PE section header")
	}
	if uint64(sectionTableOffset+sectionCount*40) > uint64(len(exe)) {
		return nil, errors.New("Windows executable has a truncated section table")
	}

	var maxVirtualEnd uint32
	for _, section := range parsed.Sections {
		sectionSize := section.VirtualSize
		if section.Size > sectionSize {
			sectionSize = section.Size
		}
		end := section.VirtualAddress + sectionSize
		if end < section.VirtualAddress {
			return nil, errors.New("Windows executable section address overflow")
		}
		if end > maxVirtualEnd {
			maxVirtualEnd = end
		}
	}
	resourceRVA, ok := align32(maxVirtualEnd, optional.SectionAlignment)
	if !ok {
		return nil, errors.New("Windows executable section address overflow")
	}
	resourceData, err := buildResourceSection(icons, resourceRVA)
	if err != nil {
		return nil, err
	}
	if uint64(len(resourceData)) > 0x7fffffff {
		return nil, errors.New("Windows icon resource section is too large")
	}
	rawOffset, ok := align32(uint32(len(exe)), optional.FileAlignment)
	if !ok {
		return nil, errors.New("Windows executable file offset overflow")
	}
	rawSize, ok := align32(uint32(len(resourceData)), optional.FileAlignment)
	if !ok {
		return nil, errors.New("Windows icon resource size overflow")
	}
	imageEnd, ok := add32(resourceRVA, uint32(len(resourceData)))
	if !ok {
		return nil, errors.New("Windows executable image size overflow")
	}
	imageSize, ok := align32(imageEnd, optional.SectionAlignment)
	if !ok {
		return nil, errors.New("Windows executable image size overflow")
	}
	if uint64(rawOffset)+uint64(rawSize) > uint64(int(^uint(0)>>1)) {
		return nil, errors.New("Windows executable file is too large")
	}

	patched := make([]byte, int(rawOffset)+int(rawSize))
	copy(patched, exe)
	copy(patched[int(rawOffset):], resourceData)
	binary.LittleEndian.PutUint16(patched[fileHeaderOffset+2:fileHeaderOffset+4], uint16(sectionCount+1))
	sectionHeader := patched[newHeaderOffset : newHeaderOffset+40]
	copy(sectionHeader[0:8], []byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	binary.LittleEndian.PutUint32(sectionHeader[8:12], uint32(len(resourceData)))
	binary.LittleEndian.PutUint32(sectionHeader[12:16], resourceRVA)
	binary.LittleEndian.PutUint32(sectionHeader[16:20], rawSize)
	binary.LittleEndian.PutUint32(sectionHeader[20:24], rawOffset)
	binary.LittleEndian.PutUint32(sectionHeader[36:40], imageScnInitializedData|imageScnMemRead)
	optionalBytes := patched[optionalOffset : optionalOffset+optionalSize]
	binary.LittleEndian.PutUint32(optionalBytes[8:12], optional.SizeOfInitializedData+rawSize)
	binary.LittleEndian.PutUint32(optionalBytes[56:60], imageSize)
	binary.LittleEndian.PutUint32(optionalBytes[64:68], 0)
	resourceDirectoryOffset := 112 + pe.IMAGE_DIRECTORY_ENTRY_RESOURCE*8
	if len(optionalBytes) < resourceDirectoryOffset+8 || optional.NumberOfRvaAndSizes <= pe.IMAGE_DIRECTORY_ENTRY_RESOURCE {
		return nil, errors.New("Windows executable has no resource data directory")
	}
	binary.LittleEndian.PutUint32(optionalBytes[resourceDirectoryOffset:resourceDirectoryOffset+4], resourceRVA)
	binary.LittleEndian.PutUint32(optionalBytes[resourceDirectoryOffset+4:resourceDirectoryOffset+8], uint32(len(resourceData)))
	if _, err := pe.NewFile(bytes.NewReader(patched)); err != nil {
		return nil, fmt.Errorf("validate Windows executable after embedding icon: %w", err)
	}
	return patched, nil
}

func buildResourceSection(icons []iconImage, resourceRVA uint32) ([]byte, error) {
	if len(icons) == 0 {
		return nil, errors.New("cannot embed an ICO with no images")
	}
	rootOffset := 0
	iconTypeOffset := 16 + 2*8
	iconTypeSize := 16 + len(icons)*8
	groupTypeOffset := iconTypeOffset + iconTypeSize
	groupTypeSize := 16 + 8
	nameDirectoriesOffset := groupTypeOffset + groupTypeSize
	nameDirectorySize := 16 + 8
	dataEntriesOffset := nameDirectoriesOffset + (len(icons)+1)*nameDirectorySize
	dataEntriesSize := (len(icons) + 1) * 16
	payloadOffset := alignInt(dataEntriesOffset+dataEntriesSize, 4)
	payloadOffsets := make([]int, 0, len(icons)+1)
	for _, icon := range icons {
		payloadOffset = alignInt(payloadOffset, 4)
		payloadOffsets = append(payloadOffsets, payloadOffset)
		payloadOffset += len(icon.data)
	}
	groupData, err := buildGroupIcon(icons)
	if err != nil {
		return nil, err
	}
	payloadOffset = alignInt(payloadOffset, 4)
	payloadOffsets = append(payloadOffsets, payloadOffset)
	payloadOffset += len(groupData)
	if uint64(payloadOffset) > 0x7fffffff || uint64(resourceRVA)+uint64(payloadOffset) > 0xffffffff {
		return nil, errors.New("Windows icon resource section exceeds PE limits")
	}
	resource := make([]byte, payloadOffset)
	writeDirectoryHeader(resource, rootOffset, 2)
	writeDirectoryEntry(resource, rootOffset+16, resourceTypeIcon, 0x80000000|uint32(iconTypeOffset))
	writeDirectoryEntry(resource, rootOffset+24, resourceTypeGroupIcon, 0x80000000|uint32(groupTypeOffset))
	writeDirectoryHeader(resource, iconTypeOffset, uint16(len(icons)))
	for i := range icons {
		nameDirectoryOffset := nameDirectoriesOffset + i*nameDirectorySize
		writeDirectoryEntry(resource, iconTypeOffset+16+i*8, uint16(i+1), 0x80000000|uint32(nameDirectoryOffset))
		writeLanguageDirectory(resource, nameDirectoryOffset, dataEntriesOffset+i*16)
		writeResourceDataEntry(resource, dataEntriesOffset+i*16, resourceRVA, payloadOffsets[i], len(icons[i].data))
		copy(resource[payloadOffsets[i]:], icons[i].data)
	}
	writeDirectoryHeader(resource, groupTypeOffset, 1)
	groupNameDirectoryOffset := nameDirectoriesOffset + len(icons)*nameDirectorySize
	writeDirectoryEntry(resource, groupTypeOffset+16, 1, 0x80000000|uint32(groupNameDirectoryOffset))
	groupDataEntryOffset := dataEntriesOffset + len(icons)*16
	writeLanguageDirectory(resource, groupNameDirectoryOffset, groupDataEntryOffset)
	writeResourceDataEntry(resource, groupDataEntryOffset, resourceRVA, payloadOffsets[len(icons)], len(groupData))
	copy(resource[payloadOffsets[len(icons)]:], groupData)
	return resource, nil
}

func buildGroupIcon(icons []iconImage) ([]byte, error) {
	if len(icons) == 0 || len(icons) > 0xffff {
		return nil, errors.New("invalid icon group size")
	}
	group := make([]byte, 6+len(icons)*14)
	binary.LittleEndian.PutUint16(group[2:4], 1)
	binary.LittleEndian.PutUint16(group[4:6], uint16(len(icons)))
	for i, icon := range icons {
		entry := group[6+i*14 : 6+(i+1)*14]
		entry[0], entry[1], entry[2], entry[3] = icon.width, icon.height, icon.colorCount, icon.reserved
		binary.LittleEndian.PutUint16(entry[4:6], icon.planes)
		binary.LittleEndian.PutUint16(entry[6:8], icon.bitCount)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(icon.data)))
		binary.LittleEndian.PutUint16(entry[12:14], uint16(i+1))
	}
	return group, nil
}

func writeLanguageDirectory(data []byte, offset, dataEntryOffset int) {
	writeDirectoryHeader(data, offset, 1)
	writeDirectoryEntry(data, offset+16, 0, uint32(dataEntryOffset))
}

func writeDirectoryHeader(data []byte, offset int, idEntries uint16) {
	binary.LittleEndian.PutUint16(data[offset+14:offset+16], idEntries)
}

func writeDirectoryEntry(data []byte, offset int, id uint16, target uint32) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(id))
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], target)
}

func writeResourceDataEntry(data []byte, offset int, resourceRVA uint32, payloadOffset, payloadSize int) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], resourceRVA+uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], uint32(payloadSize))
}

func peHeaderOffset(data []byte) (int, error) {
	if len(data) < 0x40 || string(data[0:2]) != "MZ" {
		return 0, errors.New("invalid DOS header in Windows executable")
	}
	offset := int(binary.LittleEndian.Uint32(data[0x3c:0x40]))
	if offset < 0 || offset+4 > len(data) || string(data[offset:offset+4]) != peSignature {
		return 0, errors.New("invalid PE signature in Windows executable")
	}
	if offset+24 > len(data) {
		return 0, errors.New("truncated PE file header")
	}
	if binary.LittleEndian.Uint16(data[offset+4:offset+6]) != imageFileMachineAMD64 {
		return 0, fmt.Errorf("expected amd64 Windows executable, got machine 0x%x", binary.LittleEndian.Uint16(data[offset+4:offset+6]))
	}
	if offset+24+int(binary.LittleEndian.Uint16(data[offset+20:offset+22])) > len(data) {
		return 0, errors.New("truncated PE optional header")
	}
	if binary.LittleEndian.Uint16(data[offset+24:offset+26]) != pe32PlusMagic {
		return 0, errors.New("Windows executable is not PE32+")
	}
	return offset, nil
}

func align32(value, alignment uint32) (uint32, bool) {
	if alignment == 0 || alignment&(alignment-1) != 0 || value > ^uint32(0)-(alignment-1) {
		return 0, false
	}
	return (value + alignment - 1) & ^(alignment - 1), true
}

func alignInt(value, alignment int) int {
	return (value + alignment - 1) & ^(alignment - 1)
}

func add32(left, right uint32) (uint32, bool) {
	result := left + right
	return result, result >= left
}
