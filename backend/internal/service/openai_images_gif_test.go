package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var openAIGIFTestPalette = color.Palette{
	color.RGBA{A: 0xff},
	color.RGBA{R: 0xff, A: 0xff},
	color.RGBA{G: 0xff, A: 0xff},
	color.RGBA{B: 0xff, A: 0xff},
}

// encodeOpenAIGIFTestAnimation builds a full-canvas test animation where every
// frame is filled with one palette color, so a test can tell which source frame
// survived a frame-count rewrite.
func encodeOpenAIGIFTestAnimation(t *testing.T, frames, delayCS, width, height int) []byte {
	t.Helper()
	images := make([]*image.Paletted, frames)
	delays := make([]int, frames)
	for index := range images {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), openAIGIFTestPalette)
		for pixel := range frame.Pix {
			frame.Pix[pixel] = uint8(index % len(openAIGIFTestPalette))
		}
		images[index] = frame
		delays[index] = delayCS
	}
	var buffer bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buffer, &gif.GIF{
		Image:     images,
		Delay:     delays,
		LoopCount: 0,
		Config:    image.Config{ColorModel: openAIGIFTestPalette, Width: width, Height: height},
	}))
	return buffer.Bytes()
}

func decodeOpenAIGIFTestAnimation(t *testing.T, data []byte) *gif.GIF {
	t.Helper()
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	require.NoError(t, err)
	return animation
}

func openAIGIFTestDecodeBase64(t *testing.T, encoded string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	return decoded
}

func TestNormalizeOpenAIImagesGIFAnimation_HonorsExplicitControls(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 9, 20, 64, 64)
	frameCount := 4
	frameDurationMS := 100

	normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{
		frameCount:      &frameCount,
		frameDurationMS: &frameDurationMS,
		size:            "32x32",
	})
	require.False(t, bytes.Equal(source, normalized), "mismatched animation must be rewritten")

	animation := decodeOpenAIGIFTestAnimation(t, normalized)
	require.Len(t, animation.Image, 4)
	require.Equal(t, 32, animation.Config.Width)
	require.Equal(t, 32, animation.Config.Height)
	require.Len(t, animation.Delay, 4)
	for index, delay := range animation.Delay {
		require.Equal(t, 10, delay, "frame %d delay", index)
	}
	for index, frame := range animation.Image {
		require.Equal(t, image.Rect(0, 0, 32, 32), frame.Bounds(), "frame %d bounds", index)
	}
	// Four evenly-spaced frames of nine: 0, 2, 4, 6.
	sourceAnimation := decodeOpenAIGIFTestAnimation(t, source)
	for index, wantSource := range []int{0, 2, 4, 6} {
		require.Equal(
			t,
			color.RGBAModel.Convert(sourceAnimation.Image[wantSource].At(0, 0)),
			color.RGBAModel.Convert(animation.Image[index].At(0, 0)),
			"frame %d samples source frame %d", index, wantSource,
		)
	}
	require.Equal(t, 0, animation.LoopCount)
}

func TestNormalizeOpenAIImagesGIFAnimation_KeepsMatchingAnimation(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 2, 10, 16, 16)
	frameCount := 2
	frameDurationMS := 100

	normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{
		frameCount:      &frameCount,
		frameDurationMS: &frameDurationMS,
		size:            "16x16",
	})
	require.True(t, bytes.Equal(source, normalized), "an animation that already matches stays byte-identical")
}

func TestNormalizeOpenAIImagesGIFAnimation_WithoutControlsIsNoop(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 9, 20, 32, 32)
	require.True(t, bytes.Equal(source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{})))
}

func TestNormalizeOpenAIImagesGIFAnimation_ExpandsFrameCount(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 3, 20, 16, 16)
	frameCount := 5

	normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})
	animation := decodeOpenAIGIFTestAnimation(t, normalized)
	require.Len(t, animation.Image, 5)
	sourceAnimation := decodeOpenAIGIFTestAnimation(t, source)
	for index, wantSource := range []int{0, 0, 1, 1, 2} {
		require.Equal(
			t,
			color.RGBAModel.Convert(sourceAnimation.Image[wantSource].At(0, 0)),
			color.RGBAModel.Convert(animation.Image[index].At(0, 0)),
			"frame %d duplicates source frame %d", index, wantSource,
		)
		for _, delay := range animation.Delay {
			require.Equal(t, 20, delay, "unspecified duration keeps the upstream delay")
		}
	}
}

func TestNormalizeOpenAIImagesGIFAnimation_RoundsFrameDuration(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 2, 20, 16, 16)
	frameDurationMS := 125

	normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameDurationMS: &frameDurationMS})
	animation := decodeOpenAIGIFTestAnimation(t, normalized)
	require.Len(t, animation.Image, 2)
	require.Equal(t, []int{13, 13}, animation.Delay, "125 ms rounds to the 10 ms GIF delay unit")
}

func TestNormalizeOpenAIImagesGIFAnimation_IgnoresOutOfRangeControls(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 9, 20, 16, 16)
	for _, frameCount := range []int{0, 1, 17, 64} {
		require.True(
			t,
			bytes.Equal(source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})),
			"frame_count=%d stays upstream-owned", frameCount,
		)
	}
	for _, duration := range []int{0, 5, 19, 2001} {
		require.True(
			t,
			bytes.Equal(source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameDurationMS: &duration})),
			"frame_duration_ms=%d stays upstream-owned", duration,
		)
	}
	require.True(t, bytes.Equal(source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{size: "auto"})))
}

func TestNormalizeOpenAIImagesGIFAnimation_LeavesNonGIFBytesUntouched(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n not really a png")
	frameCount := 4
	require.True(t, bytes.Equal(png, normalizeOpenAIImagesGIFAnimation(png, openAIGIFAnimationControls{frameCount: &frameCount})))
}

func TestNormalizeOpenAIImagesGIFAnimationResponseBody_RewritesGIFItemsOnly(t *testing.T) {
	gifSource := encodeOpenAIGIFTestAnimation(t, 9, 20, 32, 32)
	pngPayload := []byte("not a gif")
	body := []byte(`{"created":1,"data":[{"b64_json":""},{"b64_json":""},{"b64_json":""}]}`)
	var err error
	body, err = sjson.SetBytes(body, "data.0.b64_json", base64.StdEncoding.EncodeToString(gifSource))
	require.NoError(t, err)
	body, err = sjson.SetBytes(body, "data.0.output_format", "gif")
	require.NoError(t, err)
	body, err = sjson.SetBytes(body, "data.1.b64_json", base64.StdEncoding.EncodeToString(pngPayload))
	require.NoError(t, err)
	body, err = sjson.SetBytes(body, "data.1.output_format", "png")
	require.NoError(t, err)

	frameCount := 2
	parsed := &OpenAIImagesRequest{OutputFormat: "gif", FrameCount: &frameCount}
	normalizedBody := normalizeOpenAIImagesGIFAnimationResponseBody(body, parsed)

	rewritten := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, gjson.GetBytes(normalizedBody, "data.0.b64_json").String()))
	require.Len(t, rewritten.Image, 2)
	require.Equal(t, base64.StdEncoding.EncodeToString(pngPayload), gjson.GetBytes(normalizedBody, "data.1.b64_json").String(), "non-GIF results are untouched")
}

func TestNormalizeOpenAIImagesGIFAnimationResponseBody_WithoutOutputFormatUsesItemFormat(t *testing.T) {
	gifSource := encodeOpenAIGIFTestAnimation(t, 9, 20, 16, 16)
	body := []byte(`{"data":[{"b64_json":"","output_format":"gif"}]}`)
	body, err := sjson.SetBytes(body, "data.0.b64_json", base64.StdEncoding.EncodeToString(gifSource))
	require.NoError(t, err)

	frameDurationMS := 600
	parsed := &OpenAIImagesRequest{FrameDurationMS: &frameDurationMS}
	normalizedBody := normalizeOpenAIImagesGIFAnimationResponseBody(body, parsed)
	animation := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, gjson.GetBytes(normalizedBody, "data.0.b64_json").String()))
	require.Len(t, animation.Image, 9, "the request did not change the frame count")
	for _, delay := range animation.Delay {
		require.Equal(t, 60, delay)
	}
}

func TestNormalizeOpenAIImagesGIFAnimationEvent(t *testing.T) {
	gifSource := encodeOpenAIGIFTestAnimation(t, 9, 20, 16, 16)
	event := []byte(`{"type":"image_generation.completed","b64_json":"","output_format":"gif"}`)
	event, err := sjson.SetBytes(event, "b64_json", base64.StdEncoding.EncodeToString(gifSource))
	require.NoError(t, err)

	frameCount := 4
	parsed := &OpenAIImagesRequest{OutputFormat: "gif", FrameCount: &frameCount}
	budget := newOpenAIGIFAnimationBudget()
	normalizedEvent := normalizeOpenAIImagesGIFAnimationEvent(event, parsed, &budget)
	require.Equal(t, "image_generation.completed", gjson.GetBytes(normalizedEvent, "type").String())
	animation := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, gjson.GetBytes(normalizedEvent, "b64_json").String()))
	require.Len(t, animation.Image, 4)
}

func TestResizeOpenAIImagesGIFFrame_PreservesSubRectOffsets(t *testing.T) {
	source := image.NewPaletted(image.Rect(8, 8, 24, 24), openAIGIFTestPalette)
	resized := resizeOpenAIImagesGIFFrame(source, 64, 64, 32, 32)
	require.Equal(t, image.Rect(4, 4, 12, 12), resized.Bounds())
}

func TestOpenAIGatewayServiceForwardImages_APIKeyHonorsGIFAnimation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"a looping lantern","size":"32x32","output_format":"gif","frame_count":2,"frame_duration_ms":100}`)
	c, rec := newOpenAIImagesTestContext(t, body)

	upstreamGIF := encodeOpenAIGIFTestAnimation(t, 9, 20, 64, 64)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"created":1710000000,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(upstreamGIF) + `","output_format":"gif","size":"64x64"}]}`,
		)),
	}}
	svc := newOpenAIImagesTestService(upstream)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
	require.NoError(t, err)

	animation := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, gjson.Get(rec.Body.String(), "data.0.b64_json").String()))
	require.Len(t, animation.Image, 2)
	require.Equal(t, 32, animation.Config.Width)
	require.Equal(t, 32, animation.Config.Height)
	require.Equal(t, []int{10, 10}, animation.Delay)
	require.Equal(t, "32x32", gjson.Get(rec.Body.String(), "data.0.size").String())
	require.Equal(t, []string{"32x32"}, result.ImageOutputSizes)
}

func TestOpenAIGatewayServiceForwardImages_OAuthDirectHonorsGIFAnimation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, responseFormat := range []string{"b64_json", "url"} {
		t.Run(responseFormat, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2.6-flare","prompt":"a looping lantern","size":"32x32","output_format":"gif","frame_count":2,"frame_duration_ms":100,"response_format":%q}`, responseFormat))
			c, rec := newOpenAIImagesTestContext(t, body)
			encoded := base64.StdEncoding.EncodeToString(encodeOpenAIGIFTestAnimation(t, 9, 20, 64, 64))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"` + encoded + `","output_format":"gif","size":"64x64"}],"usage":{"input_tokens":11,"output_tokens":22,"input_tokens_details":{"cached_tokens":3}}}`)),
			}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.ImageCount)
			require.Equal(t, []string{"32x32"}, result.ImageOutputSizes)
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 22, result.Usage.ImageOutputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
			require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "frame_count").Int())
			require.Equal(t, int64(100), gjson.GetBytes(upstream.lastBody, "frame_duration_ms").Int())
			require.Equal(t, "32x32", gjson.Get(rec.Body.String(), "size").String())
			require.Equal(t, "32x32", gjson.Get(rec.Body.String(), "data.0.size").String())
			response := gjson.Get(rec.Body.String(), "data.0.b64_json").String()
			if responseFormat == "url" {
				require.Empty(t, response)
				url := gjson.Get(rec.Body.String(), "data.0.url").String()
				require.True(t, strings.HasPrefix(url, "data:image/gif;base64,"))
				response = strings.TrimPrefix(url, "data:image/gif;base64,")
			}
			animation := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, response))
			require.Len(t, animation.Image, 2)
			require.Equal(t, 32, animation.Config.Width)
			require.Equal(t, 32, animation.Config.Height)
			require.Equal(t, []int{10, 10}, animation.Delay)
		})
	}
}

func TestOpenAIGatewayServiceForwardImages_OAuthDirectStreamingHonorsGIFAnimation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"a looping lantern","size":"16x16","output_format":"gif","frame_count":4,"frame_duration_ms":200,"stream":true,"response_format":"url"}`)
	c, rec := newOpenAIImagesTestContext(t, body)

	upstreamGIF := encodeOpenAIGIFTestAnimation(t, 9, 20, 64, 64)
	streamBody := "event: image_generation.completed\n" +
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"" + base64.StdEncoding.EncodeToString(upstreamGIF) + "\",\"output_format\":\"gif\",\"usage\":{\"input_tokens\":11,\"output_tokens\":22}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}}
	svc := newOpenAIImagesTestService(upstream)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), body, parsed, "")
	require.NoError(t, err)

	var completedData string
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "image_generation.completed") {
			completedData = strings.TrimPrefix(line, "data: ")
		}
	}
	require.NotEmpty(t, completedData)
	animation := decodeOpenAIGIFTestAnimation(t, openAIGIFTestDecodeBase64(t, gjson.Get(completedData, "b64_json").String()))
	require.Len(t, animation.Image, 4)
	require.Equal(t, 16, animation.Config.Width)
	require.Equal(t, 16, animation.Config.Height)
	require.Equal(t, []int{20, 20, 20, 20}, animation.Delay)
	require.Equal(t, "16x16", gjson.Get(completedData, "size").String())
	require.Equal(t, "data:image/gif;base64,"+gjson.Get(completedData, "b64_json").String(), gjson.Get(completedData, "url").String())
	require.Equal(t, []string{"16x16"}, result.ImageOutputSizes)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 22, result.Usage.ImageOutputTokens)
}

func TestScanOpenAIGIFMetadata(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 9, 20, 64, 48)
	metadata, ok := scanOpenAIGIFMetadata(source)
	require.True(t, ok)
	require.Equal(t, 64, metadata.width)
	require.Equal(t, 48, metadata.height)
	require.Equal(t, 9, metadata.frameCount)
	require.Len(t, metadata.delays, 9)
	for _, delay := range metadata.delays {
		require.Equal(t, 20, delay)
	}

	_, ok = scanOpenAIGIFMetadata([]byte("not a gif at all"))
	require.False(t, ok)
}

// encodeOpenAIGIFDeltaTestAnimation builds an animation whose frames after the
// first only encode the rectangle that changed since the previous frame, like
// Pillow's default encoder does, and returns the logical fully rendered frames
// so a test can assert what the animation should look like.
func encodeOpenAIGIFDeltaTestAnimation(t *testing.T, frames, width, height int) ([]byte, []*image.Paletted) {
	t.Helper()
	palette := color.Palette{
		color.RGBA{A: 0xff},
		color.RGBA{B: 0xff, A: 0xff},
		color.RGBA{R: 0xff, A: 0xff},
	}
	logical := make([]*image.Paletted, frames)
	for index := range logical {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if (x/8+y/8)%2 == 0 {
					frame.SetColorIndex(x, y, 0)
				} else {
					frame.SetColorIndex(x, y, 1)
				}
			}
		}
		// A 10x10 block marching right by six pixels per frame, which is the
		// pattern that exposes delta-frame sampling bugs.
		left := 4 + index*6
		for y := 20; y < 30; y++ {
			for x := left; x < left+10; x++ {
				frame.SetColorIndex(x, y, 2)
			}
		}
		logical[index] = frame
	}
	delta := make([]*image.Paletted, frames)
	delta[0] = logical[0]
	for index := 1; index < frames; index++ {
		bounds := openAIGIFTestDifferenceBounds(logical[index-1], logical[index])
		require.False(t, bounds.Empty(), "frame %d must differ from its predecessor", index)
		frame := image.NewPaletted(bounds, palette)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				frame.SetColorIndex(x, y, logical[index].ColorIndexAt(x, y))
			}
		}
		delta[index] = frame
	}
	delays := make([]int, frames)
	for index := range delays {
		delays[index] = 20
	}
	var buffer bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buffer, &gif.GIF{
		Image:     delta,
		Delay:     delays,
		LoopCount: 0,
		Config:    image.Config{ColorModel: palette, Width: width, Height: height},
	}))
	return buffer.Bytes(), logical
}

func openAIGIFTestDifferenceBounds(previous, current *image.Paletted) image.Rectangle {
	bounds := previous.Bounds().Union(current.Bounds())
	result := image.Rectangle{Min: image.Pt(bounds.Max.X, bounds.Max.Y), Max: image.Pt(bounds.Min.X, bounds.Min.Y)}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if previous.ColorIndexAt(x, y) == current.ColorIndexAt(x, y) {
				continue
			}
			result = result.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return result
}

func requireOpenAIGIFTestFrameMatches(t *testing.T, want, got *image.Paletted, frameIndex int) {
	t.Helper()
	require.Equal(t, want.Bounds(), got.Bounds(), "frame %d covers the whole canvas", frameIndex)
	for y := want.Bounds().Min.Y; y < want.Bounds().Max.Y; y++ {
		for x := want.Bounds().Min.X; x < want.Bounds().Max.X; x++ {
			require.Equal(
				t,
				color.RGBAModel.Convert(want.At(x, y)),
				color.RGBAModel.Convert(got.At(x, y)),
				"frame %d pixel (%d, %d)", frameIndex, x, y,
			)
		}
	}
}

// TestNormalizeOpenAIImagesGIFAnimation_ReducedFrameCountComposesDeltaFrames is
// the regression test for sampling delta frames directly: dropping frames left
// the dropped frames' pixels on the canvas (娈嬪奖 / 鑺卞睆).
func TestNormalizeOpenAIImagesGIFAnimation_ReducedFrameCountComposesDeltaFrames(t *testing.T) {
	source, logical := encodeOpenAIGIFDeltaTestAnimation(t, 9, 64, 64)
	decodedSource := decodeOpenAIGIFTestAnimation(t, source)
	deltaFrames := 0
	for _, frame := range decodedSource.Image {
		if frame.Bounds() != decodedSource.Image[0].Bounds() {
			deltaFrames++
		}
	}
	require.Positive(t, deltaFrames, "fixture must contain sub-rectangle delta frames")

	for _, target := range []int{2, 4, 5} {
		frameCount := target
		normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})
		require.False(t, bytes.Equal(source, normalized), "frame_count=%d must rewrite the animation", target)
		animation := decodeOpenAIGIFTestAnimation(t, normalized)
		require.Len(t, animation.Image, target)
		for index, frame := range animation.Image {
			want := logical[openAIGIFFrameIndex(index, len(logical), target)]
			requireOpenAIGIFTestFrameMatches(t, want, frame, index)
		}
	}
}

// reduce without other controls must not be affected; check a matching
// frame_count stays byte-identical.
func TestNormalizeOpenAIImagesGIFAnimation_DeltaFrameCountUnchangedIsNoop(t *testing.T) {
	source, _ := encodeOpenAIGIFDeltaTestAnimation(t, 9, 64, 64)
	frameCount := 9
	require.True(t, bytes.Equal(source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})))
}

func TestNormalizeOpenAIImagesGIFAnimation_OversizedTargetIsNoop(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 2, 20, 4, 4)
	for _, size := range []string{"65536x1", "8192x8192", "4294967296x4294967296"} {
		t.Run(size, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.Equal(t, source, normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{size: size}))
			})
		})
	}
}

func TestNormalizeOpenAIImagesGIFAnimation_ResourceGuardsKeepSource(t *testing.T) {
	frameCount := 2
	largeCanvas := encodeOpenAIGIFTestAnimation(t, 3, 20, 1, 1)
	binary.LittleEndian.PutUint16(largeCanvas[6:8], 65535)
	binary.LittleEndian.PutUint16(largeCanvas[8:10], 65535)
	require.Equal(t, largeCanvas, normalizeOpenAIImagesGIFAnimation(largeCanvas, openAIGIFAnimationControls{frameCount: &frameCount}))

	manyFrames := encodeOpenAIGIFTestAnimation(t, maxOpenAIGIFAnimationFrames+1, 20, 1, 1)
	require.Equal(t, manyFrames, normalizeOpenAIImagesGIFAnimation(manyFrames, openAIGIFAnimationControls{frameCount: &frameCount}))

	// The input and output have only 2K and 128K pixels, but CatmullRom's
	// intermediate would require more than 2 GiB without a separate guard.
	tall := encodeOpenAIGIFTestAnimation(t, 2, 20, 1, 1024)
	require.Equal(t, tall, normalizeOpenAIImagesGIFAnimation(tall, openAIGIFAnimationControls{size: "65535x1"}))
}

func TestNormalizeOpenAIImagesGIFAnimation_TruncatedGIFIsNoop(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 3, 20, 4, 4)
	frameCount := 2
	for end := 0; end < len(source); end++ {
		require.Equal(t, source[:end], normalizeOpenAIImagesGIFAnimation(source[:end], openAIGIFAnimationControls{frameCount: &frameCount}), "prefix length %d", end)
	}
}

func TestNormalizeOpenAIImagesGIFAnimationResponseBody_NoopPreservesBytes(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(encodeOpenAIGIFTestAnimation(t, 2, 20, 4, 4))
	frameCount := 2
	for _, body := range []string{
		`not JSON`, `{"data":{}}`, `{"data":[{"b64_json":"not base64","output_format":"gif"}]}`,
		`{"data":[{"b64_json":"` + encoded + `","output_format":"png"}]}`,
		`{ "data": [{"b64_json":"` + encoded + `","output_format":"gif"}], "usage":{"output_tokens":42} }`,
	} {
		parsed := &OpenAIImagesRequest{FrameCount: &frameCount}
		require.Equal(t, []byte(body), normalizeOpenAIImagesGIFAnimationResponseBody([]byte(body), parsed))
		require.Equal(t, []byte(body), normalizeOpenAIImagesGIFAnimationResponseBody([]byte(body), nil))
	}
}

func TestNormalizeOpenAIImagesGIFAnimationResponseBody_SharesBudget(t *testing.T) {
	for _, resource := range []string{"frames", "pixels"} {
		t.Run(resource, func(t *testing.T) {
			var source []byte
			if resource == "frames" {
				source = encodeOpenAIGIFTestAnimation(t, 600, 20, 1, 1)
			} else {
				// Valid small sub-rectangles on a 4K canvas exercise the
				// conservative pixel budget without allocating large images.
				source = encodeOpenAIGIFTestAnimation(t, 4, 20, 1, 1)
				binary.LittleEndian.PutUint16(source[6:8], 4096)
				binary.LittleEndian.PutUint16(source[8:10], 4096)
			}
			encoded := base64.StdEncoding.EncodeToString(source)
			small := base64.StdEncoding.EncodeToString(encodeOpenAIGIFTestAnimation(t, 2, 20, 1, 1))
			body := []byte(fmt.Sprintf(`{"data":[{"b64_json":%q},{"b64_json":%q},{"b64_json":%q}],"usage":{"output_tokens":22}}`, encoded, encoded, small))
			duration := 120
			parsed := &OpenAIImagesRequest{OutputFormat: "gif", FrameDurationMS: &duration}
			normalized := normalizeOpenAIImagesGIFAnimationResponseBody(body, parsed)
			first := gjson.GetBytes(normalized, "data.0.b64_json").String()
			require.NotEqual(t, encoded, first)
			require.Equal(t, encoded, gjson.GetBytes(normalized, "data.1.b64_json").String())
			require.Equal(t, small, gjson.GetBytes(normalized, "data.2.b64_json").String(), "budget exhaustion persists for later, smaller items")
			require.Equal(t, int64(22), gjson.GetBytes(normalized, "usage.output_tokens").Int())
		})
	}
}

func TestOpenAIGIFAnimationBudget_ReservesBeforeDecode(t *testing.T) {
	duration := 120
	controls := openAIGIFAnimationControls{frameDurationMS: &duration}
	source := encodeOpenAIGIFTestAnimation(t, 2, 20, 4, 4)
	// The block structure remains valid, but the first frame's LZW code size
	// is invalid. DecodeAll must fail after the work has already been charged.
	position := bytes.Index(source, []byte{0x2c, 0, 0, 0, 0, 4, 0, 4, 0, 0})
	require.NotEqual(t, -1, position)
	require.Zero(t, source[position+9]&0x80, "fixture uses the global color table")
	source[position+10] = 1
	budget := newOpenAIGIFAnimationBudget()
	require.Equal(t, source, normalizeOpenAIImagesGIFAnimationWithBudget(source, controls, &budget))
	require.Equal(t, maxOpenAIGIFAnimationFrames-2, budget.sourceFrames)
	require.Equal(t, maxOpenAIGIFAnimationPixels-int64(32), budget.sourcePixels)
	require.Equal(t, maxOpenAIGIFAnimationPixels-int64(32), budget.targetPixels)
}

func TestOpenAIGIFAnimationBudget_NoopDoesNotSpend(t *testing.T) {
	source := encodeOpenAIGIFTestAnimation(t, 2, 20, 4, 4)
	count, duration := 2, 200
	badCount, badDuration := 17, 19
	for _, controls := range []openAIGIFAnimationControls{
		{}, {frameCount: &count, frameDurationMS: &duration, size: "4x4"},
		{frameCount: &badCount, frameDurationMS: &badDuration, size: "auto"},
	} {
		budget := newOpenAIGIFAnimationBudget()
		before := budget
		require.Equal(t, source, normalizeOpenAIImagesGIFAnimationWithBudget(source, controls, &budget))
		require.Equal(t, before, budget)
	}
}

func TestOpenAIGIFAnimationBudget_ResizeWorkIsCumulative(t *testing.T) {
	budget := newOpenAIGIFAnimationBudget()
	// Each resize fits the 8 Mi-pixel peak scratch limit; six such animations
	// exhaust the response-wide work limit, despite tiny endpoint images.
	metadata := openAIGIFMetadata{width: 1, height: 32768, frameCount: 2}
	for index := 0; index < 6; index++ {
		require.True(t, budget.reserve(metadata, 2, 256, 1, true), "animation %d", index)
	}
	require.True(t, budget.exhausted)
	require.False(t, budget.reserve(metadata, 2, 256, 1, true))

	// A normal 2K-to-4K nine-frame resize is admitted; peak scratch is not
	// incorrectly treated as a response-wide allocation limit.
	budget = newOpenAIGIFAnimationBudget()
	require.True(t, budget.reserve(openAIGIFMetadata{width: 2048, height: 1536, frameCount: 9}, 9, 3840, 2160, true))
	// Expanding repeated frames scales each source frame only once.
	budget = newOpenAIGIFAnimationBudget()
	require.True(t, budget.reserve(openAIGIFMetadata{width: 32, height: 32, frameCount: 2}, 16, 64, 64, true))
	require.Equal(t, int64(maxOpenAIGIFAnimationPixels-2*64*32), budget.resizePixels)
}

func TestOpenAIGatewayServiceForwardImages_OAuthDirectGIFStreamSharesBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, disconnected := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnected=%t", disconnected), func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"a looping lantern","n":3,"output_format":"gif","frame_duration_ms":120,"stream":true}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			if disconnected {
				c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 1}
			}
			encoded := base64.StdEncoding.EncodeToString(encodeOpenAIGIFTestAnimation(t, 600, 20, 1, 1))
			small := base64.StdEncoding.EncodeToString(encodeOpenAIGIFTestAnimation(t, 2, 20, 1, 1))
			var stream strings.Builder
			for index, image := range []string{encoded, encoded, small} {
				fmt.Fprintf(&stream, "data: {\"type\":\"image_generation.completed\",\"image_index\":%d,\"b64_json\":%q,\"output_format\":\"gif\",\"usage\":{\"input_tokens\":11,\"output_tokens\":%d}}\n\n", index, image, 20+index)
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(stream.String())),
			}}
			svc := newOpenAIImagesTestService(upstream)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, newDirectImagesOAuthAccount(), body, parsed, "")
			require.NoError(t, err)
			if !disconnected {
				var results []string
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") {
						results = append(results, gjson.Get(strings.TrimPrefix(line, "data: "), "b64_json").String())
					}
				}
				require.Len(t, results, 3)
				require.NotEqual(t, encoded, results[0])
				require.Equal(t, encoded, results[1])
				require.Equal(t, small, results[2])
			}
			require.Equal(t, 3, result.ImageCount)
			require.Equal(t, 22, result.Usage.OutputTokens, "budget exhaustion must not stop terminal usage collection")
		})
	}
}

func TestNormalizeOpenAIImagesGIFAnimation_ReductionPreservesDisposal(t *testing.T) {
	for _, transparent := range []bool{false, true} {
		for _, disposal := range []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalPrevious} {
			t.Run(fmt.Sprintf("transparent=%t/disposal=%d", transparent, disposal), func(t *testing.T) {
				palette := append(color.Palette(nil), openAIGIFTestPalette...)
				if transparent {
					palette[0] = color.Transparent
				}
				images := make([]*image.Paletted, 4)
				for index := range images {
					images[index] = image.NewPaletted(image.Rect(index, 0, index+1, 1), palette)
					images[index].SetColorIndex(index, 0, uint8(index%3+1))
				}
				var buffer bytes.Buffer
				require.NoError(t, gif.EncodeAll(&buffer, &gif.GIF{
					Image: images, Delay: []int{20, 20, 20, 20},
					Disposal: []byte{disposal, disposal, disposal, disposal},
					Config:   image.Config{ColorModel: palette, Width: 4, Height: 1}, BackgroundIndex: 2,
				}))
				source := buffer.Bytes()
				frameCount := 2
				normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})
				require.NotEqual(t, source, normalized)
				want := renderOpenAIGIFTestAnimation(decodeOpenAIGIFTestAnimation(t, source))
				got := renderOpenAIGIFTestAnimation(decodeOpenAIGIFTestAnimation(t, normalized))
				require.Len(t, got, 2)
				require.Equal(t, want[0].Pix, got[0].Pix)
				require.Equal(t, want[2].Pix, got[1].Pix)
			})
		}
	}
}

func TestNormalizeOpenAIImagesGIFAnimation_LocalPaletteBackground(t *testing.T) {
	local := color.Palette{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	images := make([]*image.Paletted, 4)
	for index := range images {
		images[index] = image.NewPaletted(image.Rect(index, 0, index+1, 1), local)
		images[index].SetColorIndex(index, 0, uint8(index%2))
	}
	var buffer bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buffer, &gif.GIF{
		Image: images, Delay: []int{20, 20, 20, 20},
		Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious, gif.DisposalNone, gif.DisposalNone},
		Config:   image.Config{ColorModel: openAIGIFTestPalette, Width: 4, Height: 1}, BackgroundIndex: 2,
	}))
	frameCount := 2
	source := buffer.Bytes()
	normalized := normalizeOpenAIImagesGIFAnimation(source, openAIGIFAnimationControls{frameCount: &frameCount})
	require.NotEqual(t, source, normalized)
	want := renderOpenAIGIFTestAnimation(decodeOpenAIGIFTestAnimation(t, source))
	got := renderOpenAIGIFTestAnimation(decodeOpenAIGIFTestAnimation(t, normalized))
	require.Len(t, got, 2)
	require.Equal(t, want[0].Pix, got[0].Pix)
	require.Equal(t, want[2].Pix, got[1].Pix)
}

func TestNormalizeOpenAIImagesGIFAnimation_ExpansionPreservesDisplayedFrames(t *testing.T) {
	for _, disposal := range []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalPrevious} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("disposal=%d/partial=%t", disposal, partial), func(t *testing.T) {
				first := image.NewPaletted(image.Rect(0, 0, 2, 1), openAIGIFTestPalette)
				first.Pix = []uint8{1, 1} // red / red
				transparentPalette := append(color.Palette(nil), openAIGIFTestPalette...)
				transparentPalette[0] = color.Transparent
				middle := image.NewPaletted(first.Bounds(), transparentPalette)
				middle.Pix = []uint8{0, 3} // transparent / blue
				if partial {
					middle = image.NewPaletted(image.Rect(1, 0, 2, 1), openAIGIFTestPalette)
					middle.Pix = []uint8{3}
				}
				last := image.NewPaletted(first.Bounds(), openAIGIFTestPalette)
				last.Pix = []uint8{2, 2} // green / green
				var buffer bytes.Buffer
				require.NoError(t, gif.EncodeAll(&buffer, &gif.GIF{
					Image: []*image.Paletted{first, middle, last}, Delay: []int{10, 20, 30},
					Disposal: []byte{gif.DisposalNone, disposal, gif.DisposalNone}, LoopCount: 2,
					Config: image.Config{ColorModel: openAIGIFTestPalette, Width: 2, Height: 1},
				}))
				source := buffer.Bytes()
				frameCount := 5
				budget := newOpenAIGIFAnimationBudget()
				normalized := normalizeOpenAIImagesGIFAnimationWithBudget(source, openAIGIFAnimationControls{frameCount: &frameCount}, &budget)
				require.NotEqual(t, source, normalized)
				animation := decodeOpenAIGIFTestAnimation(t, normalized)
				require.Equal(t, []int{10, 10, 20, 20, 30}, animation.Delay)
				require.Equal(t, 2, animation.LoopCount)
				got := renderOpenAIGIFTestAnimation(animation)
				red := color.RGBA{R: 255, A: 255}
				blue := color.RGBA{B: 255, A: 255}
				green := color.RGBA{G: 255, A: 255}
				want := [][2]color.RGBA{{red, red}, {red, red}, {red, blue}, {red, blue}, {green, green}}
				require.Len(t, got, len(want))
				for index, pixels := range want {
					for x, pixel := range pixels {
						require.Equal(t, pixel, got[index].RGBAAt(x, 0), "frame %d pixel %d", index, x)
					}
				}
				require.Equal(t, maxOpenAIGIFAnimationFrames-3, budget.sourceFrames)
				require.Equal(t, maxOpenAIGIFAnimationFrames-5, budget.targetFrames)
				require.Equal(t, int64(maxOpenAIGIFAnimationPixels-6), budget.sourcePixels)
				require.Equal(t, int64(maxOpenAIGIFAnimationPixels-10), budget.targetPixels)
			})
		}
	}
}

// Render independently of the production compositor, including clearing between
// displayed frames. Looking only at decoded frame pixels misses transparency
// trails left by the preceding displayed frame.
func renderOpenAIGIFTestAnimation(animation *gif.GIF) []*image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	background := func(frame *image.Paletted) color.Color {
		for _, entry := range frame.Palette {
			if _, _, _, alpha := entry.RGBA(); alpha == 0 {
				return color.Transparent
			}
		}
		if palette, ok := animation.Config.ColorModel.(color.Palette); ok && int(animation.BackgroundIndex) < len(palette) {
			return palette[animation.BackgroundIndex]
		}
		return color.Transparent
	}
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(background(animation.Image[0])), image.Point{}, draw.Src)
	frames := make([]*image.RGBA, len(animation.Image))
	for index, frame := range animation.Image {
		previous := image.NewRGBA(canvas.Bounds())
		copy(previous.Pix, canvas.Pix)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		frames[index] = image.NewRGBA(canvas.Bounds())
		copy(frames[index].Pix, canvas.Pix)
		switch animation.Disposal[index] {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.NewUniform(background(frame)), image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			copy(canvas.Pix, previous.Pix)
		}
	}
	return frames
}
