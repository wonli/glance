package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

type presentationElementVideoCreatePayload struct {
	SlideId    uint32 `json:"slide_id"`
	SourcePath string `json:"source_path"`
	MediaType  string `json:"media_type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	AltText    string `json:"alt_text"`
}

type presentationElementAudioCreatePayload struct {
	SlideId    uint32 `json:"slide_id"`
	SourcePath string `json:"source_path"`
	MediaType  string `json:"media_type"`
	Role       string `json:"role"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

type testPresentationmediaSlideIdSourcePathArguments struct {
	SlideId    uint32 `json:"slide_id"`
	SourcePath string `json:"source_path"`
	MediaType  string `json:"media_type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

type testPresentationmediaSlideIdSourcePathMediaTypeArguments struct {
	SlideId    uint32 `json:"slide_id"`
	SourcePath string `json:"source_path"`
	MediaType  string `json:"media_type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Decorative bool   `json:"decorative"`
	AltText    string `json:"alt_text"`
}

type testPresentationmediaSlideIdSourcePathMediaTypeXArguments struct {
	SlideId    uint32 `json:"slide_id"`
	SourcePath string `json:"source_path"`
	MediaType  string `json:"media_type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

func TestPresentationMediaCapabilitiesValidateAndEmbedExplicitFiles(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect new presentation: %v", err)
	}
	var inspected struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &inspected); err != nil || len(inspected.Slides) != 1 || inspected.Slides[0].ID == 0 {
		t.Fatalf("new presentation inspection=%s error=%v", inspection.Content, err)
	}
	slideID := inspected.Slides[0].ID
	operator := Operator{ID: "pptx-media-agent", Kind: OperatorExternalAgent}
	revision := doc.Revision
	requestNumber := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("pptx-media-%d", requestNumber), DocumentID: doc.DocumentID,
			CapabilityID: capabilityID, ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, args),
		})
		if invokeErr == nil {
			revision = result.Revision
		}
		return result, invokeErr
	}
	info := func() DocumentInfo {
		t.Helper()
		current, stateErr := runtime.GetDocument(ctx, doc.DocumentID)
		if stateErr != nil {
			t.Fatalf("GetDocument: %v", stateErr)
		}
		return current
	}

	audioBytes := presentationWAV(16000)
	mp3Bytes := presentationMP3()
	videoBytes := []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm',
		0x00, 0x00, 0x00, 0x00, 'm', 'p', '4', '2', 'a', 'v', 'c', '1',
	}
	audioPath := filepath.Join(t.TempDir(), "report.wav")
	mp3Path := filepath.Join(t.TempDir(), "announcement.mp3")
	videoPath := filepath.Join(t.TempDir(), "summary.mp4")
	badVideoPath := filepath.Join(t.TempDir(), "invalid.mp4")
	if err := os.WriteFile(audioPath, audioBytes, 0o600); err != nil {
		t.Fatalf("write WAV fixture: %v", err)
	}
	if err := os.WriteFile(mp3Path, mp3Bytes, 0o600); err != nil {
		t.Fatalf("write MP3 fixture: %v", err)
	}
	if err := os.WriteFile(videoPath, videoBytes, 0o600); err != nil {
		t.Fatalf("write MP4 fixture: %v", err)
	}
	if err := os.WriteFile(badVideoPath, []byte("not a media container"), 0o600); err != nil {
		t.Fatalf("write invalid MP4 fixture: %v", err)
	}
	emptyPath := filepath.Join(t.TempDir(), "empty.wav")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("write empty media fixture: %v", err)
	}

	beforeInvalid := info()
	if _, err := invoke("presentation.element.audio.create", testPresentationmediaSlideIdSourcePathMediaTypeXArguments{SlideId: slideID, SourcePath: audioPath, MediaType: "audio/mpeg", X: 48, Y: 60, Width: 100, Height: 48}); err == nil {
		t.Fatal("audio capability accepted a media type that does not match the file extension")
	}
	if _, err := invoke("presentation.element.audio.create", testPresentationmediaSlideIdSourcePathMediaTypeXArguments{SlideId: slideID, SourcePath: emptyPath, MediaType: "audio/wav", X: 48, Y: 60, Width: 100, Height: 48}); err == nil {
		t.Fatal("audio capability accepted an empty file")
	}
	if _, err := invoke("presentation.element.video.create", testPresentationmediaSlideIdSourcePathMediaTypeArguments{SlideId: slideID, SourcePath: videoPath, MediaType: "video/mp4", X: 48, Y: 60, Width: 100, Height: 48, Decorative: true, AltText: "decorative media cannot have alt text"}); err == nil {
		t.Fatal("video capability accepted alt text for decorative video")
	}
	if _, err := invoke("presentation.element.video.create", testPresentationmediaSlideIdSourcePathArguments{SlideId: slideID, SourcePath: badVideoPath, MediaType: "video/mp4", X: 48, Y: 60, Width: 100, Height: 48}); err == nil {
		t.Fatal("video capability accepted an unrecognized media container")
	}
	if after := info(); after.Revision != beforeInvalid.Revision || after.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid media requests changed presentation: before=%+v after=%+v", beforeInvalid, after)
	}

	audio, err := invoke("presentation.element.audio.create", presentationElementAudioCreatePayload{SlideId: slideID, SourcePath: audioPath, MediaType: "audio/wav", Role: "narration", X: 48, Y: 60, Width: 120, Height: 48})
	if err != nil {
		t.Fatalf("embed audio: %v", err)
	}
	var audioResult struct {
		ElementID      int64  `json:"element_id"`
		MediaType      string `json:"media_type"`
		MediaSizeBytes int    `json:"media_size_bytes"`
		Changed        bool   `json:"changed"`
	}
	if err := json.Unmarshal(audio.Data, &audioResult); err != nil || audioResult.ElementID <= 0 || audioResult.MediaType != "audio/wav" || audioResult.MediaSizeBytes != len(audioBytes) || !audioResult.Changed {
		t.Fatalf("audio result=%s error=%v", audio.Data, err)
	}
	mp3, err := invoke("presentation.element.audio.create", presentationElementAudioCreatePayload{SlideId: slideID, SourcePath: mp3Path, MediaType: "audio/mpeg", Role: "background", X: 48, Y: 120, Width: 120, Height: 48})
	if err != nil {
		t.Fatalf("embed MP3 audio: %v", err)
	}
	var mp3Result struct {
		ElementID      int64  `json:"element_id"`
		MediaType      string `json:"media_type"`
		MediaSizeBytes int    `json:"media_size_bytes"`
	}
	if err := json.Unmarshal(mp3.Data, &mp3Result); err != nil || mp3Result.ElementID <= 0 || mp3Result.MediaType != "audio/mpeg" || mp3Result.MediaSizeBytes != len(mp3Bytes) {
		t.Fatalf("MP3 result=%s error=%v", mp3.Data, err)
	}

	video, err := invoke("presentation.element.video.create", presentationElementVideoCreatePayload{SlideId: slideID, SourcePath: videoPath, MediaType: "video/mp4", X: 220, Y: 60, Width: 240, Height: 135, AltText: "季度经营汇报视频"})
	if err != nil {
		t.Fatalf("embed video: %v", err)
	}
	var videoResult struct {
		ElementID      int64  `json:"element_id"`
		MediaType      string `json:"media_type"`
		MediaSizeBytes int    `json:"media_size_bytes"`
		Changed        bool   `json:"changed"`
	}
	if err := json.Unmarshal(video.Data, &videoResult); err != nil || videoResult.ElementID <= 0 || videoResult.MediaType != "video/mp4" || videoResult.MediaSizeBytes != len(videoBytes) || !videoResult.Changed {
		t.Fatalf("video result=%s error=%v", video.Data, err)
	}
	if current := info(); current.Revision != revision || !current.Dirty || current.Revision != doc.Revision+3 {
		t.Fatalf("media inserts did not create three revisions: %+v initial=%+v", current, doc)
	}

	path := filepath.Join(t.TempDir(), "presentation-media.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save media presentation: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close media presentation: %v", err)
	}
	reopened, err := pptx.Open(path)
	if err != nil {
		t.Fatalf("reopen media PPTX: %v", err)
	}
	snapshot := reopened.Snapshot()
	_ = reopened.Close()
	if len(snapshot.Slides) != 1 {
		t.Fatalf("reopened media slides=%d; want 1", len(snapshot.Slides))
	}
	var foundWAV, foundMP3, foundVideo bool
	for _, element := range snapshot.Slides[0].Elements {
		switch element.ID {
		case audioResult.ElementID:
			foundWAV = element.Kind == document.ElementKindAudio && element.Bounds == (document.PresentationRect{X: 48, Y: 60, W: 120, H: 48})
		case mp3Result.ElementID:
			foundMP3 = element.Kind == document.ElementKindAudio && element.Bounds == (document.PresentationRect{X: 48, Y: 120, W: 120, H: 48})
		case videoResult.ElementID:
			foundVideo = element.Kind == document.ElementKindVideo && element.AltText == "季度经营汇报视频" && element.Bounds == (document.PresentationRect{X: 220, Y: 60, W: 240, H: 135})
		}
	}
	if !foundWAV || !foundMP3 || !foundVideo {
		t.Fatalf("media targets did not round-trip: wav=%t mp3=%t video=%t elements=%+v", foundWAV, foundMP3, foundVideo, snapshot.Slides[0].Elements)
	}
	assertPPTXMediaPartBytes(t, path, audioBytes)
	assertPPTXMediaPartBytes(t, path, mp3Bytes)
	assertPPTXMediaPartBytes(t, path, videoBytes)
}

func assertPPTXMediaPartBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open media PPTX package: %v", err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "ppt/media/") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open media part %q: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read media part %q: read=%v close=%v", file.Name, readErr, closeErr)
		}
		if bytes.Equal(data, want) {
			return
		}
	}
	t.Fatal("PPTX media part did not preserve exact source bytes")
}

func presentationWAV(dataSize int) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(36+dataSize))
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(8000))
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(16000))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(dataSize))
	buffer.Write(make([]byte, dataSize))
	return buffer.Bytes()
}

func presentationMP3() []byte {
	header := uint32(0xFFE00000) | uint32(0b11)<<19 | uint32(0b01)<<17 | uint32(1)<<16 | uint32(9)<<12
	data := make([]byte, 144*128000/44100)
	data[0], data[1], data[2], data[3] = byte(header>>24), byte(header>>16), byte(header>>8), byte(header)
	return data
}
